package audioui

import (
	"math"
	"time"
)

// Fling glides a view on after a flick, as a waveform's view is
// dragged along and let go while it moves: on at the speed it was
// dragged, slowing, to a stop or the view's bounds.
//
// While the view is held, call Held each frame with where it starts.
// As it is let go, call Release. Each frame after, Step glides it.
type Fling struct {
	// v is how fast the view glides, in its units a second; dragV is
	// how fast the drag moved it, was where it started the last frame,
	// and still how long since it moved.
	v, dragV float64
	was      float64
	still    time.Duration
}

// Held follows the view, held, starting at v0 this frame, dt after the
// last.
func (f *Fling) Held(v0 float64, dt time.Duration) {
	f.v = 0
	if sec := dt.Seconds(); sec > 0 {
		if v0 != f.was {
			f.dragV = 0.6*f.dragV + 0.4*(v0-f.was)/sec
			f.still = 0
		} else {
			f.still += dt
		}
	}
	f.was = v0
}

// Grab starts a hold of the view at v0, stopping any glide.
func (f *Fling) Grab(v0 float64) { f.v, f.dragV, f.was, f.still = 0, 0, v0, 0 }

// Release lets the view go, gliding on where it moved in the last 80 ms
// faster than least units a second.
func (f *Fling) Release(least float64) {
	if f.still < 80*time.Millisecond && math.Abs(f.dragV) > least {
		f.v = f.dragV
	}
}

// Stop stops the glide.
func (f *Fling) Stop() { f.v = 0 }

// Gliding says the view glides.
func (f *Fling) Gliding() bool { return f.v != 0 }

// Step glides a view starting at v0, span long, over dt, kept within lo
// to hi, and returns where it starts now and whether it glides on. It
// stops at the bounds, and as it slows under 2% of the span a second.
func (f *Fling) Step(v0, span, lo, hi float64, dt time.Duration) (float64, bool) {
	sec := dt.Seconds()
	if f.v == 0 || sec <= 0 {
		f.was = v0
		return v0, f.v != 0
	}
	v0 += f.v * sec
	if v0 < lo || v0+span > hi {
		v0 = max(lo, min(v0, hi-span))
		f.v = 0
	}
	f.v *= math.Exp(-4 * sec)
	if math.Abs(f.v) < span*0.02 {
		f.v = 0
	}
	f.was = v0
	return v0, f.v != 0
}
