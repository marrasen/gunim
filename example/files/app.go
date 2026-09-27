package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/marrasen/gunim"
)

// pollEvery is how often the folder showing is checked for changes.
const pollEvery = 2 * time.Second

// options are what the command line sets.
type options struct {
	dir       string
	prefsPath string
	// noPoll turns off the check for changes, and trash stands in for the
	// system's trash, for tests.
	noPoll bool
	trash  trasher
	// pick is a name to select once the first folder is read, and do the
	// steps of a script to run after it; see runScript.
	pick, do string
	// hub is the windows this one opens beside, and nil for a window alone.
	hub *hub
}

// app is the application half. Everything on it runs on the serve loop;
// work done elsewhere comes back through done.
type app struct {
	ctx   context.Context
	c     gunim.Client
	done  chan func()
	trash trasher
	// stopped closes once the serve loop has stopped reading done.
	stopped chan struct{}

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
	// script is what is left of the steps to run, once the first folder
	// is read.
	script    []string
	scripting bool
	handlers  []handler
	// hub is the windows of the process, and dnd what drops need.
	hub *hub
	dnd dndState
}

// handler is one area's share of the intents: it reports whether it
// took in.
type handler func(in gunim.Intent) bool

// serve runs the application half until ctx ends or the window closes.
func serve(ctx context.Context, c gunim.Client, o options) error {
	a, err := launch(ctx, c, o)
	if err != nil {
		return err
	}
	handlers := a.handlers
	var tick <-chan time.Time
	if !o.noPoll {
		t := time.NewTicker(pollEvery)
		defer t.Stop()
		tick = t.C
	}
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

// launch makes the application half and fills the window.
func launch(ctx context.Context, c gunim.Client, o options) (*app, error) {
	a, err := newApp(ctx, c, o)
	if err != nil {
		return nil, err
	}
	if err := c.Mount(gunim.Root, browserID, "browser", a.shell); err != nil {
		return nil, err
	}
	a.hub = joinHub(a, o.hub)
	a.publishClip()
	a.handlers = []handler{a.handleDnd, a.handleNav, a.handleOps, a.handlePreview, a.handlePlaces, a.handleShell}
	a.startup(o)
	return a, nil
}

// newApp makes the application half, with the settings read.
func newApp(ctx context.Context, c gunim.Client, o options) (*app, error) {
	tr := o.trash
	if tr == nil {
		var err error
		if tr, err = systemTrash(); err != nil {
			return nil, err
		}
	}
	var err error
	a := &app{ctx: ctx, c: c, done: make(chan func(), 256), stopped: make(chan struct{}), trash: tr,
		prefsPath: o.prefsPath}
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
	if o.do != "" {
		a.script = strings.Split(o.do, ",")
	}
	a.startNav(o.dir)
	a.nav.pick = o.pick
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
			a.close()
			return true
		default:
			return false
		}
		a.publishShell()
		a.savePrefs()
		return true
	case CloseAsked:
		a.close()
		return true
	case SidebarMoved:
		a.shell.Sidebar = v.Width
		a.prefs.Sidebar = v.Width
		a.savePrefs()
		return true
	}
	return false
}

// close closes the window, once the user agrees to stop what is running.
func (a *app) close() {
	n := len(a.ops.running)
	if n == 0 {
		a.c.Leave()
		return
	}
	a.confirm(Confirm{Title: "Stop " + plural(n, "operation") + " and close?",
		Body: "What is running stops where it has got to. Anything half copied is taken away.", OK: "Stop and close"},
		func() {
			for _, r := range a.ops.running {
				r.cancel()
			}
			a.c.Leave()
		})
}

func (a *app) publishShell() { a.send(a.c.Update(browserID, a.shell)) }

// post runs fn on the serve loop, unless the app has stopped.
func (a *app) post(fn func()) {
	select {
	case a.done <- fn:
	case <-a.ctx.Done():
	case <-a.stopped:
	}
}

// patch sends a patch to the browser.
func (a *app) patch(v any) { a.send(a.c.Patch(string(browserID), v)) }

// send logs a command the window did not take. A window that has closed
// takes nothing, and needs no word about it.
func (a *app) send(err error) {
	if err != nil && !errors.Is(err, gunim.ErrWindowClosed) {
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
		return
	}
	a.prefsSaved()
}

// stopAll cancels what is running, and waits for the operations to stop,
// so none is left writing once the app has gone.
func (a *app) stopAll() {
	for _, r := range a.ops.running {
		r.cancel()
	}
	a.nav.stop()
	a.preview.stop()
	a.hub.leave(a)
	close(a.stopped)
	a.ops.wg.Wait()
}
