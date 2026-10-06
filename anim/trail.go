package anim

import (
	"time"

	"github.com/marrasen/gunim/geom"
)

// Trail follows a point a pointer drags, for how fast it was going as
// it was let go: the speed to fling something on at, with [Fling] or a
// coast of a widget's own, as a finger flicks a picture or a page.
//
//	case input.PointerMove:
//		t.Add(e.Pos, e.Time)
//	case input.PointerUp:
//		v := t.Velocity(e.Time)
type Trail struct {
	at [8]trailPoint
	n  int
}

type trailPoint struct {
	p geom.Point
	t time.Time
}

// trailWindow is how far back the velocity looks: the last tenth of a
// second, so a drag that slowed before letting go flings slowly.
const trailWindow = 100 * time.Millisecond

// Add records the point at p at time t, the time of the event that
// moved it; a zero t takes the time now.
func (t *Trail) Add(p geom.Point, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	copy(t.at[1:], t.at[:len(t.at)-1])
	t.at[0] = trailPoint{p, at}
	t.n = min(t.n+1, len(t.at))
}

// Reset forgets the points, for a new drag.
func (t *Trail) Reset() { t.n = 0 }

// Velocity returns how fast the point went over the last tenth of a
// second before now, in units a second, or nothing when it had come to
// rest before then: a drag held still and let go flings nowhere. A zero
// now takes the time now.
func (t *Trail) Velocity(now time.Time) geom.Point {
	if t.n < 2 {
		return geom.Point{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	last := t.at[0]
	if now.Sub(last.t) > trailWindow {
		return geom.Point{}
	}
	first := last
	for _, q := range t.at[1:t.n] {
		if last.t.Sub(q.t) > trailWindow {
			break
		}
		first = q
	}
	dt := float32(last.t.Sub(first.t).Seconds())
	if dt <= 0 {
		return geom.Point{}
	}
	return geom.Pt((last.p.X-first.p.X)/dt, (last.p.Y-first.p.Y)/dt)
}
