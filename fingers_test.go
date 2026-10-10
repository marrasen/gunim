package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// fingersOf returns the fingers r heard, without their times.
func fingersOf(r *recorder) []input.Finger {
	var out []input.Finger
	for _, e := range r.events {
		if f, ok := e.(input.Finger); ok {
			f.Time = time.Time{}
			out = append(out, f)
		}
	}
	return out
}

func TestAFingerGoesToTheNodeItCameDownOnWhereverItGoes(t *testing.T) {
	w := newTestWindow()
	left, right := &recorder{}, &recorder{}
	s := &swapped{}
	w.ui.Insert(w.ui.Root(), s)
	w.ui.Insert(s, left)
	w.ui.Insert(s, right)
	run(w, 1)
	hand := func(id int, ph input.FingerPhase, at geom.Point) {
		w.ui.handlePlatform(input.Finger{ID: id, Pos: at, Phase: ph, Time: time.Now()})
	}
	hand(3, input.FingerDown, geom.Pt(150, 40))
	hand(5, input.FingerDown, geom.Pt(20, 30))
	hand(3, input.FingerMove, geom.Pt(60, 45)) // over the left node now
	hand(3, input.FingerUp, geom.Pt(60, 45))
	hand(3, input.FingerMove, geom.Pt(160, 45)) // lifted: goes nowhere
	hand(5, input.FingerUp, input.Away)
	want := []input.Finger{
		{ID: 3, Pos: geom.Pt(50, 40), Phase: input.FingerDown},
		{ID: 3, Pos: geom.Pt(-40, 45), Phase: input.FingerMove},
		{ID: 3, Pos: geom.Pt(-40, 45), Phase: input.FingerUp},
	}
	if got := fingersOf(right); !equalFingers(got, want) {
		t.Errorf("the right node heard %+v, want %+v", got, want)
	}
	wantL := []input.Finger{
		{ID: 5, Pos: geom.Pt(20, 30), Phase: input.FingerDown},
		{ID: 5, Pos: input.Away, Phase: input.FingerUp},
	}
	if got := fingersOf(left); !equalFingers(got, wantL) {
		t.Errorf("the left node heard %+v, want %+v", got, wantL)
	}
}

func TestAFingerOnANodeThatLeavesGoesNowhereMore(t *testing.T) {
	w := newTestWindow()
	r := &recorder{}
	w.ui.Insert(w.ui.Root(), r)
	run(w, 1)
	w.ui.handlePlatform(input.Finger{ID: 1, Pos: geom.Pt(10, 10), Phase: input.FingerDown})
	w.ui.Remove(r)
	run(w, 1)
	w.ui.handlePlatform(input.Finger{ID: 1, Pos: geom.Pt(12, 10), Phase: input.FingerMove})
	if got := fingersOf(r); len(got) != 1 {
		t.Errorf("a node gone heard %+v, want only the finger coming down before it went", got)
	}
	if len(w.ui.fingers) != 0 {
		t.Errorf("the window still sends fingers %v somewhere", w.ui.fingers)
	}
}

func TestTakeFingersReachesTheWindow(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 800), nil)
	if err := w.Client().TakeFingers(true); err != nil {
		t.Fatal(err)
	}
	if !w.Offscreen().FingersTaken() {
		t.Fatal("the window does not take fingers after TakeFingers(true)")
	}
	if err := w.Client().TakeFingers(false); err != nil || w.Offscreen().FingersTaken() {
		t.Fatalf("after TakeFingers(false) the window takes fingers %v (err %v)", w.Offscreen().FingersTaken(), err)
	}
	var _ driver.FingerTaker = w.Offscreen()
}

func equalFingers(a, b []input.Finger) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
