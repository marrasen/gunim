package gunim

import (
	"math"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A TouchDragger is a node a finger drags, as a slider's thumb or a
// plot that pans does. A finger that presses one and moves goes on
// moving the pointer, as a mouse does. A finger that presses anything
// else and moves scrolls what it came down on.
type TouchDragger interface {
	Node
	DragsTouch() bool
}

// Touch slop and fling, in logical pixels and seconds. A finger that
// moves further than touchSlop from where it went down has chosen
// between scrolling and dragging. A fling slows by e every flingDecay
// seconds, and stops below flingStop pixels a second.
const (
	touchSlop  = 8
	flingDecay = 0.33
	flingStop  = 20
)

// touchPress follows a finger's press in a window: where it went down,
// whether its moves have chosen yet, and, for a scroll, where it was
// last and how fast it moves, in logical pixels a second.
type touchPress struct {
	root        *state
	start, last geom.Point
	lastAt      time.Time
	chosen      bool
	scrolling   bool
	vel         geom.Point
}

// touchFling is a scroll coasting on after the finger lifted.
type touchFling struct {
	root *state
	at   geom.Point
	vel  geom.Point
}

// away is a point far outside any window: a press let go there lets go
// of nothing.
var away = geom.Pt(-1e6, -1e6)

// touchEvent takes the events of a finger pressing with the primary
// button, and reports whether it dealt with ev itself.
//
// Moves within the slop are held: the finger has not shown yet whether
// it drags or scrolls. Past it, a press on a [TouchDragger] goes on as
// a mouse's does. Any other press is let go away from where it went
// down, so a button under the finger does nothing, and each move after
// scrolls what the finger came down on by as far as the finger went, as
// a wheel would. A finger that lifts while scrolling flings: the scroll
// coasts on, slowing, over the frames that follow.
func (u *UI) touchEvent(root *state, ev any) bool {
	switch e := ev.(type) {
	case input.PointerDown:
		u.fling = nil
		u.touch = nil
		if e.Touch && e.Button == input.ButtonPrimary {
			u.touch = &touchPress{root: root, start: e.Pos, last: e.Pos, lastAt: e.Time}
		}
	case input.PointerMove:
		t := u.touch
		if !e.Touch || t == nil || t.root != root {
			return false
		}
		if !t.chosen {
			d := e.Pos.Sub(t.start)
			if math.Hypot(float64(d.X), float64(d.Y)) <= touchSlop {
				return true
			}
			t.chosen = true
			if c := u.capture; c != nil {
				if d, ok := c.node.(TouchDragger); ok && d.DragsTouch() {
					return false
				}
			}
			t.scrolling = true
			u.handleOn(root, input.PointerUp{Pos: away, Button: input.ButtonPrimary, Mods: e.Mods, Time: e.Time})
			// The scroll starts from here, with no jump for the slop.
			t.last, t.lastAt = e.Pos, e.Time
			return true
		}
		if !t.scrolling {
			return false
		}
		delta := e.Pos.Sub(t.last)
		if dt := e.Time.Sub(t.lastAt).Seconds(); dt > 0 {
			t.vel = t.vel.Mul(0.6).Add(delta.Mul(float32(0.4 / dt)))
		}
		t.last, t.lastAt = e.Pos, e.Time
		u.handleOn(root, input.Scroll{Pos: t.start, Delta: delta, Mods: e.Mods, Time: e.Time})
		return true
	case input.PointerUp:
		t := u.touch
		if !e.Touch || t == nil || t.root != root {
			return false
		}
		u.touch = nil
		if !t.scrolling {
			return false
		}
		speed := math.Hypot(float64(t.vel.X), float64(t.vel.Y))
		if e.Time.Sub(t.lastAt) < 100*time.Millisecond && speed > flingStop {
			u.fling = &touchFling{root: root, at: t.start, vel: t.vel}
			u.invalid = true
		}
		return true
	}
	return false
}

// stepFling scrolls a fling on by dt, slowing it, and reports whether it
// coasts on.
func (u *UI) stepFling(dt time.Duration) bool {
	f := u.fling
	if f == nil || dt <= 0 {
		return f != nil
	}
	s := dt.Seconds()
	f.vel = f.vel.Mul(float32(math.Exp(-s / flingDecay)))
	if math.Hypot(float64(f.vel.X), float64(f.vel.Y)) < flingStop {
		u.fling = nil
		return false
	}
	u.handleOn(f.root, input.Scroll{Pos: f.at, Delta: f.vel.Mul(float32(s)), Time: u.now})
	return true
}
