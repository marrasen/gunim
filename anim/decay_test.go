package anim

import (
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
