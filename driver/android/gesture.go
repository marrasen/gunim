//go:build android

package android

import (
	"math"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// gesture is a touch as it goes. pinched says a second finger came
// down: the touch is a pinch from then on, and the finger left after it
// only lifts. spread is how far apart the two fingers were last, in
// logical pixels.
type gesture struct {
	pinched bool
	spread  float32
}

// touch turns the first finger into the pointer: it presses, moves and
// lets go as a mouse's primary button does, marked as touch, and leaves
// the window as it lifts, since a finger has no hover. The engine works
// out from what the finger does and what it pressed whether it taps,
// drags, scrolls or long presses. x and y are in device pixels.
//
// The events go to the window's queue with the driver's lock held, so a
// touch and a pinch reach the engine in the order they happened.
func (d *Driver) touch(action int, x, y float32, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// The windows are drawn slid up by the pan, so the point touched
	// is that much further down them.
	at := geom.Pt(x/d.density, (y+float32(math.Round(d.pan)))/d.density)
	g := &d.gesture
	w := d.touched
	if action == touchDown {
		w = d.hitLocked(at)
		d.touched = w
		d.keyed, d.refit = false, false
		*g = gesture{}
		if now.Sub(d.lastAt) < doubleTapTime && abs(at.X-d.lastTap.X) < doubleTapSpace && abs(at.Y-d.lastTap.Y) < doubleTapSpace {
			d.clicks++
		} else {
			d.clicks = 1
		}
		d.lastTap, d.lastAt = at, now
	}
	if action == touchUp || action == touchCancel {
		d.touched = nil
	}
	if w == nil {
		return
	}
	pos := at.Sub(w.pos)
	if g.pinched && action != touchDown {
		// After a pinch the finger only lifts.
		if action == touchUp || action == touchCancel {
			w.in.Push(input.PointerLeave{Time: now})
		}
		return
	}
	switch action {
	case touchDown:
		w.in.Push(input.PointerMove{Pos: pos, Touch: true, Time: now})
		w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: d.clicks, Touch: true, Time: now})
	case touchMove:
		w.in.Push(input.PointerMove{Pos: pos, Touch: true, Time: now})
	case touchUp:
		w.in.Push(input.PointerUp{Pos: pos, Button: input.ButtonPrimary, Touch: true, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	case touchCancel:
		w.in.Push(input.PointerUp{Pos: input.Away, Button: input.ButtonPrimary, Touch: true, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	}
}

// Pinch actions, as GunimView sends them.
const (
	pinchStart = iota
	pinchMove
	pinchEnd
)

// pinchStep is how much the fingers' spread grows for one notch of
// zoom: a notch of a wheel zooms by about a quarter.
const pinchStep = 1.25

// pinch turns two fingers into zoom. As the second finger comes down,
// the first one's press is let go at [input.Away], so it neither clicks
// nor scrolls nor opens a menu. As the fingers spread or close, the window under them hears
// Ctrl with the wheel at the point between them, a notch for each
// quarter the spread grows or shrinks, which zooms a picture, a plot or
// a grid that zooms with the wheel, or the window where it zooms. x0,
// y0, x1 and y1 are the fingers, in device pixels.
func (d *Driver) pinch(action int, x0, y0, x1, y1 float32, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g := &d.gesture
	w := d.touched
	if w == nil {
		return
	}
	pan := float32(math.Round(d.pan))
	a := geom.Pt(x0/d.density, (y0+pan)/d.density)
	b := geom.Pt(x1/d.density, (y1+pan)/d.density)
	spread := float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
	switch action {
	case pinchStart:
		if !g.pinched {
			// The first finger's press, whatever it became, is called
			// off.
			w.in.Push(input.PointerUp{Pos: input.Away, Button: input.ButtonPrimary, Touch: true, Time: now})
		}
		g.pinched, g.spread = true, spread
		return
	case pinchEnd:
		return
	}
	if g.spread <= 0 || spread <= 0 {
		g.spread = spread
		return
	}
	notches := float32(math.Log(float64(spread/g.spread)) / math.Log(pinchStep))
	g.spread = spread
	w.in.Push(input.Scroll{Pos: a.Add(b).Mul(0.5).Sub(w.pos), Notches: geom.Pt(0, notches), Mods: input.ModControl, Time: now})
}
