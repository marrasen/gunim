//go:build android

package android

import (
	"math"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// Touch slop and fling, in logical pixels and seconds. A finger that
// moves further than touchSlop from where it went down is scrolling. A
// fling slows by e every flingDecay seconds, and stops below
// flingStop pixels a second.
const (
	touchSlop  = 8
	flingDecay = 0.33
	flingStop  = 20
	flingTick  = 8 * time.Millisecond
)

// gesture is a touch as it goes: a press, until the finger has moved
// far enough to be scrolling, and then a scroll. last and lastAt are
// where the finger was last seen and when, and vel its smoothed speed
// in logical pixels a second, for a fling as it lifts. stop ends the
// fling still coasting from the last touch.
type gesture struct {
	start, last geom.Point
	lastAt      time.Time
	scrolling   bool
	vel         geom.Point
	stop        chan struct{}
}

// touch turns the first finger into the pointer. A tap presses and lets
// go as a mouse's primary button does, and leaves the window as it
// lifts, since a finger has no hover. A finger that moves past the
// slop scrolls instead: the press is let go away from where it went
// down, so a button under it does nothing, and each move after scrolls
// what the finger came down on by as far as the finger went, as a
// wheel would. A finger that lifts while moving flings: the scroll
// coasts on and slows. x and y are in device pixels.
func (d *Driver) touch(action int, x, y float32, now time.Time) {
	d.mu.Lock()
	// The windows are drawn slid up by the pan, so the point touched
	// is that much further down them.
	at := geom.Pt(x/d.density, (y+float32(math.Round(d.pan)))/d.density)
	g := &d.gesture
	w := d.touched
	if action == touchDown {
		if g.stop != nil {
			close(g.stop)
			g.stop = nil
		}
		w = d.hitLocked(at)
		d.touched = w
		d.keyed = false
		*g = gesture{start: at, last: at, lastAt: now}
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
	var origin geom.Point
	if w != nil {
		origin = w.pos
	}
	pos := at.Sub(origin)
	startScroll := action == touchMove && !g.scrolling && math.Hypot(float64(at.X-g.start.X), float64(at.Y-g.start.Y)) > touchSlop
	if startScroll {
		// The scroll starts from here, with no jump for the slop.
		g.scrolling = true
		g.last, g.lastAt = at, now
	}
	scrolling := g.scrolling
	var delta geom.Point
	if action == touchMove && scrolling && !startScroll {
		delta = at.Sub(g.last)
		if dt := now.Sub(g.lastAt).Seconds(); dt > 0 {
			v := delta.Mul(float32(1 / dt))
			g.vel = g.vel.Mul(0.6).Add(v.Mul(0.4))
		}
		g.last, g.lastAt = at, now
	}
	var fling chan struct{}
	// A scroll, and the fling after it, goes where the finger came down,
	// so it stays with what it started on as that slides or scrolls.
	vel, from := g.vel, g.start.Sub(origin)
	if action == touchUp && scrolling && now.Sub(g.lastAt) < 100*time.Millisecond &&
		math.Hypot(float64(vel.X), float64(vel.Y)) > flingStop {
		fling = make(chan struct{})
		g.stop = fling
	}
	d.mu.Unlock()
	if w == nil {
		return
	}
	switch {
	case action == touchDown:
		w.in.Push(input.PointerMove{Pos: pos, Time: now})
		w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: clicks, Time: now})
	case startScroll:
		w.in.Push(input.PointerUp{Pos: away, Button: input.ButtonPrimary, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	case action == touchMove && scrolling:
		w.in.Push(input.Scroll{Pos: from, Delta: delta, Time: now})
	case action == touchMove:
		w.in.Push(input.PointerMove{Pos: pos, Time: now})
	case action == touchUp && !scrolling:
		w.in.Push(input.PointerUp{Pos: pos, Button: input.ButtonPrimary, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	case action == touchCancel && !scrolling:
		w.in.Push(input.PointerUp{Pos: away, Button: input.ButtonPrimary, Time: now})
		w.in.Push(input.PointerLeave{Time: now})
	}
	if fling != nil {
		go flingOn(w, from, vel, fling)
	}
}

// away is a point far outside any window: a press let go there lets go
// of nothing.
var away = geom.Pt(-1e6, -1e6)

// flingOn scrolls w at p on from vel, in logical pixels a second,
// slowing as it goes, until it stops or stop closes.
func flingOn(w *Window, p, vel geom.Point, stop chan struct{}) {
	t := time.NewTicker(flingTick)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-stop:
			return
		case <-w.quit:
			return
		case now := <-t.C:
			dt := now.Sub(last).Seconds()
			last = now
			vel = vel.Mul(float32(math.Exp(-dt / flingDecay)))
			if math.Hypot(float64(vel.X), float64(vel.Y)) < flingStop {
				return
			}
			w.in.Push(input.Scroll{Pos: p, Delta: vel.Mul(float32(dt)), Time: now})
		}
	}
}
