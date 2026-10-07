package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// glides asserts on every frame of a second that got leaves from and settles on to: the first frame shows it part
// way, each frame before it reaches to moves on toward it, and the last is at to. A bounce may carry it past to and back.
func glides(t *testing.T, run func(int), what string, from, to float32, got func() float32) {
	t.Helper()
	near := func(a, b float32) bool { return abs32(a-b) < 1e-3 }
	prev, past := from, false
	for i := range 60 {
		run(1)
		g := got()
		if i == 0 && (near(g, from) || near(g, to)) {
			t.Fatalf("frame 0: %s is at %v, want part way from %v to %v", what, g, from, to)
		}
		past = past || (g-to)*(from-to) <= 0
		if !past && abs32(g-to) > abs32(prev-to)+1e-4 {
			t.Fatalf("frame %d: %s went from %v back to %v, away from %v", i, what, prev, g, to)
		}
		prev = g
	}
	if !near(prev, to) {
		t.Fatalf("after a second %s is at %v, want %v", what, prev, to)
	}
}

// jumps asserts on every frame of a second that got is at want.
func jumps(t *testing.T, run func(int), what string, want float32, got func() float32) {
	t.Helper()
	for i := range 60 {
		run(1)
		if g := got(); abs32(g-want) > 1e-4 {
			t.Fatalf("frame %d: %s is at %v, want %v at once", i, what, g, want)
		}
	}
}

// quiet fails when the window sent the application anything.
func quiet(t *testing.T, w *gunim.Window, what string) {
	t.Helper()
	if got := sent(w); len(got) != 0 {
		t.Fatalf("%s sent %v, want nothing", what, got)
	}
}

func TestSetCheckedGlidesOnceLaidOutAndJumpsWithoutAUI(t *testing.T) {
	c := NewCheckbox("Tick")
	c.OnChange = func(on bool) gunim.Intent { return flipped{on} }
	w, run := stage(t, &frame{child: c, size: geom.Sz(200, 28)})
	u := stageUI(t, w, run)
	c.SetChecked(true, u)
	if !c.Checked() {
		t.Fatal("SetChecked(true) left the box unticked")
	}
	glides(t, run, "the tick", 0, 1, c.lit.Value)
	c.SetChecked(false, nil)
	jumps(t, run, "the tick set with no UI", 0, c.lit.Value)
	quiet(t, w, "SetChecked")
}
