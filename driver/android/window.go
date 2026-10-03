//go:build android

package android

import (
	"errors"
	"image/color"
	"math"
	"sync"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/inbox"
	"github.com/marrasen/gunim/driver/internal/render"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// errClosed is returned by a closed window.
var errClosed = errors.New("android: window closed")

// Window is the Android implementation of [driver.Window]: a rectangle
// of the one surface, drawn into a texture of its own.
type Window struct {
	d         *Driver
	in        *inbox.Inbox
	quit      chan struct{}
	presented chan driver.Frame
	closeOnce sync.Once

	parent                   *Window
	over, above, passthrough bool
	transparent              bool

	// The fields below are guarded by the driver's mu.

	// fills says the window covers the screen, as the first one does.
	// pos and size place any other, in logical pixels of the screen.
	fills  bool
	pos    geom.Point
	size   geom.Size
	hidden bool
	closed bool
	// next is the frame waiting for the render thread, and under the
	// colour under the window's frames.
	next  *frame
	under color.NRGBA

	// The fields below belong to the render thread.

	r          *render.Renderer
	tex, fbo   uint32
	texW, texH int
}

// frame is one frame for the render thread.
type frame struct {
	ops    []paint.Op
	damage geom.Rect
}

func newWindow(d *Driver, o driver.Options) *Window {
	w := &Window{
		d:           d,
		quit:        make(chan struct{}),
		presented:   make(chan driver.Frame, 1),
		over:        o.Over,
		above:       o.Above,
		passthrough: o.Passthrough,
		transparent: o.Kind == driver.KindPopup,
	}
	w.in = inbox.New(w.quit)
	return w
}

// rectLocked returns where the window is on the screen, in logical
// pixels. It runs with the driver's mu held.
func (w *Window) rectLocked() geom.Rect {
	if w.fills {
		return w.d.screen()
	}
	return geom.Rect{Min: w.pos, Max: w.pos.Add(geom.Pt(w.size.W, w.size.H))}
}

// Presented implements [driver.Window].
func (w *Window) Presented() <-chan driver.Frame { return w.presented }

// Input implements [driver.Window].
func (w *Window) Input() <-chan any { return w.in.Out() }

// Present implements [driver.Window]. It leaves the frame for the
// render thread, which draws it with the next frame of the surface.
func (w *Window) Present(ops []paint.Op, damage geom.Rect) error {
	w.d.mu.Lock()
	if w.closed {
		w.d.mu.Unlock()
		return errClosed
	}
	w.next = &frame{ops: ops, damage: damage}
	w.d.mu.Unlock()
	w.d.kick()
	return nil
}

// Size implements [driver.Window].
func (w *Window) Size() geom.Size {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return w.rectLocked().Size()
}

// Scale implements [driver.Window].
func (w *Window) Scale() float32 {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return w.d.density
}

// RefreshRate implements [driver.Window].
func (w *Window) RefreshRate() float64 {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return w.d.rate
}

// Clipboard implements [driver.Window].
func (w *Window) Clipboard() (string, error) { return clipboard(), nil }

// SetClipboard implements [driver.Window].
func (w *Window) SetClipboard(s string) error {
	setClipboard(s)
	return nil
}

// Close implements [driver.Window].
func (w *Window) Close() error {
	w.closeOnce.Do(func() {
		w.d.mu.Lock()
		w.closed = true
		w.next = nil
		w.d.mu.Unlock()
		w.d.closed(w)
		close(w.quit)
		w.in.Close()
	})
	return nil
}

// Place implements [driver.Placer].
func (w *Window) Place(anchor geom.Rect, size geom.Size) error {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	if w.parent == nil || w.fills {
		return nil
	}
	w.size = size
	w.d.placeLocked(w, anchor)
	w.d.kick()
	return nil
}

// PopupRoom implements [driver.PopupRoomer]: the room from anchor, in
// w's logical space, to the screen's edges.
func (w *Window) PopupRoom(anchor geom.Rect) driver.Room {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	s := w.d.screen()
	a := anchor.Add(w.rectLocked().Min)
	return driver.Room{
		Below: s.Max.Y - a.Max.Y,
		Above: a.Min.Y - s.Min.Y,
		Left:  a.Min.X - s.Min.X,
		Right: s.Max.X - a.Min.X,
	}
}

// ToScreen implements [driver.Screener]. The screen's coordinates are
// logical pixels here.
func (w *Window) ToScreen(p geom.Point) geom.Point {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return p.Add(w.rectLocked().Min)
}

// FromScreen implements [driver.Screener].
func (w *Window) FromScreen(p geom.Point) geom.Point {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return p.Sub(w.rectLocked().Min)
}

// Transparent implements [driver.Transparent]: a popup shows the
// windows under it where it paints nothing.
func (w *Window) Transparent() bool { return w.transparent }

// SetBackground implements [driver.Backgrounder].
func (w *Window) SetBackground(c color.NRGBA) {
	w.d.mu.Lock()
	w.under = c
	w.d.mu.Unlock()
}

// Hide implements [driver.Recycler].
func (w *Window) Hide() error {
	w.d.mu.Lock()
	w.hidden = true
	w.d.mu.Unlock()
	w.d.kick()
	return nil
}

// Show implements [driver.Recycler].
func (w *Window) Show() error {
	w.d.mu.Lock()
	w.hidden = false
	w.d.mu.Unlock()
	w.d.kick()
	return nil
}

// SetTextInput implements [driver.TextInputter]. The window takes the
// keyboard's input while it takes text, and the soft keyboard goes away
// as it stops; a tap brings the keyboard up, through ShowKeyboard.
func (w *Window) SetTextInput(active bool) {
	w.d.mu.Lock()
	if active {
		w.d.typing = w
	} else if w.d.typing == w {
		w.d.typing = nil
		w.d.caretSet, w.d.boxSet = false, false
	} else {
		w.d.mu.Unlock()
		return
	}
	w.d.mu.Unlock()
	if !active {
		showKeyboard(false)
	}
}

// SetTextCaret implements [driver.CaretPlacer]. The driver keeps the
// caret above the soft keyboard: it slides the windows up just far
// enough to show it as the keyboard opens, and follows the caret as
// typing moves it. A caret that moves as its text scrolls stays where
// it goes. Before Android 11, which reports no keyboard as it
// slides, Android pans the window to the caret itself.
func (w *Window) SetTextCaret(r geom.Rect) {
	w.d.mu.Lock()
	at, f := w.rectLocked().Min, w.d.density
	w.d.caret, w.d.caretSet = r.Add(at), true
	if w.d.keyed {
		w.d.revealLocked()
	}
	w.d.mu.Unlock()
	w.d.kick()
	px := func(v float32) int { return int(math.Round(float64(v * f))) }
	sendCaret(px(r.Min.X+at.X), px(r.Min.Y+at.Y), px(r.Max.X+at.X), px(r.Max.Y+at.Y))
}

// SetTextBox implements [driver.TextBoxPlacer]: the slide keeps the
// whole text box above the keyboard where it fits.
func (w *Window) SetTextBox(r geom.Rect) {
	w.d.mu.Lock()
	w.d.box, w.d.boxSet = r.Add(w.rectLocked().Min), true
	if w.d.keyed {
		w.d.revealLocked()
	}
	w.d.mu.Unlock()
	w.d.kick()
}

// ShowKeyboard implements [driver.KeyboardShower].
func (w *Window) ShowKeyboard() { showKeyboard(true) }

// SetTextState implements [driver.TextStater]. Java keeps the copy,
// and decides from seq whether the state is behind the keyboard's
// edits.
func (w *Window) SetTextState(s *input.TextState, seq uint64) {
	w.d.mu.Lock()
	typing := w.d.typing == w || w.d.typing == nil
	w.d.mu.Unlock()
	if typing {
		sendTextState(s, seq)
	}
}

var (
	_ driver.Placer         = (*Window)(nil)
	_ driver.PopupRoomer    = (*Window)(nil)
	_ driver.Screener       = (*Window)(nil)
	_ driver.Transparent    = (*Window)(nil)
	_ driver.Backgrounder   = (*Window)(nil)
	_ driver.Recycler       = (*Window)(nil)
	_ driver.TextInputter   = (*Window)(nil)
	_ driver.TextStater     = (*Window)(nil)
	_ driver.KeyboardShower = (*Window)(nil)
	_ driver.CaretPlacer    = (*Window)(nil)
	_ driver.TextBoxPlacer  = (*Window)(nil)
)
