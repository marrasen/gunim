package filemanager

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
)

// pollEvery is how often the folder showing is checked for changes.
const pollEvery = 2 * time.Second

// Options set up the program half of one window.
type Options struct {
	// FS is the file system the window shows, and the computer's own,
	// as LocalFS returns it, when nil.
	FS FS
	// Dir is the folder the window opens on, and the file system's home
	// folder when empty.
	Dir string
	// PrefsPath is the file the settings are kept in, and
	// gunim-files/prefs.json in the user's configuration folder when
	// empty.
	PrefsPath string
	// Poll is how often the folder showing is checked for changes: every
	// two seconds when zero, and never when below zero.
	Poll time.Duration
	// Select is a name to select once the first folder is read, and
	// Script the steps of a script to run after that, split by commas,
	// such as copy,into:Documents,paste. Script is for demonstrations
	// and tests; see runScript for the steps.
	Select, Script string

	// trash stands in for the file system's trash, for tests.
	trash Trasher
}

// app is the application half. Everything on it runs on the serve loop;
// work done elsewhere comes back through done.
type app struct {
	ctx context.Context
	c   gunim.Client
	// mods are the modifier keys held as the intent being handled was sent.
	mods input.Mods
	done chan func()
	// fs is the file system the window shows, ps how it writes paths,
	// and trash its trash, or nil where it has none.
	fs    FS
	ps    PathStyle
	trash Trasher
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
	thumbs  thumbState
	viewer  viewerState
	search  searchState
	places  []Place
	banner  int
	// script is what is left of the steps to run, once the first folder
	// is read.
	script    []string
	scripting bool
	handlers  []handler
	// opts are the options the window opened with, which a window it
	// opens takes too.
	opts Options
	// hub is the windows of the process, and dnd what drops need.
	hub *Hub
	dnd dndState
}

// handler is one area's share of the intents: it reports whether it
// took in.
type handler func(in gunim.Intent) bool

// serveWindow runs the application half, in hub h, until ctx ends or the
// window closes.
func serveWindow(ctx context.Context, c gunim.Client, o Options, h *Hub) error {
	a, err := launch(ctx, c, o, h)
	if err != nil {
		return err
	}
	handlers := a.handlers
	var tick <-chan time.Time
	if o.Poll >= 0 {
		every := o.Poll
		if every == 0 {
			every = pollEvery
		}
		t := time.NewTicker(every)
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
			a.take(handlers, ev)
		}
	}
}

// launch makes the application half and fills the window. A nil h puts
// the window in a hub of its own.
func launch(ctx context.Context, c gunim.Client, o Options, h *Hub) (*app, error) {
	a, err := newApp(ctx, c, o)
	if err != nil {
		return nil, err
	}
	if err := c.Mount(gunim.Root, browserID, "browser", a.shell); err != nil {
		return nil, err
	}
	a.hub = joinHub(a, h)
	a.publishClip()
	a.handlers = []handler{a.handleDnd, a.handleNav, a.handleOps, a.handlePreview, a.handlePlaces, a.handleShell,
		a.handleIcons, a.handleViewer, a.handleSearch, a.handleTheme}
	a.startup(o)
	return a, nil
}

// newApp makes the application half, with the settings read.
func newApp(ctx context.Context, c gunim.Client, o Options) (*app, error) {
	if o.FS == nil {
		o.FS = LocalFS()
	}
	tr := o.trash
	if tr == nil {
		tr, _ = o.FS.(Trasher)
	}
	var err error
	a := &app{ctx: ctx, c: c, done: make(chan func(), 256), stopped: make(chan struct{}), fs: o.FS, ps: o.FS.Paths(),
		trash: tr, prefsPath: o.PrefsPath, opts: o}
	a.nav.init()
	a.ops.init()
	a.thumbs.init()
	if a.prefsPath == "" {
		if a.prefsPath, err = defaultPrefsPath(); err != nil {
			return nil, err
		}
	}
	a.prefs, a.prefsErr = loadPrefs(a.prefsPath)
	a.shell = Shell{Light: a.prefs.Light, ShowHidden: a.prefs.ShowHidden, ShowPreview: !a.prefs.HidePreview,
		Sidebar: a.prefs.Sidebar, FS: a.fs.ID(), Paths: a.ps}
	a.nav.sort, a.nav.desc = a.prefs.Sort, a.prefs.Desc
	return a, nil
}

// startup fills the window once the browser is mounted.
func (a *app) startup(o Options) {
	if a.prefsErr != nil {
		a.fail(a.prefsErr.Error() + " Settings will not be saved until the file is fixed or removed.")
	}
	if a.prefs.Zoom > 0 {
		a.send(a.c.SetZoom(a.prefs.Zoom))
	}
	a.loadPlaces()
	if o.Script != "" {
		a.script = strings.Split(o.Script, ",")
	}
	a.startNav(o.Dir)
	a.nav.pick = o.Select
}

// take handles the intent ev carries, with the modifier keys held as it was sent.
func (a *app) take(handlers []handler, ev gunim.Envelope) {
	a.mods = ev.Mods
	defer func() { a.mods = 0 }()
	a.handle(handlers, ev.Intent)
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
	a.viewer.stop()
	a.search.stop()
	a.hub.leave(a)
	close(a.stopped)
	a.ops.wg.Wait()
}
