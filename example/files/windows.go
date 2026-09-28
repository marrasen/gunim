package main

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

// hub is the windows of one process, each served by an app half of its
// own, and what they share: the clipboard of paths, the favourites, and
// word of folders an operation changed.
type hub struct {
	mu   sync.Mutex
	apps []*app
	clip []string
	cut  bool
	// open opens a window on a folder, and is nil where no window can
	// open, as in a test.
	open func(dir string) error
	// wg counts the windows open.
	wg sync.WaitGroup
}

// joinHub adds a to h, or to a hub of its own when h is nil.
func joinHub(a *app, h *hub) *hub {
	if h == nil {
		h = &hub{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apps = append(h.apps, a)
	a.ops.clip, a.ops.cut = slices.Clone(h.clip), h.cut
	return h
}

// leave takes a off h.
func (h *hub) leave(a *app) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apps = slices.DeleteFunc(h.apps, func(o *app) bool { return o == a })
}

// others runs fn on the serve loop of each app of h but a.
func (h *hub) others(a *app, fn func(o *app)) {
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
	go func() {
		if err := a.hub.open(dir); err != nil {
			a.post(func() { a.fail("Opening a new window: " + err.Error()) })
		}
	}()
}

// windowOptions are the options every window of the app opens with.
func windowOptions() gunim.WindowOptions {
	return gunim.WindowOptions{
		Title:      "Files",
		Size:       geom.Sz(1180, 740),
		Root:       &root{},
		ZoomKeys:   true,
		Icons:      icons(),
		AskToClose: CloseAsked{},
	}
}

// serveWindows opens the first window with o, and serves it and every
// window opened from it until they have all closed. first, when set, is
// handed the first window's client.
func serveWindows(ctx context.Context, ga *gunim.App, o options, first func(gunim.Client)) error {
	h := &hub{}
	var mu sync.Mutex
	var errs []error
	h.open = func(dir string) error {
		w, err := ga.NewWindow(windowOptions())
		if err != nil {
			return fmt.Errorf("files: %w", err)
		}
		registerViews(w)
		wo := options{dir: dir, prefsPath: o.prefsPath, noPoll: o.noPoll, trash: o.trash, hub: h}
		h.wg.Go(func() {
			if err := serve(ctx, w.Client(), wo); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
		return nil
	}
	w, err := ga.NewWindow(windowOptions())
	if err != nil {
		return fmt.Errorf("files: %w", err)
	}
	registerViews(w)
	if first != nil {
		first(w.Client())
	}
	o.hub = h
	err = serve(ctx, w.Client(), o)
	h.wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	return errors.Join(append([]error{err}, errs...)...)
}

// favName is what the sidebar calls the favourite at path: the name the
// user gave it, or the folder's.
func (a *app) favName(path string) string {
	if n := strings.TrimSpace(a.prefs.FavNames[path]); n != "" {
		return n
	}
	return placeName(path)
}
