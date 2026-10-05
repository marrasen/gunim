package install

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"runtime"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The installer's own tokens.
var (
	// backdrop is the window's dark, under the accent's wash.
	backdrop = theme.Color("install.backdrop", color.NRGBA{R: 0x12, G: 0x14, B: 0x1a, A: 0xff})
	// inkSoft is the colour of what the installer says in passing.
	inkSoft = theme.Color("install.ink.soft", color.NRGBA{R: 0xa4, G: 0xab, B: 0xbb, A: 0xff})
	// titleSize is the size of the page's heading.
	titleSize = theme.Length("install.title.size", 26)
	// smallSize is the size of the line that says where and how much.
	smallSize = theme.Length("install.small.size", 12.5)
	// doneInk is the tick's badge, and failInk the ring at a failure.
	doneInk = theme.Color("install.done", color.NRGBA{R: 0x34, G: 0xc7, B: 0x6f, A: 0xff})
	failInk = theme.Color("install.fail", color.NRGBA{R: 0xe5, G: 0x5a, B: 0x52, A: 0xff})
	// monogramSize and monogramInk are the letter on the tile that
	// stands in for a missing icon.
	monogramSize = theme.Length("install.monogram.size", 64)
	monogramInk  = theme.Color("install.monogram.ink", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	// detailIndent puts an offer's detail under its label, clear of the
	// box.
	detailIndent = theme.Insets("install.detail.indent", geom.Insets{Left: 30})
)

// The pages of the installer.
const (
	pageWelcome  = "welcome" // what to install and how
	pageRunning  = "running" // a copy runs that must end first
	pageClosing  = "closing" // the copies running were asked to end
	pageWorking  = "working" // installing or uninstalling
	pageDone     = "done"    // installed
	pageFailed   = "failed"  // it went wrong
	pageRemove   = "remove"  // uninstall?
	pageRemoved  = "removed" // uninstalled
	defaultWidth = 640
)

// scene is what the installer's window shows. It is the view's state,
// all values, so the window and the work need share nothing else.
type scene struct {
	Name, Version, Publisher, Description string
	// Have is the version installed already, Where the folder the
	// program goes in, and Room how much it needs and how much is free.
	Have, Where, Room string
	// NoRoom says there is not room enough.
	NoRoom bool
	Mode   Mode
	Page   string
	Offers []Offer
	// Step is what the work does now, and Progress how far it got.
	Step     string
	Progress float32
	// Problem is what went wrong.
	Problem string
	// Data says there is data to offer to remove, Quit that the
	// program can be asked to end, and Running how many copies run.
	Data, Quit bool
	Running    int
	// DoneWords and RemovedWords are what to say at the end.
	DoneWords, RemovedWords, Welcome string
	// System is the system's name in Go, for saying where the program
	// is found.
	System string
	// Removing says the failure or the running copies came of an
	// uninstall.
	Removing bool
}

// What the installer's window sends.
type (
	// started asks to install, with the offers as ticked.
	started struct{ Picks map[string]bool }
	// opened asks to open the installed program.
	opened struct{}
	// ranHere asks to run this copy as it is.
	ranHere struct{}
	// closed asks to close the window.
	closed struct{}
	// retried asks to go back after a failure.
	retried struct{}
	// quitThem asks the copies running to end.
	quitThem struct{}
	// removed asks to uninstall, taking the user's data too with Data.
	removed struct{ Data bool }
)

func init() {
	gunim.RegisterType[scene]("gunim.install.scene")
	gunim.RegisterType[started]("gunim.install.started")
	gunim.RegisterType[opened]("gunim.install.opened")
	gunim.RegisterType[ranHere]("gunim.install.ranHere")
	gunim.RegisterType[closed]("gunim.install.closed")
	gunim.RegisterType[retried]("gunim.install.retried")
	gunim.RegisterType[quitThem]("gunim.install.quitThem")
	gunim.RegisterType[removed]("gunim.install.removed")
}

// installTheme is the installer's theme: dark, its accent and its
// primary buttons in the program's colour.
func installTheme(accent color.NRGBA) theme.Theme {
	h, s, _ := hsv(accent.R, accent.G, accent.B)
	fill := fromHSV(h, min(s, 0.8), 0.68)
	hover := fromHSV(h, min(s, 0.75), 0.78)
	return widget.Dark().With(
		theme.Set(widget.Accent, accent),
		theme.Set(widget.ButtonPrimaryFill, fill),
		theme.Set(widget.ButtonPrimaryHover, hover),
		theme.Set(widget.CardFill, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0c}),
		theme.Set(widget.ButtonHeight, 38),
		theme.Set(widget.ButtonRadius, 10),
	)
}

// accentFor is the colour the installer moves in: as the application
// set it, or the icon's most vivid, or a blue.
func accentFor(a *App) color.NRGBA {
	if a.Look.Accent.A != 0 {
		return a.Look.Accent
	}
	if c, ok := accentOf(a.Icon); ok {
		return c
	}
	return color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff}
}

// window runs the installer's window on s until it closes.
func window(ctx context.Context, app *gunim.App, s *Session) error {
	r, err := openWindow(app, s)
	if err != nil {
		return err
	}
	return r.serve(ctx)
}

// openWindow opens the installer's window on s, showing its first page,
// and returns its application half, ready to serve.
func openWindow(app *gunim.App, s *Session) (*runner, error) {
	a := &s.App
	accent := accentFor(a)
	var img *paint.Image
	var icons []image.Image
	if a.Icon != nil {
		b := a.Icon.Bounds()
		img = paint.NewImage(square(a.Icon, min(512, max(b.Dx(), b.Dy(), 128))))
		icons = []image.Image{square(a.Icon, 256), square(a.Icon, 64), square(a.Icon, 32)}
	}
	size := geom.Sz(defaultWidth, 600)
	if a.Look.Width > 0 {
		size.W = a.Look.Width
	}
	if a.Look.Height > 0 {
		size.H = a.Look.Height
	}
	title := "Install " + a.Name
	if s.Mode == Remove {
		title = "Remove " + a.Name
	}
	// A plain root, not a Surface: the backdrop runs up under the title
	// bar, and the stage keeps its content clear of it. The window opens
	// in the middle of the screen the pointer is on, where the user just
	// started the program, and has only the button that closes it.
	bar := widget.NewTitleBar(title)
	bar.NoMinimize, bar.NoMaximize = true, true
	o := gunim.WindowOptions{Title: title, Size: size, Icons: icons, TitleBar: bar,
		UnderTitleBar: true, AskToClose: closed{}}
	if m, ok := app.PointerMonitor(); ok {
		o.Monitor = &m
	}
	w, err := app.NewWindow(o)
	if err != nil {
		return nil, err
	}
	return attach(w, s, img, accent)
}

// attach registers the installer's views with w, shows s's first page
// there, and returns the application half, ready to serve.
func attach(w *gunim.Window, s *Session, img *paint.Image, accent color.NRGBA) (*runner, error) {
	th := installTheme(accent)
	if s.App.Look.Theme != nil {
		th = *s.App.Look.Theme
	}
	w.RegisterTheme(th)
	gunim.RegisterView(w, "gunim.install", func(sc scene) *stage {
		st := newStage(img, sc.Name, accent)
		st.page = buildPage(sc)
		return st
	}, updateStage)
	c := w.Client()
	_ = c.SetTheme(th.Name)
	r := &runner{s: s, c: c, sc: firstScene(s), events: make(chan func(), 16)}
	if err := c.Mount(gunim.Root, "installer", "gunim.install", r.sc); err != nil {
		return nil, err
	}
	return r, nil
}

// firstScene is what the window opens on.
func firstScene(s *Session) scene {
	a := &s.App
	sc := scene{
		Name: a.Name, Version: strings.TrimPrefix(a.Version, "v"), Publisher: a.Publisher, Description: a.Description,
		Where: s.Dir, Mode: s.Mode, Page: pageWelcome, Offers: s.Offers, System: runtime.GOOS,
		Data: len(a.Data) > 0, Quit: a.Quit != nil,
		DoneWords: a.Words.Done, RemovedWords: a.Words.Removed, Welcome: a.Words.Welcome,
	}
	if s.Have != nil {
		sc.Have = strings.TrimPrefix(s.Have.Version, "v")
	}
	if s.Mode == Remove {
		sc.Page = pageRemove
	}
	if s.Free >= 0 {
		sc.Room = fmt.Sprintf("Needs %s · %s free", Bytes(s.Need), Bytes(s.Free))
		sc.NoRoom = s.Room() != nil
	} else {
		sc.Room = "Needs " + Bytes(s.Need)
	}
	return sc
}

// runner is the installer's application half: it hears the window,
// does the work, and shows how it goes.
type runner struct {
	s  *Session
	c  gunim.Client
	sc scene
	// working says the work runs, which the window may not close on,
	// and quitting that the copies running were asked to end and have
	// not yet. Neither takes another press; Cancel still closes the
	// window while the copies are closing. watching says a look for the
	// copies to have ended runs.
	working, quitting, watching bool
	// running lists the copies of the program running; a test sets it.
	running func() []int
	// ctx is serve's, which the looks end with.
	ctx context.Context
	// then is what to do once no copy runs: install with these picks,
	// or uninstall.
	then func()
	// events are the work's reports, run on serve's goroutine.
	events chan func()
}

// leastWork is the shortest the work shows for, so the ring is seen to
// go round however quick the copying is.
var leastWork = 1600 * time.Millisecond

func (r *runner) show() { _ = r.c.Update("installer", r.sc) }

func (r *runner) serve(ctx context.Context) error {
	r.ctx = ctx
	for {
		select {
		case <-ctx.Done():
			return nil
		case fn := <-r.events:
			fn()
		case ev, ok := <-r.c.Intents():
			if !ok {
				return nil
			}
			r.handle(ctx, ev.Intent)
		}
	}
}

func (r *runner) handle(ctx context.Context, in gunim.Intent) {
	if r.ctx == nil {
		r.ctx = ctx
	}
	if _, cancel := in.(closed); r.working || r.quitting && !cancel {
		// A second press of Install would start the work twice.
		return
	}
	switch in := in.(type) {
	case started:
		r.then = func() { r.install(ctx, in.Picks) }
		r.whenFree()
	case removed:
		r.then = func() { r.uninstall(ctx, in.Data) }
		r.sc.Removing = true
		r.whenFree()
	case opened:
		r.s.Open()
		r.c.Leave()
	case ranHere:
		r.s.RunHere()
		r.c.Leave()
	case closed:
		r.c.Leave()
	case retried:
		r.sc.Page, r.sc.Problem = pageWelcome, ""
		if r.sc.Removing {
			r.sc.Page = pageRemove
		}
		r.show()
	case quitThem:
		quit := r.s.App.Quit
		if quit == nil {
			return
		}
		r.quitting = true
		r.sc.Page, r.sc.Problem = pageClosing, ""
		r.show()
		r.watch()
		go func() {
			err := quit(ctx)
			if err == nil {
				// The watch goes on as soon as they have gone; past this,
				// they are not going by themselves.
				select {
				case <-time.After(closeWait):
				case <-ctx.Done():
					return
				}
			}
			r.send(func() {
				if r.sc.Page != pageClosing {
					// They went, and the work is on.
					return
				}
				r.quitting = false
				r.sc.Page, r.sc.Problem = pageRunning, "It didn't close."
				if err != nil {
					r.sc.Problem = "It didn't close: " + err.Error()
				}
				r.show()
			})
		}()
	}
}

// closeWait is how long the copies asked to end are given to go before
// the installer says they did not.
var closeWait = 15 * time.Second

// watchEvery is how often the installer looks whether the copies it
// waits for have ended.
var watchEvery = 400 * time.Millisecond

// send runs fn on serve's goroutine, unless the window has closed.
func (r *runner) send(fn func()) {
	select {
	case r.events <- fn:
	case <-r.ctx.Done():
	}
}

// runningNow lists the copies of the program running now.
func (r *runner) runningNow() []int {
	if r.running != nil {
		return r.running()
	}
	return r.s.Running()
}

// watch looks for the copies running to have ended, and goes on with
// what was asked once they have, so the user only closes them.
func (r *runner) watch() {
	if r.watching {
		return
	}
	r.watching = true
	go func() {
		t := time.NewTicker(watchEvery)
		defer t.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-t.C:
			}
			if len(r.runningNow()) == 0 {
				r.send(func() {
					r.watching = false
					if r.sc.Page == pageRunning || r.sc.Page == pageClosing {
						r.whenFree()
					}
				})
				return
			}
		}
	}()
}

// whenFree goes on with what was asked once no copy of the program
// runs, or says one does. A fresh install has none to wait for.
func (r *runner) whenFree() {
	if r.s.Mode != Fresh || r.sc.Removing {
		if n := len(r.runningNow()); n > 0 {
			if r.sc.Page != pageClosing {
				r.sc.Page = pageRunning
			}
			r.sc.Running = n
			r.show()
			r.watch()
			return
		}
	}
	r.quitting = false
	if r.then != nil {
		then := r.then
		r.then = nil
		then()
	}
}

// install installs with picks, on a goroutine, showing the ring go
// round.
func (r *runner) install(ctx context.Context, picks map[string]bool) {
	r.work(func(report func(Progress)) error {
		_, err := r.s.Install(ctx, picks, report)
		return err
	}, pageDone)
}

// uninstall uninstalls, taking the user's data too with data.
func (r *runner) uninstall(ctx context.Context, data bool) {
	r.work(func(report func(Progress)) error {
		return r.s.Uninstall(ctx, data, report)
	}, pageRemoved)
}

// work runs do on a goroutine, reporting its progress to the window,
// and shows done when it has.
func (r *runner) work(do func(func(Progress)) error, done string) {
	r.working = true
	r.sc.Page, r.sc.Step, r.sc.Progress, r.sc.Problem = pageWorking, "Starting", 0, ""
	r.show()
	go func() {
		start := time.Now()
		err := do(func(p Progress) {
			r.events <- func() {
				r.sc.Step, r.sc.Progress = p.Step, p.Done
				r.show()
			}
		})
		if wait := leastWork - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
		r.events <- func() {
			r.working = false
			if err != nil {
				r.failed(err)
				return
			}
			r.sc.Page, r.sc.Progress = done, 1
			r.show()
		}
	}()
}

// failed shows err.
func (r *runner) failed(err error) {
	r.sc.Page, r.sc.Problem = pageFailed, err.Error()
	r.show()
}

// updateStage shows sc: the page it is on, and the icon's part in it.
func updateStage(st *stage, sc scene, u *gunim.UI) {
	if st.page == nil || st.page.name != sc.Page {
		was := ""
		if st.page != nil {
			was = st.page.name
		}
		st.show(buildPage(sc), u)
		if was == pageClosing && sc.Page != pageClosing {
			st.spin(false, u)
		}
		switch sc.Page {
		case pageRunning:
			if was != "" {
				st.back(u)
			}
		case pageClosing:
			st.spin(true, u)
		case pageWorking:
			st.work(u)
		case pageDone:
			st.finish(u)
		case pageRemoved:
			st.removed(u)
		case pageFailed:
			st.fail(u)
		case pageWelcome, pageRemove:
			if was != "" {
				st.back(u)
			}
		}
	}
	if sc.Page == pageWorking {
		st.progress(sc.Progress, u)
		// The work's last step says it is done; the page under the ring
		// keeps the step before until the next page comes.
		if l, ok := st.page.step.(*widget.Label); ok && sc.Step != "Done" && l.Text != sc.Step+"…" {
			l.SetText(sc.Step + "…")
		}
	}
	u.Invalidate()
}

// buildPage makes the page sc is on.
func buildPage(sc scene) *page {
	p := newPage(sc.Page, 500)
	switch sc.Page {
	case pageWelcome:
		welcomePage(p, sc)
	case pageRunning:
		heading(p, sc.Name+" is running", "")
		line := "Close it, and this goes on by itself."
		if sc.Running > 1 {
			line = fmt.Sprintf("%d copies of it are open. Close them, and this goes on by itself.", sc.Running)
		}
		say(p, line)
		if sc.Problem != "" {
			problem := soft(sc.Problem)
			problem.Color = failInk
			p.add(8, problem)
		}
		var buttons []gunim.Node
		if sc.Quit {
			q := widget.NewButton("Close " + sc.Name)
			q.Kind = widget.ButtonPrimary
			q.On = quitThem{}
			buttons = append(buttons, q)
		}
		p.foot = footer(cancel(), buttons...)
	case pageClosing:
		heading(p, "Closing "+sc.Name+"…", "")
		say(p, "This goes on by itself once it has closed.")
		p.foot = footer(cancel())
	case pageWorking:
		title := "Installing"
		switch {
		case sc.Removing:
			title = "Removing"
		case sc.Mode == Upgrade:
			title = "Updating"
		}
		heading(p, title+" "+sc.Name, "")
		step := soft(sc.Step + "…")
		p.add(8, step)
		p.step = step
	case pageDone:
		done := sc.DoneWords
		if done == "" {
			done = sc.Name + " is installed"
			if sc.Mode == Upgrade {
				done = sc.Name + " is up to date"
			}
		}
		heading(p, done, "")
		where := "Find it among your applications."
		if sc.System == "windows" {
			where = "Find it in the Start menu."
		}
		say(p, where)
		open := widget.NewButton("Open " + sc.Name)
		open.Kind = widget.ButtonPrimary
		open.On = opened{}
		p.foot = footer(closeButton("Close"), open)
	case pageFailed:
		title := "Couldn't install " + sc.Name
		if sc.Removing {
			title = "Couldn't remove " + sc.Name
		}
		heading(p, title, "")
		problem := say(p, sc.Problem)
		problem.Selectable = true
		again := widget.NewButton("Try Again")
		again.Kind = widget.ButtonPrimary
		again.On = retried{}
		p.foot = footer(closeButton("Close"), again)
	case pageRemove:
		heading(p, "Remove "+sc.Name+"?", versionLine(sc))
		say(p, "The program, its shortcuts and the system's entries for it go.")
		var data *widget.Checkbox
		if sc.Data {
			data = widget.NewCheckbox("Also remove your settings and data")
			p.add(18, center(data))
		}
		remove := widget.NewButton("Remove")
		remove.Kind = widget.ButtonDanger
		remove.OnActivate(func(u *gunim.UI) {
			u.Send(remove, removed{Data: data != nil && data.On})
		})
		p.foot = footer(cancel(), remove)
	case pageRemoved:
		words := sc.RemovedWords
		if words == "" {
			words = sc.Name + " is removed"
		}
		heading(p, words, "")
		say(p, "Thank you for using it.")
		p.foot = footer(nil, closeButton("Close"))
	}
	return p
}

// welcomePage is the page that says what is about to happen and makes
// the offers.
func welcomePage(p *page, sc scene) {
	title, sub, act := sc.Name, versionLine(sc), "Install"
	switch sc.Mode {
	case Upgrade:
		title, act = "Update "+sc.Name, "Update"
		sub = "From " + sc.Have + " to " + sc.Version
	case Reinstall:
		sub = sc.Version + " is installed already"
		act = "Reinstall"
	case Downgrade:
		sub = "A newer version, " + sc.Have + ", is installed"
		act = "Install " + sc.Version
	}
	heading(p, title, sub)
	if sc.Description != "" {
		say(p, sc.Description)
	}
	var boxes []*widget.Checkbox
	var keys []string
	if len(sc.Offers) > 0 {
		var rows []gunim.Node
		for _, o := range sc.Offers {
			box := widget.NewCheckbox(o.Label)
			box.On = o.On
			boxes = append(boxes, box)
			keys = append(keys, o.Key)
			rows = append(rows, box)
			if o.Detail != "" {
				d := soft(o.Detail)
				d.Size = smallSize
				d.Align = text.AlignStart
				pad := widget.NewPad(d)
				pad.Padding = detailIndent
				rows = append(rows, pad)
			}
		}
		col := widget.Column(rows...)
		col.Cross = widget.CrossStretch
		p.add(22, widget.NewCard(col))
	}
	welcome := sc.Welcome
	if welcome == "" {
		welcome = "For you alone, in " + sc.Where + ". No administrator needed."
	}
	w := soft(welcome)
	w.Size = smallSize
	p.add(14, w)

	room := soft(sc.Room)
	room.Size = smallSize
	room.Align = text.AlignStart
	if sc.NoRoom {
		room.Color = failInk
	}
	pick := func() map[string]bool {
		picks := map[string]bool{}
		for i, b := range boxes {
			picks[keys[i]] = b.On
		}
		return picks
	}
	here := widget.NewLink("Or run it without installing")
	here.Size = smallSize
	here.On = ranHere{}
	p.add(6, center(here))
	var buttons []gunim.Node
	if sc.Mode == Reinstall || sc.Mode == Downgrade {
		again := widget.NewButton(act)
		again.OnActivate(func(u *gunim.UI) { u.Send(again, started{Picks: pick()}) })
		again.Disabled = sc.NoRoom
		open := widget.NewButton("Open")
		if sc.Mode == Downgrade {
			open.SetLabel("Open " + sc.Have)
		}
		open.Kind = widget.ButtonPrimary
		open.On = opened{}
		buttons = append(buttons, again, open)
	} else {
		do := widget.NewButton(act)
		do.Kind = widget.ButtonPrimary
		do.Disabled = sc.NoRoom
		do.OnActivate(func(u *gunim.UI) { u.Send(do, started{Picks: pick()}) })
		buttons = append(buttons, do)
	}
	p.foot = footer(room, buttons...)
}

// versionLine is the version and who made it.
func versionLine(sc scene) string {
	line := ""
	if sc.Version != "" {
		line = "Version " + sc.Version
	}
	if sc.Publisher != "" {
		if line != "" {
			line += " · "
		}
		line += sc.Publisher
	}
	return line
}

// heading adds the page's title, and sub under it.
func heading(p *page, title, sub string) {
	t := widget.NewLabel(title)
	t.Face = widget.BoldFont
	t.Size = titleSize
	t.Align = text.AlignCenter
	p.add(0, t)
	if sub != "" {
		p.add(6, soft(sub))
	}
}

// say adds a line of text, centred.
func say(p *page, s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Align = text.AlignCenter
	p.add(12, l)
	return l
}

// soft is a label in the soft ink, centred.
func soft(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Color = inkSoft
	l.Align = text.AlignCenter
	return l
}

// center holds n in the middle of its row.
func center(n gunim.Node) gunim.Node {
	r := widget.Row(n)
	r.Justify = widget.JustifyCenter
	return r
}

// footer is the row of buttons along the bottom: lead at the left, and
// the buttons at the right, the last the one the page expects.
func footer(lead gunim.Node, buttons ...gunim.Node) gunim.Node {
	spacer := widget.NewSpacer()
	kids := []gunim.Node{}
	if lead != nil {
		kids = append(kids, lead)
	}
	kids = append(kids, spacer)
	kids = append(kids, buttons...)
	r := widget.Row(kids...).Grow(spacer, 1)
	r.Cross = widget.CrossCenter
	return r
}

// cancel is the button that closes the window having done nothing.
func cancel() gunim.Node { return closeButton("Cancel") }

func closeButton(label string) *widget.Button {
	b := widget.NewButton(label)
	b.On = closed{}
	return b
}
