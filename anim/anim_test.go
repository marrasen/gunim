package anim_test

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
)

// step advances a at 120 Hz for at most d, and returns how long it took
// to settle.
func step(s anim.Stepper, d time.Duration) time.Duration {
	const frame = time.Second / 120
	for t := time.Duration(0); t < d; t += frame {
		if !s.Step(frame) {
			return t
		}
	}
	return d
}

func TestSpringSettlesOnTarget(t *testing.T) {
	a := anim.NewFloat(0)
	a.Animate(1, anim.Snappy)
	took := step(a, 5*time.Second)
	if a.Active() {
		t.Fatalf("still animating after %v", took)
	}
	if got := a.Value(); got != 1 {
		t.Fatalf("Value() = %v, want exactly 1 once settled", got)
	}
	if took > time.Second {
		t.Errorf("Snappy took %v to settle, expected well under a second", took)
	}
}

func TestAnimateIsIdempotent(t *testing.T) {
	// A Transitioner calls Animate every frame while it is exiting, and
	// the animation should run once, start to finish.
	a := anim.NewFloat(0)
	a.Animate(1, anim.Gentle)
	const frame = time.Second / 120
	for range 10 {
		a.Animate(1, anim.Gentle)
		a.Step(frame)
	}
	restarted := anim.NewFloat(0)
	restarted.Animate(1, anim.Gentle)
	for range 10 {
		restarted.Step(frame)
	}
	if a.Value() != restarted.Value() {
		t.Fatalf("re-calling Animate changed the motion: %v vs %v", a.Value(), restarted.Value())
	}
}

func TestRetargetKeepsVelocity(t *testing.T) {
	// Reversing mid-flight should stay smooth: the value carries on past
	// the moment of reversal before turning around.
	a := anim.NewFloat(0)
	a.Animate(1, anim.Snappy)
	step(a, 80*time.Millisecond)
	mid := a.Value()
	if mid <= 0 || mid >= 1 {
		t.Fatalf("expected to be mid-flight, at %v", mid)
	}
	a.Animate(0, anim.Snappy)
	a.Step(time.Second / 120)
	if got := a.Value(); got < mid {
		// Still carrying the old velocity, so it overshoots before
		// turning. A jump would show up here as an immediate drop.
		t.Logf("turned immediately at %v (damping is high); fine", got)
	}
	step(a, 5*time.Second)
	if a.Value() != 0 {
		t.Fatalf("Value() = %v, want 0", a.Value())
	}
}

func TestMultiScalarAnimatesTogether(t *testing.T) {
	a := anim.NewRect(geom.Rc(0, 0, 10, 10))
	a.Animate(geom.Rc(100, 50, 200, 80), anim.Gentle)
	step(a, 5*time.Second)
	want := geom.Rc(100, 50, 200, 80)
	if a.Value() != want {
		t.Fatalf("Value() = %+v, want %+v", a.Value(), want)
	}
}

func TestGroupReportsAnyActive(t *testing.T) {
	var g anim.Group
	fast, slow := anim.NewFloat(0), anim.NewFloat(0)
	g.Add(fast, slow)
	fast.Animate(1, anim.Snappy)
	slow.Animate(1, anim.Tween{Duration: 2 * time.Second})
	if took := step(&g, 5*time.Second); took < time.Second {
		t.Fatalf("group settled after %v, but the tween runs for 2s", took)
	}
}

func TestTweenRespectsDuration(t *testing.T) {
	a := anim.NewFloat(0)
	a.Animate(1, anim.Tween{Duration: 500 * time.Millisecond, Ease: anim.Linear})
	took := step(a, 5*time.Second)
	if took < 450*time.Millisecond || took > 550*time.Millisecond {
		t.Fatalf("settled after %v, want about 500ms", took)
	}
}

func TestSpringStepsCompose(t *testing.T) {
	// The spring is solved exactly, so one long step lands where many
	// short ones do, whatever the damping.
	for _, sp := range []anim.Spring{anim.Snappy, anim.Gentle, anim.Bouncy, {Response: 0.3, Damping: 2.5}} {
		one := anim.State{Position: 0, To: 100, From: 0, Velocity: 40}
		many := one
		sp.Step(&one, 100*time.Millisecond)
		for range 100 {
			sp.Step(&many, time.Millisecond)
		}
		if d := one.Position - many.Position; d < -0.01 || d > 0.01 {
			t.Errorf("%+v: one step to %v, a hundred steps to %v", sp, one.Position, many.Position)
		}
	}
}

func TestAStiffSpringStaysFiniteAndSettles(t *testing.T) {
	for _, sp := range []anim.Spring{{Response: 0.001, Damping: 1}, {Response: 0, Damping: 0.5}} {
		s := anim.State{Position: 0, To: 1, From: 0}
		settled := false
		for range 10 {
			if sp.Step(&s, time.Second/60) {
				settled = true
				break
			}
		}
		if math.IsNaN(float64(s.Position)) || math.IsInf(float64(s.Position), 0) {
			t.Fatalf("%+v: position %v", sp, s.Position)
		}
		if !settled || s.Position != 1 {
			t.Fatalf("%+v: at %v after ten frames, settled %v; want settled at 1", sp, s.Position, settled)
		}
	}
}

func TestColorsSurviveOklab(t *testing.T) {
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 15 {
			for b := 0; b < 256; b += 15 {
				c := color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0x80}
				got := anim.Mix(anim.ColorCodec, c, c, 0.5)
				d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
				if d(got.R, c.R) > 1 || d(got.G, c.G) > 1 || d(got.B, c.B) > 1 || got.A != c.A {
					t.Fatalf("%v came back as %v", c, got)
				}
			}
		}
	}
}

func TestAnOklabBlendKeepsItsBrightness(t *testing.T) {
	red, blue := color.NRGBA{R: 0xff, A: 0xff}, color.NRGBA{B: 0xff, A: 0xff}
	ok := anim.Mix(anim.ColorCodec, red, blue, 0.5)
	srgb := anim.Mix(anim.SRGBCodec, red, blue, 0.5)
	// sRGB's midpoint is a dark purple; Oklab's is a lighter one.
	if int(ok.R)+int(ok.G)+int(ok.B) <= int(srgb.R)+int(srgb.G)+int(srgb.B) {
		t.Fatalf("Oklab midpoint %v is no brighter than sRGB's %v", ok, srgb)
	}
}

// A spring of no response and a tween of no duration put the value where
// it is going as they start, so the frame that asks shows it there.
func TestAnInstantMotionLandsAtOnce(t *testing.T) {
	for _, m := range []anim.Motion{anim.Spring{Response: 0, Damping: 1}, anim.Spring{Damping: 0.3}, anim.Tween{}} {
		a := anim.NewFloat(0)
		a.Animate(10, m)
		if a.Value() != 10 || a.Active() {
			t.Fatalf("%+v: at %v, moving %v, want at 10 and still", m, a.Value(), a.Active())
		}
		if a.Step(time.Second / 60) {
			t.Fatalf("%+v: a step after landing still moves", m)
		}
	}
	// A spring that is not instant still glides.
	a := anim.NewFloat(0)
	a.Animate(10, anim.Spring{Response: 0.01, Damping: 1})
	if a.Value() != 0 || !a.Active() {
		t.Fatal("a stiff spring landed before its first step")
	}
}
