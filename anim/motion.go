package anim

import (
	"math"
	"time"
)

// maxStep caps how much time a single Step may integrate.
//
// A window that was hidden, or a machine that went to sleep, hands back
// a delta of seconds. The cap keeps a spring to a sane number of
// substeps and keeps the integration stable.
const maxStep = 100 * time.Millisecond

// Spring moves a value the way a damped weight on a spring moves.
//
// Prefer it for anything the pointer drives. A spring carries its
// velocity across a retarget, so a hover that reverses halfway turns
// around smoothly. Use [Tween] for scripted motion where the exact
// duration matters.
type Spring struct {
	// Response is roughly how long the value takes to arrive, in
	// seconds. Smaller is faster.
	Response float32
	// Damping is the damping ratio. 1 settles straight onto the target,
	// below 1 bounces, above 1 is sluggish.
	Damping float32
}

// Stock springs. Treat them as starting points and retune freely.
var (
	// Snappy is the default for pointer feedback: hover, press, focus.
	Snappy = Spring{Response: 0.25, Damping: 0.85}
	// Gentle settles straight onto the target. Good for things leaving.
	Gentle = Spring{Response: 0.45, Damping: 1.0}
	// Bouncy overshoots visibly. Good for things arriving.
	Bouncy = Spring{Response: 0.40, Damping: 0.58}
)

// Step implements [Motion].
func (sp Spring) Step(s *State, dt time.Duration) bool {
	resp := sp.Response
	if resp <= 0 {
		resp = 0.001
	}
	omega := float32(2*math.Pi) / resp
	zeta := sp.Damping
	if zeta < 0 {
		zeta = 0
	}

	if dt > maxStep {
		dt = maxStep
	}
	s.Elapsed += dt

	// Semi-implicit Euler over fixed 1 ms substeps. It stays stable for
	// every Response a person would actually pick, and it is short
	// enough to read, which matters more here than an analytic
	// solution.
	const h = float32(0.001)
	left := float32(dt.Seconds())
	for left > 0 {
		step := h
		if left < step {
			step = left
		}
		left -= step
		accel := -omega*omega*(s.Position-s.To) - 2*zeta*omega*s.Velocity
		s.Velocity += accel * step
		s.Position += s.Velocity * step
	}

	// Settle relative to the distance travelled, so a spring over 400
	// pixels and one over 0..1 both stop at the point they look still.
	scale := abs32(s.To - s.From)
	if scale < 1 {
		scale = 1
	}
	if abs32(s.Position-s.To) < 0.002*scale && abs32(s.Velocity) < 0.02*scale {
		s.Position = s.To
		s.Velocity = 0
		return true
	}
	return false
}

// An Ease reshapes linear progress in 0..1 into eased progress.
type Ease func(t float32) float32

// Tween moves a value over a fixed duration along an easing curve.
//
// Reach for it when the duration is the point: a scripted intro, motion
// that has to line up with something else. For anything interruptible,
// use [Spring] instead.
type Tween struct {
	Duration time.Duration
	// Ease shapes the curve. A nil Ease means [EaseOut].
	Ease Ease
}

// Step implements [Motion].
func (tw Tween) Step(s *State, dt time.Duration) bool {
	if dt > maxStep {
		dt = maxStep
	}
	s.Elapsed += dt
	if tw.Duration <= 0 {
		s.Position, s.Velocity = s.To, 0
		return true
	}
	p := float32(s.Elapsed) / float32(tw.Duration)
	if p >= 1 {
		s.Position, s.Velocity = s.To, 0
		return true
	}
	ease := tw.Ease
	if ease == nil {
		ease = EaseOut
	}
	prev := s.Position
	s.Position = s.From + (s.To-s.From)*ease(p)
	// Keep velocity current so a spring taking over mid-tween inherits
	// the motion.
	if secs := float32(dt.Seconds()); secs > 0 {
		s.Velocity = (s.Position - prev) / secs
	}
	return false
}

// Easing curves.
var (
	Linear Ease = func(t float32) float32 { return t }

	// EaseOut starts fast and slows into place. It is the right default
	// for interface motion: the result shows up immediately.
	EaseOut Ease = func(t float32) float32 {
		u := 1 - t
		return 1 - u*u*u
	}

	// EaseInOut accelerates and decelerates. Use it for something
	// travelling a long way across the screen.
	EaseInOut Ease = func(t float32) float32 {
		if t < 0.5 {
			return 4 * t * t * t
		}
		u := -2*t + 2
		return 1 - u*u*u/2
	}
)

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
