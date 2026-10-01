package filemanager

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// Hub is the Files windows of one process, each served by a program half
// of its own, and what they share: the clipboard of paths, the
// favourites, and word of folders an operation changed. A window opens
// another in its hub, as Ctrl and a click on a folder do.
type Hub struct {
	mu   sync.Mutex
	apps []*app
	clip []string
	cut  bool
	// open opens a window with o and serves it until ctx ends, and is nil
	// where no window can open, as in a test.
	open func(ctx context.Context, o Options) error
	// wg counts the windows Open opened that are still open, and errs
	// holds what they ended with.
	wg   sync.WaitGroup
	errs []error
	ga   *gunim.App
}

// NewHub makes a hub whose windows open on ga.
func NewHub(ga *gunim.App) *Hub {
	h := &Hub{ga: ga}
	h.open = func(ctx context.Context, o Options) error {
		_, err := h.Open(ctx, o)
		return err
	}
	return h
}

// Open opens a window with o, and serves it in the background
// until ctx ends or the window closes. It returns the window's client,
// for what the caller does with the window itself, such as a
// screenshot. Wait waits for the window to close.
func (h *Hub) Open(ctx context.Context, o Options) (gunim.Client, error) {
	if h.ga == nil {
		return gunim.Client{}, errors.New("files: the hub has no app to open windows on")
	}
	w, err := h.ga.NewWindow(WindowOptions())
	if err != nil {
		return gunim.Client{}, fmt.Errorf("files: %w", err)
	}
	RegisterViews(w)
	c := w.Client()
	h.wg.Go(func() {
		if err := serve(ctx, c, o, h); err != nil {
			h.mu.Lock()
			h.errs = append(h.errs, err)
			h.mu.Unlock()
		}
	})
	return c, nil
}

// Serve serves a window the caller opened, on c, until ctx ends or the
// window closes. The window must have the views RegisterViews registers,
// and should open with WindowOptions or options like them.
func (h *Hub) Serve(ctx context.Context, c gunim.Client, o Options) error {
	return serve(ctx, c, o, h)
}

// Wait waits for every window Open opened to close, and returns the
// errors they ended with.
func (h *Hub) Wait() error {
	h.wg.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	return errors.Join(h.errs...)
}

// Serve opens a window on ga with o, and serves it and every window
// opened from it until ctx ends or they have all closed.
func Serve(ctx context.Context, ga *gunim.App, o Options) error {
	h := NewHub(ga)
	if _, err := h.Open(ctx, o); err != nil {
		return err
	}
	return h.Wait()
}

// joinHub adds a to h, or to a hub of its own when h is nil.
func joinHub(a *app, h *Hub) *Hub {
	if h == nil {
		h = &Hub{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apps = append(h.apps, a)
	a.ops.clip, a.ops.cut = slices.Clone(h.clip), h.cut
	return h
}

// leave takes a off h.
func (h *Hub) leave(a *app) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apps = slices.DeleteFunc(h.apps, func(o *app) bool { return o == a })
}

// others runs fn on the serve loop of each app of h but a.
func (h *Hub) others(a *app, fn func(o *app)) {
	h.mu.Lock()
	list := slices.Clone(h.apps)
	h.mu.Unlock()
	for _, o := range list {
		if o != a {
			go o.post(func() { fn(o) })
		}
	}
}

// clipChanged shares a's clipboard with the other windows, and tells a's
// window what Paste would paste.
func (a *app) clipChanged() {
	h := a.hub
	h.mu.Lock()
	h.clip, h.cut = slices.Clone(a.ops.clip), a.ops.cut
	h.mu.Unlock()
	clip, cut := slices.Clone(a.ops.clip), a.ops.cut
	a.publishClip()
	h.others(a, func(o *app) {
		o.ops.clip, o.ops.cut = slices.Clone(clip), cut
		o.publishClip()
	})
}

func (a *app) publishClip() { a.patch(ClipState{Count: len(a.ops.clip), Cut: a.ops.cut}) }

// prefsSaved shares the favourites with the other windows.
func (a *app) prefsSaved() {
	favs, names := slices.Clone(a.prefs.Favourites), cloneNames(a.prefs.FavNames)
	a.hub.others(a, func(o *app) {
		o.prefs.Favourites, o.prefs.FavNames = favs, cloneNames(names)
		o.publishPlaces()
	})
}

func cloneNames(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// touched tells the other windows that j changed folders, so a window
// showing one reads it again at once.
func (a *app) touched(j job) {
	dirs := []string{j.dest}
	for _, s := range j.srcs {
		dirs = append(dirs, filepath.Dir(s))
	}
	if j.undo != nil {
		for _, s := range j.undo.steps {
			dirs = append(dirs, filepath.Dir(s.from), filepath.Dir(s.to))
		}
	}
	a.hub.others(a, func(o *app) {
		if slices.ContainsFunc(dirs, func(d string) bool { return d != "" && samePath(d, o.nav.path) }) {
			o.relist()
		}
	})
}

// openWindow opens another window on dir.
func (a *app) openWindow(dir string) {
	if a.hub.open == nil {
		a.fail("Another window cannot open here.")
		return
	}
	o := a.opts
	o.Dir, o.Select, o.Script = dir, "", ""
	go func() {
		if err := a.hub.open(a.ctx, o); err != nil {
			a.post(func() { a.fail("Opening a new window: " + err.Error()) })
		}
	}()
}

// WindowOptions are the options a Files window opens with.
func WindowOptions() gunim.WindowOptions {
	return gunim.WindowOptions{
		Title:      "Files",
		Size:       geom.Sz(1180, 740),
		Root:       &root{},
		ZoomKeys:   true,
		Icons:      icons(),
		AskToClose: CloseAsked{},
	}
}

// favName is what the sidebar calls the favourite at path: the name the
// user gave it, or the folder's.
func (a *app) favName(path string) string {
	if n := strings.TrimSpace(a.prefs.FavNames[path]); n != "" {
		return n
	}
	return placeName(path)
}
