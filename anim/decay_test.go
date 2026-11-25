package anim

import (
	"math"
	"testing"
	"time"
)

func TestFlingComesToRestWhereItAimed(t *testing.T) {
	f := NewFloat(0)
	Fling(f, 1000, Decay{Tau: 0.3})
	aim := f.Target()
	for f.Step(time.Second / 60) {
	}
	if v := f.Value(); v < aim-10 || v > aim+10 || aim != 300 {
		t.Fatalf("came to rest at %v aiming for %v, want about 300", v, aim)
	}
}

func TestAFlingTurnsIntoABounce(t *testing.T) {
	f := NewFloat(0)
	Fling(f, 1000, Decay{Tau: 0.3})
	for range 6 {
		f.Step(time.Second / 60)
	}
	// Past an edge at 50: a spring back to it carries the velocity on,
	// so the value goes further before it turns.
	past := f.Value()
	f.Retarget(50, Spring{Response: 0.4, Damping: 1})
	f.Step(time.Second / 60)
	if f.Value() <= past {
		t.Fatalf("the bounce turned at once, from %v to %v", past, f.Value())
	}
	for f.Step(time.Second / 60) {
	}
	if v := f.Value(); v < 49.9 || v > 50.1 {
		t.Fatalf("settled at %v, want 50", v)
	}
}

// TestAKickSwingsOutAndBack: each time, from rest and from a swing, wherever it rests.
func TestAKickSwingsOutAndBack(t *testing.T) {
	sp := Spring{Response: 0.4, Damping: 0.4}
	for _, rest := range []float32{0, 5} {
		f := NewFloat(rest)
		for turn := range 3 {
			at := f.Value()
			Kick(f, 2*math.Pi/0.4, sp)
			if f.Value() != at || f.Target() != rest || !f.Active() {
				t.Fatalf("kick %d moved it from %v to %v at once, heading for %v", turn, at, f.Value(), f.Target())
			}
			most, frames := float32(0), 0
			for f.Step(time.Second / 60) {
				// The third comes as the second swings.
				frames++
				if turn == 1 && frames == 8 {
					break
				}
				most = max(most, f.Value()-rest)
			}
			if most < 0.4 || most > 1 {
				t.Errorf("kick %d, resting at %v, swung out %v, want under 1 and more than 0.4", turn, rest, most)
			}
		}
		if f.Value() != rest || f.Active() {
			t.Errorf("it came to rest at %v, want %v", f.Value(), rest)
		}
	}
}

func TestAKickWithNoMotionStaysWhereItIs(t *testing.T) {
	f := NewFloat(3)
	f.Animate(4, Gentle)
	Kick(f, 10, Spring{})
	if f.Value() != 4 || f.Active() {
		t.Errorf("under a spring that takes no time it is at %v, moving %v", f.Value(), f.Active())
	}
	scaledTo(t, 0)
	Kick(f, 10, Gentle)
	if f.Value() != 4 || f.Active() {
		t.Errorf("with animation off it is at %v, moving %v", f.Value(), f.Active())
	}
}

// TestHeadingWhereItIsLeavesNoSpeed: for what starts next to carry on.
func TestHeadingWhereItIsLeavesNoSpeed(t *testing.T) {
	f, want := NewFloat(0), NewFloat(0)
	Fling(f, -500, Decay{})
	f.Retarget(0, Gentle)
	f.Animate(1, Gentle)
	want.Animate(1, Gentle)
	for range 20 {
		f.Step(time.Second / 60)
		want.Step(time.Second / 60)
		if f.Value() != want.Value() {
			t.Fatalf("it is at %v, want %v", f.Value(), want.Value())
		}
	}
}
