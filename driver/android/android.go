//go:build android

// Package android is gunim's driver for Android.
//
// An Android application is a Java activity, and gunim's Go code runs
// inside it as a library, libgunim.so. The activity's Java half, in the
// java directory, owns one SurfaceView. It hands Go the surface, the
// touches, the keys, and the soft keyboard's edits. Go draws every
// gunim window into that one surface.
//
// The activity starts the program's main function on a goroutine of
// its own, once. main calls gunim.Main as on the desktop, and Main
// opens this driver.
//
// # With no activity
//
// Android may run a program for a service, a job or a receiver, with no
// activity, as after it ended the program's process in the middle of a
// job. Such a component, in Java of the program's own, calls
// gunim.android.Native.boot, which starts main the same way. gunim.Main
// runs its function at once then, and App.NewWindow and App.Monitors wait
// for an activity to show the window, which may never come. So a program
// that works in the background starts that work before it opens its
// window, or apart from it. Once an activity starts in the process, the
// window opens there, and the work goes on.
//
// # The end
//
// main runs once in a process. The activity finishes as gunim.Main does,
// when the program's last window closes or its context ends, and no
// window opens after that: App.NewWindow returns an error, as it does
// where it was waiting for an activity that never came. Then main may go
// on, with work the background still has.
//
// As main returns, the process ends, once no activity and no service of
// the program's is left in it, as a job's: Android may start the next job
// in a process it has, where main has run already. Goroutines that main
// leaves run until then. A file the program shared by its path is served
// by another process, which stays. One that the program serves itself, a
// [driver.Served], keeps the process for as long as another application
// has it open, stopped or not, and main's goroutines read for it. Where something comes
// too late, as below, it stops first: a reader hears of an error, and has
// 5 s to let the file go, as Android may end it with the process. A
// program that serves files stays in main, with its window, for as long as
// they are open. Android counts neither a provider of the program's
// own nor a receiver that works on past its onReceive, so a program with
// one of those stays in main, or gives it a process of its own in the
// manifest.
//
// What comes in between does not get the main that has run. An activity,
// or a component that calls boot, that comes after main has returned ends
// the process, once no activity of the program's own shows. Android starts
// an activity again at once in a new one, a job too, after a while, and a
// service where that asked to be. It sends no broadcast again. One that comes
// after gunim.Main has ended, and before main has, waits for main to
// return, and then gets its new process the same way; an activity shows
// nothing meanwhile. So a program with much work left as the user is done
// with its window does best to leave the window open. The user leaves it
// with Home, and Android lets a job's work go on.
//
// # Windows on one surface
//
// Android gives an activity one surface, and gunim opens several
// windows: menus, tooltips and lists are popups of their own. So the
// driver keeps the windows in a stack, bottom to top. Each draws into a
// texture of its own with the shared renderer, and each frame the
// render thread lays the textures over each other on the surface and
// swaps once. A touch goes to the topmost window under it.
//
// # The keyboard
//
// The soft keyboard asks the text it edits questions, and wants the
// answers at once on the UI thread. So Java keeps a copy of the focused
// text, an Android Editable, which Android's own BaseInputConnection
// edits as the keyboard asks. Java sends each change as an
// [input.TextEdit], and the engine sends each new state of the text
// back through [driver.TextStater]; see GunimInput.java.
package android

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"
	"time"
	_ "unsafe" // for go:linkname

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/compose"
	"github.com/marrasen/gunim/driver/internal/touch"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

/*
#include "glue.h"
*/
import "C"

// mainMain is the program's main function, which a library's loader
// leaves uncalled.
//
//go:linkname mainMain main.main
func mainMain()

// theDriver is the one driver: Java's calls reach it with no other way
// to find it.
var theDriver = newDriver()

// errNotStarted is returned by Open before Java has started gunim.
var errNotStarted = errors.New("android: nothing started gunim")

// errOver is returned for a window the program asks for once Run has
// ended.
var errOver = errors.New("android: the program is done with its windows")

// Open returns the driver. The activity, or another component, has
// started it by the time main runs. It gives the driver for one run:
// there is none after that, as the activity is gone.
func Open() (*Driver, error) {
	d := theDriver
	d.mu.Lock()
	started := d.started
	d.mu.Unlock()
	if !started {
		return nil, errNotStarted
	}
	if d.isOver() {
		return nil, errOver
	}
	return d, nil
}

// Driver is the Android implementation of [driver.Driver].
type Driver struct {
	mu      sync.Mutex
	started bool
	// sized is closed once the first surface has a size, which Java
	// sends after the density and the refresh rate. A window opened
	// before then waits for it.
	sized   chan struct{}
	density float32
	rate    float64
	// surfW and surfH are the surface's size in device pixels.
	surfW, surfH int
	// windows is the stack of open windows, bottom first, and gone the
	// panes of windows closed, for the render thread to free.
	windows []*Window
	gone    []*compose.Pane
	// opened holds the files handed over before a window opened to take
	// them, one batch to an entry.
	opened []any
	// gest is the touch in progress; see gestures.
	gest touch.Gestures
	// fingersOn is what the view last heard from takeFingers.
	fingersOn bool
	// typing is the window the keyboard types into: the one that turned
	// text input on last.
	typing *Window
	// editSeq is the seq of the keyboard's latest edit.
	editSeq uint64
	// keyboard is how much of the surface the soft keyboard covers, in
	// device pixels from the bottom, as Java reports it frame by frame
	// while the keyboard slides. caret is the typing window's text
	// caret in logical pixels of the screen, when caretSet, and box the
	// bounds of the text node it is in, when boxSet. keyed says a
	// key or an edit has gone to the windows since the last touch, so
	// the caret moving is typing, which the slide follows, and the
	// caret scrolled with its text, which it leaves. pan is how far the
	// windows are drawn slid up, in device pixels, easing toward
	// panTarget from panAt.
	keyboard int
	// safe is how far in from each edge, top, right, bottom and left,
	// the system's bars and the camera's cutout reach over the surface,
	// in device pixels.
	safe     [4]int
	caret    geom.Rect
	caretSet bool
	box      geom.Rect
	boxSet   bool
	keyed    bool
	// refit says the keyboard has changed height since the last touch
	// or typing, so the windows lay themselves out afresh round it: a
	// caret or text box they move aims the slide anew, at the least
	// slide that shows it.
	refit     bool
	pan       float64
	panTarget float64
	panAt     time.Time
	// panRest says the pan stood at its target at the last frame.
	panRest bool
	// quit is closed with the last window that fills the screen, and over
	// as Run ends, or main where it never ran.
	quit, over chan struct{}
	overOnce   sync.Once

	// headingTo is the windows that watch the heading, and compassOn
	// says the sensors run for them. compass says the phone has the
	// sensors, once compassOnce has asked.
	headingTo   map[*Window]bool
	compassOn   bool
	compass     bool
	compassOnce sync.Once

	// secureMu is held, with mu not, while Java is told whether what the
	// activity shows is kept to the screen. secureSent is what it was last
	// told, and secureErr what it said.
	secureMu   sync.Mutex
	secureSent bool
	secureErr  error
	// secureUntil is when a secure window that closed has left the screen.
	// mu guards it.
	secureUntil time.Time
	// awakeSent is what keepAwakeLocked last handed Java.
	awakeSent bool
	// excludedSent is the rects, four device pixel coordinates each,
	// last handed to Java to keep from the system's edge gestures.
	excludedSent []int32

	// backLeft is whether the next Back leaves the program, as Java last
	// heard.
	backLeft bool

	// wake nudges the render thread, and surfaces carries surface
	// changes to it.
	wake     chan struct{}
	surfaces chan surfaceChange
}

func newDriver() *Driver {
	return &Driver{
		sized:     make(chan struct{}),
		headingTo: map[*Window]bool{},
		quit:      make(chan struct{}),
		over:      make(chan struct{}),
		wake:      make(chan struct{}, 1),
		surfaces:  make(chan surfaceChange),
	}
}

// start runs main once, for the first activity or other component, in
// the phone's time zone, zone. An activity made again, as after the user
// leaves and comes back, finds it running. quiet keeps what the program
// writes out of the system's log.
func (d *Driver) start(zone string, quiet bool) {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.started = true
	d.mu.Unlock()
	takeEnv()
	// The last run's copies go as this one starts, apart from the UI thread.
	go copies()
	if quiet {
		hush()
	} else {
		toLogcat()
	}
	setZone(zone)
	go d.render()
	go func() {
		mainMain()
		d.end()
		// The last word on work that is done may wait for its turn.
		notices.Drain(5 * time.Second)
		ended()
	}()
}

// end marks the windows done with, and has the activity finish.
func (d *Driver) end() {
	d.overOnce.Do(func() {
		close(d.over)
		finish()
	})
}

// isOver reports whether the windows are done with.
func (d *Driver) isOver() bool {
	select {
	case <-d.over:
		return true
	default:
		return false
	}
}

// waitSized waits for the first surface, and returns an error where the
// program is done with its windows first.
func (d *Driver) waitSized() error {
	select {
	case <-d.sized:
	case <-d.over:
	}
	if d.isOver() {
		return errOver
	}
	return nil
}

// setZone makes zone, as Europe/Stockholm, Go's local time zone. Go
// on Android keeps to UTC, so the activity hands over the phone's zone.
// It runs before main, while [time.Local] is the driver's alone. A zone
// the phone moves to later reaches the program as it starts again.
func setZone(zone string) {
	if zone == "" {
		return
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		log.Printf("gunim: android: time zone %q: %v", zone, err)
		return
	}
	time.Local = loc
}

// Run implements [driver.Driver]. Java pumps the events, so Run waits
// for ctx to end or the last window to close.
func (d *Driver) Run(ctx context.Context, ready func()) error {
	ready()
	select {
	case <-ctx.Done():
	case <-d.quit:
	}
	d.end()
	return nil
}

// Monitors implements [driver.Driver]: the one screen, or none once Run
// has ended.
func (d *Driver) Monitors() []driver.Monitor {
	if d.waitSized() != nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	b := geom.Rect{Max: geom.Pt(float32(d.surfW), float32(d.surfH))}
	return []driver.Monitor{{
		Name: "screen", Bounds: b, WorkArea: b, RefreshRate: d.rate,
		Scale: d.density, CoordsPerLogical: d.density, Primary: true,
	}}
}

// NewWindow implements [driver.Driver]. The first window fills the
// screen. A popup opens by its anchor, a utility window at its anchor,
// and any other window fills the screen over the ones before it.
func (d *Driver) NewWindow(o driver.Options) (driver.Window, error) {
	if err := d.waitSized(); err != nil {
		return nil, err
	}
	w := newWindow(d, o)
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := o.Parent.(*Window); ok {
		w.parent = p
	}
	switch {
	case o.Kind == driver.KindPopup && w.parent != nil:
		w.size = o.Size
		d.placeLocked(w, o.Anchor)
	case o.Kind == driver.KindUtility && w.parent != nil:
		w.size = o.Size
		w.pos = o.Anchor.Min.Add(w.parent.pos)
	default:
		w.fills = true
		for _, e := range d.opened {
			w.in.Push(e)
		}
		d.opened = nil
	}
	w.hidden = o.Hidden
	d.windows = append(d.windows, w)
	d.keepBackLocked()
	d.kick()
	return w, nil
}

// screen returns the surface in logical pixels. It runs with mu held.
func (d *Driver) screen() geom.Rect {
	return geom.Rect{Max: geom.Pt(float32(d.surfW)/d.density, float32(d.surfH)/d.density)}
}

// visibleLocked returns the part of the screen that shows, in logical
// pixels of the windows' own space: the screen less the system's bars
// and what the keyboard covers, moved down by the slide the windows are
// aimed at. It runs with mu held.
func (d *Driver) visibleLocked() geom.Rect {
	s := d.screen()
	safe := d.safeLocked()
	top := float32(d.panTarget) / d.density
	bottom := max(float32(d.keyboard)/d.density, safe.Bottom)
	return geom.Rect{
		Min: geom.Pt(s.Min.X+safe.Left, s.Min.Y+top+safe.Top),
		Max: geom.Pt(s.Max.X-safe.Right, s.Max.Y+top-bottom),
	}
}

// placeLocked puts popup w by anchor, in its parent's logical space:
// below it from its left edge, or above it where there is more room
// there, slid sideways onto the screen, within the part of it that
// shows above the keyboard. It runs with mu held.
func (d *Driver) placeLocked(w *Window, anchor geom.Rect) {
	w.pos = compose.Place(anchor.Add(w.parent.pos), w.size, w.over, w.above, d.visibleLocked())
}

// kick wakes the render thread.
func (d *Driver) kick() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// metrics takes the screen's density and refresh rate, which Java
// sends before each surface.
func (d *Driver) metrics(density float32, rate float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.density, d.rate = density, rate
}

// surfaceChanged hands the render thread a new surface, or a new size
// for the one it has, and tells the windows that fill the screen.
func (d *Driver) surfaceChanged(w *C.ANativeWindow, width, height int) {
	d.surfaces <- surfaceChange{window: w}
	d.mu.Lock()
	first := d.surfW == 0
	d.surfW, d.surfH = width, height
	var fill []*Window
	for _, win := range d.windows {
		if win.fills {
			fill = append(fill, win)
		}
	}
	d.mu.Unlock()
	if first {
		close(d.sized)
	}
	for _, win := range fill {
		win.in.Push(driver.Redraw{})
	}
	d.kick()
}

// surfaceDestroyed takes the surface w back from the render thread, and
// returns once it is let go: Android reclaims it when this returns.
// Where Android replaces the activity, the new activity's surface comes
// before the old one goes, so the render thread lets go of w alone, and
// keeps drawing on the new one.
func (d *Driver) surfaceDestroyed(w *C.ANativeWindow) {
	done := make(chan struct{})
	d.surfaces <- surfaceChange{window: w, gone: done}
	<-done
}

// surfaceChange is a new surface for the render thread, or with gone
// set a surface going: window, or the one it draws on where window is
// nil.
type surfaceChange struct {
	window *C.ANativeWindow
	gone   chan struct{}
}

// insets takes how far in from each edge the system's bars reach over
// the surface, in device pixels, and tells the windows that fill it.
func (d *Driver) insets(top, right, bottom, left int) {
	d.mu.Lock()
	d.safe = [4]int{top, right, bottom, left}
	var fill []*Window
	for _, win := range d.windows {
		if win.fills {
			fill = append(fill, win)
		}
	}
	d.mu.Unlock()
	for _, win := range fill {
		win.in.Push(driver.Redraw{})
	}
	d.kick()
}

// safeLocked returns how far in from each edge the system's bars reach,
// in logical pixels. It runs with mu held.
func (d *Driver) safeLocked() geom.Insets {
	if d.density <= 0 {
		return geom.Insets{}
	}
	f := d.density
	return geom.Insets{Top: float32(d.safe[0]) / f, Right: float32(d.safe[1]) / f,
		Bottom: float32(d.safe[2]) / f, Left: float32(d.safe[3]) / f}
}

// keyboardCovers takes how much of the surface the soft keyboard
// covers, in device pixels from the bottom, and tells the windows that
// fill the screen, which may lay themselves out clear of it.
func (d *Driver) keyboardCovers(px int) {
	d.mu.Lock()
	px = max(0, px)
	changed := px != d.keyboard
	d.keyboard = px
	d.panTarget = 0
	d.refit = true
	d.revealLocked()
	var fill []*Window
	if changed {
		for _, win := range d.windows {
			if win.fills {
				fill = append(fill, win)
			}
		}
	}
	d.mu.Unlock()
	for _, win := range fill {
		win.in.Push(driver.Redraw{})
	}
	d.kick()
}

// aimLocked aims the slide at the text the caret or text box just
// moved in: from where it is aimed while typing, and afresh while the
// windows lay themselves out round a keyboard that changed. A caret
// that moves for any other reason, as its text scrolls, leaves it. It
// runs with mu held.
func (d *Driver) aimLocked() {
	switch {
	case d.refit:
		d.panTarget = 0
		d.revealLocked()
	case d.keyed:
		d.revealLocked()
	}
}

// revealLocked aims the slide at the text: it moves the target just
// enough to show the text box whole, with a gap round it, in what the
// keyboard leaves of the surface, or, for a box too tall for that, the
// caret with a line of room above and below it. With no keyboard it
// aims at no slide. As the caret moves while typing, the slide moves
// from where it is aimed, as little as shows the text; as the keyboard
// changes height, keyboardCovers aims it afresh, at the least slide
// that shows the text, so the box sits the same gap above the keyboard
// each time, and a keyboard that shrinks takes the windows back down.
// It runs with mu held.
func (d *Driver) revealLocked() {
	if d.keyboard == 0 || !d.caretSet || d.typing == nil {
		d.panTarget = 0
		return
	}
	visible := float64(d.surfH - d.keyboard)
	px := func(v float32) float64 { return float64(v * d.density) }
	var top, bottom float64
	if d.boxSet && px(d.box.Max.Y-d.box.Min.Y+2*boxGap) <= visible {
		top, bottom = px(d.box.Min.Y-boxGap), px(d.box.Max.Y+boxGap)
	} else {
		line := d.caret.Max.Y - d.caret.Min.Y
		top, bottom = px(d.caret.Min.Y-line), px(d.caret.Max.Y+line+boxGap)
	}
	t := d.panTarget
	if bottom-t > visible {
		t = bottom - visible
	}
	if top-t < 0 {
		t = top
	}
	d.panTarget = max(0, min(t, float64(d.keyboard)))
}

// panLocked eases the pan toward its target and returns it in whole
// device pixels, with whether it is still on its way. It runs with mu
// held, on the render thread.
func (d *Driver) panLocked(now time.Time) (pan int, moving bool) {
	target := d.panTarget
	dt := min(now.Sub(d.panAt).Seconds(), 0.1)
	if d.panRest {
		// Set off from rest: a frame's step, after a gap of no frames.
		dt = 1 / max(d.rate, 60)
	}
	d.panAt = now
	d.pan += (target - d.pan) * (1 - math.Exp(-dt/panEase))
	if math.Abs(target-d.pan) < 0.5 {
		d.pan = target
	}
	d.panRest = d.pan == target
	return int(math.Round(d.pan)), d.pan != target
}

// boxGap is the room left between the text and the keyboard, in
// logical pixels, and panEase the time the pan takes to cover most of
// the way to its target, in seconds.
const (
	boxGap  = 8
	panEase = 0.05
)

// Permitted implements [driver.Permitter].
func (d *Driver) Permitted(p driver.Permission) bool { return permitted(p) }

// Ask implements [driver.Permitter]: the system's prompt asks the user.
func (d *Driver) Ask(p driver.Permission) bool { return ask(p) }

// UserFolder implements [driver.FolderFinder]: the shared folders of
// the phone's storage, which need permission to read.
func (d *Driver) UserFolder(f driver.UserFolder) string { return userFolder(f) }

// SetNowPlaying implements [driver.NowPlayer]: a media session shows
// what plays in Android's media controls, and a foreground service keeps
// the process running while it plays in the background.
func (d *Driver) SetNowPlaying(np *driver.NowPlaying) error {
	nowPlaying(np)
	return nil
}

// The media controls' actions, as GunimService sends them.
const (
	mediaPlay = iota + 1
	mediaPause
	mediaPlayPause
	mediaNext
	mediaPrevious
	mediaStop
	mediaSeek
)

// media takes an action of the media controls to the window that fills
// the screen, as a media key or a seek to at.
func (d *Driver) media(action int, at time.Duration) {
	d.mu.Lock()
	var top *Window
	for _, w := range d.windows {
		if w.fills {
			top = w
		}
	}
	d.mu.Unlock()
	if top == nil {
		return
	}
	now := time.Now()
	if action == mediaSeek {
		top.in.Push(input.MediaSeek{At: at, Time: now})
		return
	}
	k, ok := map[int]input.Key{
		mediaPlay: input.KeyMediaPlay, mediaPause: input.KeyMediaPause, mediaPlayPause: input.KeyMediaPlayPause,
		mediaNext: input.KeyMediaNext, mediaPrevious: input.KeyMediaPrevious, mediaStop: input.KeyMediaStop,
	}[action]
	if !ok {
		return
	}
	top.in.Push(input.KeyPress{Key: k, Time: now})
	top.in.Push(input.KeyRelease{Key: k, Time: now})
}

// shown tells the windows that fill the screen that the application went
// to the background, or came back.
func (d *Driver) shown(on bool) {
	d.mu.Lock()
	var fill []*Window
	for _, w := range d.windows {
		if w.fills {
			fill = append(fill, w)
		}
	}
	d.mu.Unlock()
	for _, w := range fill {
		w.in.Push(driver.WindowShown{Shown: on})
	}
}

// filesOpened hands files another application opened, as a
// driver.FilesOpened or a driver.StreamsOpened, to the top window that
// fills the screen, or keeps them for the first to open.
func (d *Driver) filesOpened(e any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var top *Window
	for _, w := range d.windows {
		if w.fills {
			top = w
		}
	}
	if top == nil {
		d.opened = append(d.opened, e)
		return
	}
	top.in.Push(e)
}

// windowFocus tells the window on top that the activity has the
// keyboard, or has lost it.
func (d *Driver) windowFocus(focused bool) {
	d.mu.Lock()
	var top *Window
	for _, w := range d.windows {
		if w.fills {
			top = w
		}
	}
	d.mu.Unlock()
	if top != nil {
		top.in.Push(driver.WindowFocus{Focused: focused})
	}
}

// Touch actions, as GunimView sends them.
const (
	touchDown = iota
	touchMove
	touchUp
	touchCancel
)

// hitLocked returns the topmost window under p that takes the pointer.
// It runs with mu held.
func (d *Driver) hitLocked(p geom.Point) *Window {
	for i := len(d.windows) - 1; i >= 0; i-- {
		w := d.windows[i]
		if w.hidden || w.passthrough {
			continue
		}
		if w.rectLocked().Contains(p) {
			return w
		}
	}
	return nil
}

// key sends a key to the window the keyboard types into, or the top
// window that fills the screen.
func (d *Driver) key(down bool, code, meta int, ch rune, repeat bool) {
	d.markTyped()
	w := d.keyWindow()
	if w == nil {
		return
	}
	now := time.Now()
	k, mods := keyOf(code), modsOf(meta)
	if !down {
		w.in.Push(input.KeyRelease{Key: k, Mods: mods, Time: now})
		return
	}
	typed := ch != 0 && mods&(input.ModControl|input.ModAlt|input.ModSuper) == 0 && k != input.KeyEnter && k != input.KeyTab
	w.in.Push(input.KeyPress{Key: k, Mods: mods, Repeat: repeat, Typed: typed, Char: ch, Time: now})
	if typed {
		w.in.Push(input.TextInput{Text: string(ch), Time: now})
	}
}

// edit and typed send what the soft keyboard did to the window it types
// into.
func (d *Driver) edit(e input.TextEdit) {
	d.markTyped()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.editSeq = max(d.editSeq, e.Seq)
	if w := d.keyWindowLocked(); w != nil {
		w.editsIn = e.Seq
		w.in.Push(e)
	}
}

// editSeqOf returns the seq of the keyboard's latest edit.
func (d *Driver) editSeqOf() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.editSeq
}

func (d *Driver) typed(e any) {
	d.markTyped()
	if w := d.keyWindow(); w != nil {
		w.in.Push(e)
	}
}

// markTyped notes that typing has gone to the windows, so the slide
// follows the caret again.
func (d *Driver) markTyped() {
	d.mu.Lock()
	d.keyed, d.refit = true, false
	d.mu.Unlock()
}

// keyWindow returns the window that takes the keyboard.
func (d *Driver) keyWindow() *Window {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.keyWindowLocked()
}

// keyWindowLocked is keyWindow with the driver's mu held.
func (d *Driver) keyWindowLocked() *Window {
	if d.typing != nil && !d.typing.closed {
		return d.typing
	}
	for i := len(d.windows) - 1; i >= 0; i-- {
		if d.windows[i].fills {
			return d.windows[i]
		}
	}
	return nil
}

// closed takes w off the stack, and ends the activity with the last
// window that fills the screen.
func (d *Driver) closed(w *Window) {
	d.mu.Lock()
	for i, x := range d.windows {
		if x == w {
			d.windows = append(d.windows[:i], d.windows[i+1:]...)
			break
		}
	}
	d.gest.Forget(w)
	if d.typing == w {
		d.typing = nil
	}
	delete(d.headingTo, w)
	d.runCompassLocked()
	d.excludeGesturesLocked()
	d.keepAwakeLocked()
	d.keepBackLocked()
	d.gone = append(d.gone, &w.pane)
	last := true
	for _, x := range d.windows {
		if x.fills {
			last = false
		}
	}
	if last {
		select {
		case <-d.quit:
		default:
			close(d.quit)
		}
	} else if w.secure {
		// The screen shows w until another frame is drawn. With the last
		// window the activity goes, as secure as it is.
		d.secureUntil = time.Now().Add(time.Second)
		time.AfterFunc(time.Second+time.Millisecond, func() { _ = d.tellSecure() })
	}
	d.mu.Unlock()
	d.kick()
}
