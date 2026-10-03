//go:build android

package android

import (
	"math"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// touchSlop is how far a finger moves, in logical pixels, before it is
// no longer held still for a long press, and longPress how long it is
// held for one.
const (
	touchSlop = 8
	longPress = 400 * time.Millisecond
)

// gesture is a touch as it goes. start is where the finger went down;
// moved says it has left the slop round it, and held that it was held
// still long enough for a long press. n counts touches, so a long press
// waits for the touch it began with.
type gesture struct {
	start geom.Point
	moved bool
	held  bool
	n     int
	// pinched says a second finger came down: the touch is a pinch
	// from then on, and the finger left after it only lifts. spread is
	// how far apart the two fingers were last, in logical pixels.
	pinched bool
	spread  float32
}

// touch turns the first finger into the pointer: it presses, moves and
// lets go as a mouse's primary button does, marked as touch, and leaves
// the window as it lifts, since a finger has no hover. The engine works
// out from what the finger pressed whether its moves drag or scroll. A
// finger held still is a long press; see longPress. x and y are in
// device pixels.
func (d *Driver) touch(action int, x, y float32, now time.Time) {
	d.mu.Lock()
	// The windows are drawn slid up by the pan, so the point touched
	// is that much further down them.
	at := geom.Pt(x/d.density, (y+float32(math.Round(d.pan)))/d.density)
	g := &d.gesture
	w := d.touched
	if action == touchDown {
		w = d.hitLocked(at)
		d.touched = w
		d.keyed = false
		*g = gesture{start: at, n: g.n + 1}
		n := g.n
		time.AfterFunc(longPress, func() { d.longPress(n) })
		if now.Sub(d.lastAt) < doubleTapTime && abs(at.X-d.lastTap.X) < doubleTapSpace && abs(at.Y-d.lastTap.Y) < doubleTapSpace {
			d.clicks++
		} else {
			d.clicks = 1
		}
		d.lastTap, d.lastAt = at, now
	}
	if action == touchMove && math.Hypot(float64(at.X-g.start.X), float64(at.Y-g.start.Y)) > touchSlop {
		g.moved = true
	}
	held := g.held || g.pinched
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
	if held && action != touchDown {
		// After a long press or a pinch the finger only lifts.
		if action == touchUp || action == touchCancel {
			w.in.Push(input.PointerLeave{Time: now})
		}
		return
	}
	switch action {
	case touchDown:
		w.in.Push(input.PointerMove{Pos: pos, Touch: true, Time: now})
		w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: clicks, Touch: true, Time: now})
	case touchMove:
		w.in.Push(input.PointerMove{Pos: pos, Touch: true, Time: now})
	case touchUp:
		w.in.Push(input.PointerUp{Pos: pos, Button: input.ButtonPrimary, Touch: true, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	case touchCancel:
		w.in.Push(input.PointerUp{Pos: away, Button: input.ButtonPrimary, Touch: true, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	}
}

// longPress turns touch n into a long press, if the finger is still
// down within the slop round where it went down: the press is let go
// away from it, and the secondary button is pressed there, as a right
// click does, which opens a context menu, and the phone buzzes.
func (d *Driver) longPress(n int) {
	d.mu.Lock()
	g := &d.gesture
	w := d.touched
	if g.n != n || w == nil || g.moved || g.held {
		d.mu.Unlock()
		return
	}
	g.held = true
	pos := g.start.Sub(w.pos)
	d.mu.Unlock()
	now := time.Now()
	w.in.Push(input.PointerUp{Pos: away, Button: input.ButtonPrimary, Touch: true, Time: now})
	w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonSecondary, Clicks: 1, Touch: true, Time: now})
	w.in.Push(input.PointerUp{Pos: pos, Button: input.ButtonSecondary, Touch: true, Time: now})
	buzz()
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
// the first one's press is let go far away, so it neither clicks nor
// scrolls. As the fingers spread or close, the window under them hears
// Ctrl with the wheel at the point between them, a notch for each
// quarter the spread grows or shrinks, which zooms a picture, a plot or
// a grid that zooms with the wheel, or the window where it zooms. x0,
// y0, x1 and y1 are the fingers, in device pixels.
func (d *Driver) pinch(action int, x0, y0, x1, y1 float32, now time.Time) {
	d.mu.Lock()
	g := &d.gesture
	w := d.touched
	pan := float32(math.Round(d.pan))
	a := geom.Pt(x0/d.density, (y0+pan)/d.density)
	b := geom.Pt(x1/d.density, (y1+pan)/d.density)
	spread := float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
	if w == nil {
		d.mu.Unlock()
		return
	}
	switch action {
	case pinchStart:
		was := g.pinched
		g.pinched, g.moved, g.spread = true, true, spread
		d.mu.Unlock()
		if !was && !g.held {
			w.in.Push(input.PointerUp{Pos: away, Button: input.ButtonPrimary, Time: now})
		}
		return
	case pinchEnd:
		d.mu.Unlock()
		return
	}
	if g.spread <= 0 || spread <= 0 {
		g.spread = spread
		d.mu.Unlock()
		return
	}
	notches := float32(math.Log(float64(spread/g.spread)) / math.Log(pinchStep))
	g.spread = spread
	mid := a.Add(b).Mul(0.5).Sub(w.pos)
	d.mu.Unlock()
	w.in.Push(input.Scroll{Pos: mid, Notches: geom.Pt(0, notches), Mods: input.ModControl, Time: now})
}

// away is a point far outside any window: a press let go there lets go
// of nothing.
var away = geom.Pt(-1e6, -1e6)
