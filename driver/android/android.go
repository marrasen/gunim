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
	"sync"
	"time"
	_ "unsafe" // for go:linkname

	"github.com/marrasen/gunim/driver"
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

// errNotStarted is returned by Open outside an activity.
var errNotStarted = errors.New("android: no activity started gunim")

// Open returns the driver. The activity has started it by the time
// main runs.
func Open() (*Driver, error) {
	d := theDriver
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return nil, errNotStarted
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
	// windows is the stack of open windows, bottom first.
	windows []*Window
	// touched is the window the touch in progress started in, which
	// keeps the touch until it lifts. lastTap and lastAt are the last
	// press, for a double tap.
	touched *Window
	lastTap geom.Point
	lastAt  time.Time
	clicks  int
	// typing is the window the keyboard types into: the one that turned
	// text input on last.
	typing *Window
	quit   chan struct{}

	// wake nudges the render thread, and surfaces carries surface
	// changes to it.
	wake     chan struct{}
	surfaces chan surfaceChange
}

func newDriver() *Driver {
	return &Driver{
		sized:    make(chan struct{}),
		quit:     make(chan struct{}),
		wake:     make(chan struct{}, 1),
		surfaces: make(chan surfaceChange),
	}
}

// start runs main once, for the first activity. An activity made again,
// as after the user leaves and comes back, finds it running.
func (d *Driver) start() {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.started = true
	d.mu.Unlock()
	takeEnv()
	toLogcat()
	go d.render()
	go func() {
		mainMain()
		finish()
	}()
}

// Run implements [driver.Driver]. Java pumps the events, so Run waits
// for ctx to end or the last window to close.
func (d *Driver) Run(ctx context.Context, ready func()) error {
	ready()
	select {
	case <-ctx.Done():
	case <-d.quit:
	}
	return nil
}

// Monitors implements [driver.Driver]: the one screen.
func (d *Driver) Monitors() []driver.Monitor {
	<-d.sized
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
	<-d.sized
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
	}
	w.hidden = o.Hidden
	d.windows = append(d.windows, w)
	d.kick()
	return w, nil
}

// screen returns the surface in logical pixels. It runs with mu held.
func (d *Driver) screen() geom.Rect {
	return geom.Rect{Max: geom.Pt(float32(d.surfW)/d.density, float32(d.surfH)/d.density)}
}

// placeLocked puts popup w by anchor, in its parent's logical space:
// below it from its left edge, or above it where there is more room
// there, slid sideways onto the screen. It runs with mu held.
func (d *Driver) placeLocked(w *Window, anchor geom.Rect) {
	a := anchor.Add(w.parent.pos)
	if w.over {
		w.pos = a.Min
		return
	}
	s := d.screen()
	below, above := s.Max.Y-a.Max.Y, a.Min.Y-s.Min.Y
	y := a.Max.Y
	if w.above && above >= w.size.H || w.size.H > below && above > below {
		y = a.Min.Y - w.size.H
	}
	x := max(s.Min.X, min(a.Min.X, s.Max.X-w.size.W))
	w.pos = geom.Pt(x, y)
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

// surfaceDestroyed takes the surface back from the render thread, and
// returns once it is let go: Android reclaims it when this returns.
func (d *Driver) surfaceDestroyed() {
	done := make(chan struct{})
	d.surfaces <- surfaceChange{gone: done}
	<-done
}

// surfaceChange is a new surface for the render thread, or with gone
// set the old one going.
type surfaceChange struct {
	window *C.ANativeWindow
	gone   chan struct{}
}

// windowFocus tells the windows that fill the screen that the activity
// has the keyboard, or has lost it.
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

// doubleTap is how close in time and space two taps must be to count
// as a double tap, in logical pixels.
const (
	doubleTapTime  = 300 * time.Millisecond
	doubleTapSpace = 24
)

// touch turns the first finger into the pointer: it presses, moves and
// lets go as a mouse's primary button does, and leaves the window as it
// lifts, since a finger has no hover. x and y are in device pixels.
func (d *Driver) touch(action int, x, y float32, now time.Time) {
	d.mu.Lock()
	at := geom.Pt(x/d.density, y/d.density)
	w := d.touched
	if action == touchDown {
		w = d.hitLocked(at)
		d.touched = w
		if now.Sub(d.lastAt) < doubleTapTime && abs(at.X-d.lastTap.X) < doubleTapSpace && abs(at.Y-d.lastTap.Y) < doubleTapSpace {
			d.clicks++
		} else {
			d.clicks = 1
		}
		d.lastTap, d.lastAt = at, now
	}
	clicks := d.clicks
	if action == touchUp || action == touchCancel {
		d.touched = nil
	}
	var pos geom.Point
	if w != nil {
		pos = at.Sub(w.pos)
	}
	d.mu.Unlock()
	if w == nil {
		return
	}
	switch action {
	case touchDown:
		w.in.Push(input.PointerMove{Pos: pos, Time: now})
		w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: clicks, Time: now})
	case touchMove:
		w.in.Push(input.PointerMove{Pos: pos, Time: now})
	case touchUp:
		w.in.Push(input.PointerUp{Pos: pos, Button: input.ButtonPrimary, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	case touchCancel:
		w.in.Push(input.PointerLeave{Time: now})
	}
}

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
	if w := d.keyWindow(); w != nil {
		w.in.Push(e)
	}
}

func (d *Driver) typed(e any) {
	if w := d.keyWindow(); w != nil {
		w.in.Push(e)
	}
}

// keyWindow returns the window that takes the keyboard.
func (d *Driver) keyWindow() *Window {
	d.mu.Lock()
	defer d.mu.Unlock()
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
	if d.touched == w {
		d.touched = nil
	}
	if d.typing == w {
		d.typing = nil
	}
	last := true
	for _, x := range d.windows {
		if x.fills {
			last = false
		}
	}
	d.mu.Unlock()
	d.kick()
	if last {
		select {
		case <-d.quit:
		default:
			close(d.quit)
		}
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
