package main

import (
	"context"
	"log"
	"time"

	"github.com/marrasen/gunim"
)

// pollEvery is how often the folder showing is checked for changes.
const pollEvery = 2 * time.Second

// options are what the command line sets.
type options struct {
	dir       string
	prefsPath string
	// noPoll turns off the check for changes, for tests.
	noPoll bool
	// pick is a name to select, and do a command to run, once the first
	// folder is read.
	pick, do string
}

// app is the application half. Everything on it runs on the serve loop;
// work done elsewhere comes back through done.
type app struct {
	ctx   context.Context
	c     gunim.Client
	done  chan func()
	trash trasher

	prefs     prefs
	prefsPath string
	// prefsErr is set when the settings could not be read, so they are
	// not written over.
	prefsErr error
	shell    Shell

	nav     navState
	ops     opsState
	preview previewState
	places  []Place
	banner  int
	// first holds what to do once the first folder is read.
	first    *options
	handlers []handler
}

// handler is one area's share of the intents: it reports whether it
// took in.
type handler func(in gunim.Intent) bool

// serve runs the application half until ctx ends or the window closes.
func serve(ctx context.Context, c gunim.Client, o options) error {
	a, err := newApp(ctx, c, o)
	if err != nil {
		return err
	}
	if err := c.Mount(gunim.Root, browserID, "browser", a.shell); err != nil {
		return err
	}
	a.startup(o)
	var tick <-chan time.Time
	if !o.noPoll {
		t := time.NewTicker(pollEvery)
		defer t.Stop()
		tick = t.C
	}
	handlers := []handler{a.handleNav, a.handleOps, a.handlePreview, a.handlePlaces, a.handleShell}
	a.handlers = handlers
	for {
		select {
		case <-ctx.Done():
			a.stopAll()
			return nil
		case fn := <-a.done:
			fn()
		case <-tick:
			a.poll()
		case ev, ok := <-c.Intents():
			if !ok {
				a.stopAll()
				return c.Err()
			}
			a.handle(handlers, ev.Intent)
		}
	}
}

// newApp makes the application half, with the settings read.
func newApp(ctx context.Context, c gunim.Client, o options) (*app, error) {
	tr, err := systemTrash()
	if err != nil {
		return nil, err
	}
	a := &app{ctx: ctx, c: c, done: make(chan func(), 256), trash: tr, prefsPath: o.prefsPath}
	a.nav.init()
	a.ops.init()
	if a.prefsPath == "" {
		if a.prefsPath, err = defaultPrefsPath(); err != nil {
			return nil, err
		}
	}
	a.prefs, a.prefsErr = loadPrefs(a.prefsPath)
	a.shell = Shell{Light: a.prefs.Light, ShowHidden: a.prefs.ShowHidden, ShowPreview: !a.prefs.HidePreview,
		Sidebar: a.prefs.Sidebar}
	a.nav.sort, a.nav.desc = a.prefs.Sort, a.prefs.Desc
	return a, nil
}

// startup fills the window once the browser is mounted.
func (a *app) startup(o options) {
	if a.prefsErr != nil {
		a.fail(a.prefsErr.Error() + " Settings will not be saved until the file is fixed or removed.")
	}
	if a.prefs.Zoom > 0 {
		a.send(a.c.SetZoom(a.prefs.Zoom))
	}
	a.loadPlaces()
	if o.pick != "" || o.do != "" {
		a.first = &o
	}
	a.startNav(o.dir)
	if a.first != nil {
		a.nav.pick = o.pick
	}
}

// firstRead runs what the command line asked for once the first folder
// is read.
func (a *app) firstRead(handlers []handler) {
	o := a.first
	a.first = nil
	if o.do != "" {
		a.handle(handlers, Command{Name: o.do})
	}
}

func (a *app) handle(handlers []handler, in gunim.Intent) {
	for _, h := range handlers {
		if h(in) {
			return
		}
	}
	switch v := in.(type) {
	case gunim.CommandFailed:
		log.Printf("command %s failed on %q%q: %s", v.Command, v.ID, v.Key, v.Reason)
	case gunim.Zoomed:
		a.prefs.Zoom = v.Zoom
		a.savePrefs()
	}
}

// handleShell takes the intents about the window as a whole.
func (a *app) handleShell(in gunim.Intent) bool {
	switch v := in.(type) {
	case Command:
		switch v.Name {
		case CmdTheme:
			a.shell.Light = !a.shell.Light
			a.prefs.Light = a.shell.Light
		case CmdPreview:
			a.shell.ShowPreview = !a.shell.ShowPreview
			a.prefs.HidePreview = !a.shell.ShowPreview
		case CmdCloseApp:
			a.c.Leave()
			return true
		default:
			return false
		}
		a.publishShell()
		a.savePrefs()
		return true
	case SidebarMoved:
		a.shell.Sidebar = v.Width
		a.prefs.Sidebar = v.Width
		a.savePrefs()
		return true
	}
	return false
}

func (a *app) publishShell() { a.send(a.c.Update(browserID, a.shell)) }

// post runs fn on the serve loop, unless the app has stopped.
func (a *app) post(fn func()) {
	select {
	case a.done <- fn:
	case <-a.ctx.Done():
	}
}

// patch sends a patch to the browser.
func (a *app) patch(v any) { a.send(a.c.Patch(string(browserID), v)) }

// send logs a command the window did not take, which happens once it has
// closed.
func (a *app) send(err error) {
	if err != nil {
		log.Printf("sending to the window: %v", err)
	}
}

// fail shows msg in the banner under the path bar.
func (a *app) fail(msg string) {
	a.banner++
	a.patch(Banner{Seq: a.banner, Text: msg})
}

// savePrefs writes the settings, unless they could not be read.
func (a *app) savePrefs() {
	if a.prefsErr != nil {
		return
	}
	if err := savePrefs(a.prefsPath, a.prefs); err != nil {
		a.fail(err.Error())
	}
}

// stopAll cancels what is running.
func (a *app) stopAll() {
	for _, r := range a.ops.running {
		r.cancel()
	}
	a.nav.stop()
	a.preview.stop()
}
