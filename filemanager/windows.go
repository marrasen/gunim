package filemanager

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// Hub is the Files windows of one process, each served by a program half
// of its own, and what they share: the clipboard of paths, the
// favourites, and word of folders an operation changed. A window opens
// another in its hub, as Ctrl and a click on a folder do.
type Hub struct {
	// ctx ends every window of the hub.
	ctx  context.Context
	mu   sync.Mutex
	apps []*app
	// clips holds the clipboard of the windows on each file system, by
	// its ID, as paths of one mean nothing on another, and favs the
	// favourites of each file system other than the computer's own,
	// for the windows whose options keep them nowhere.
	clips map[string]clipboard
	favs  map[string]*memFavourites
	// open opens a window with o and serves it, and is nil where no
	// window can open, as in a test.
	open func(o Options) error
	// wg counts the windows Open opened that are still open, and errs
	// holds what they ended with.
	wg   sync.WaitGroup
	errs []error
	ga   *gunim.App
}

// NewHub makes a hub whose windows open on ga, and close when ctx ends.
func NewHub(ctx context.Context, ga *gunim.App) *Hub {
	h := &Hub{ctx: ctx, ga: ga}
	h.open = func(o Options) error {
		_, err := h.Open(o)
		return err
	}
	return h
}

// Window is a window of a hub, which its program can watch, close, and
// turn to another file system.
type Window struct {
	c gunim.Client
	// ready closes once the window's program half runs, and done once
	// it has stopped, with err what it stopped with.
	ready, done chan struct{}
	a           *app
	err         error
}

func newWindow(c gunim.Client) *Window {
	return &Window{c: c, ready: make(chan struct{}), done: make(chan struct{})}
}

// Client is the window's client, for what the program does with the
// window itself, such as a screenshot.
func (w *Window) Client() gunim.Client { return w.c }

// Done closes once the window has closed.
func (w *Window) Done() <-chan struct{} { return w.done }

// Err is what the window stopped with, once Done has closed.
func (w *Window) Err() error {
	<-w.done
	return w.err
}

// Close closes the window, once the user agrees to stop what is running
// in it.
func (w *Window) Close() {
	w.do(func(a *app) { a.close() })
}

// Show turns the window to the folder dir on fsys, or its home folder
// when dir is empty, as a Visit may want: the history, the clipboard,
// the places and the favourites become those of fsys. Operations still
// running carry on where they started.
func (w *Window) Show(fsys FS, dir string) {
	w.do(func(a *app) { a.showFS(fsys, dir) })
}

// do runs fn on the window's serve loop, unless it has stopped.
func (w *Window) do(fn func(a *app)) {
	select {
	case <-w.ready:
	case <-w.done:
		return
	}
	w.a.post(func() { fn(w.a) })
}

// Open opens a window with o, and serves it in the background until the
// hub's context ends or the window closes. Wait waits for it to close.
func (h *Hub) Open(o Options) (*Window, error) {
	if h.ga == nil {
		return nil, errors.New("files: the hub has no app to open windows on")
	}
	wo := WindowOptions()
	if o.Name != "" {
		wo.Title = o.Name
	}
	gw, err := h.ga.NewWindow(wo)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	RegisterViews(gw)
	w := newWindow(gw.Client())
	h.wg.Go(func() {
		if err := serveWindow(h.ctx, w, o, h); err != nil {
			h.mu.Lock()
			h.errs = append(h.errs, err)
			h.mu.Unlock()
		}
	})
	return w, nil
}

// Serve serves a window the caller opened, on c, until the hub's context
// ends or the window closes. The window must have the views
// RegisterViews registers, and should open with WindowOptions or options
// like them.
func (h *Hub) Serve(c gunim.Client, o Options) error {
	return serveWindow(h.ctx, newWindow(c), o, h)
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
	h := NewHub(ctx, ga)
	if _, err := h.Open(o); err != nil {
		return err
	}
	return h.Wait()
}

// memFavourites returns the hub's store of the favourites of the file
// system of ID id.
func (h *Hub) memFavourites(id string) *memFavourites {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.favs == nil {
		h.favs = map[string]*memFavourites{}
	}
	m, ok := h.favs[id]
	if !ok {
		m = &memFavourites{}
		h.favs[id] = m
	}
	return m
}

// Refresh has every window of h find its places and read its favourites
// again, as when a caller's places or favourites have changed: a server
// that connected, say.
func (h *Hub) Refresh() {
	h.mu.Lock()
	list := slices.Clone(h.apps)
	h.mu.Unlock()
	for _, a := range list {
		go a.post(func() {
			a.loadFavourites()
			a.loadPlaces()
		})
	}
}

// joinHub adds a to h, or to a hub of its own when h is nil.
func joinHub(a *app, h *Hub) *Hub {
	if h == nil {
		h = &Hub{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.apps = append(h.apps, a)
	clip := h.clips[a.fs.ID()]
	a.ops.clip, a.ops.cut = slices.Clone(clip.paths), clip.cut
	return h
}

// clipboard is the paths cut or copied, and whether they were cut.
type clipboard struct {
	paths []string
	cut   bool
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

// neighbours runs fn on the serve loop of each app of h but a that shows
// the file system a does.
func (h *Hub) neighbours(a *app, fn func(o *app)) {
	id := a.fs.ID()
	h.others(a, func(o *app) {
		if o.fs.ID() == id {
			fn(o)
		}
	})
}

// clipChanged shares a's clipboard with the other windows, and tells a's
// window what Paste would paste.
func (a *app) clipChanged() {
	h := a.hub
	h.mu.Lock()
	if h.clips == nil {
		h.clips = map[string]clipboard{}
	}
	h.clips[a.fs.ID()] = clipboard{paths: slices.Clone(a.ops.clip), cut: a.ops.cut}
	h.mu.Unlock()
	clip, cut := slices.Clone(a.ops.clip), a.ops.cut
	a.publishClip()
	h.neighbours(a, func(o *app) {
		o.ops.clip, o.ops.cut = slices.Clone(clip), cut
		o.publishClip()
	})
}

func (a *app) publishClip() { a.patch(ClipState{Count: len(a.ops.clip), Cut: a.ops.cut}) }

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
		dirs = append(dirs, a.ps.Dir(s))
	}
	if j.undo != nil {
		for _, s := range j.undo.steps {
			dirs = append(dirs, a.ps.Dir(s.from), a.ps.Dir(s.to))
		}
	}
	a.hub.neighbours(a, func(o *app) {
		if slices.ContainsFunc(dirs, func(d string) bool { return d != "" && o.ps.Same(d, o.nav.path) }) {
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
		if err := a.hub.open(o); err != nil {
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
