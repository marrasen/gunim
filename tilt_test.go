package gunim

import (
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// tilted returns a stage whose child, at 10, 10, 100 by 50, is turned
// by tilt, and where a point of the child's own space shows.
func tilted(t *testing.T, tilt paint.Tilt) (*Window, *recorder, func(geom.Point) geom.Point) {
	t.Helper()
	w, st, r := newStage(t, paint.Identity)
	st.tilt = tilt
	run(w, 1)
	h := tilt.Homography(geom.Pt(60, 35))
	return w, r, func(p geom.Point) geom.Point {
		at, _ := h.Apply(p.Add(geom.Pt(10, 10)))
		return at
	}
}

func TestATapLandsWhereATiltedChildShows(t *testing.T) {
	w, r, shows := tilted(t, paint.Tilt{X: 0.4, Y: 0.9, Distance: 300})
	for _, want := range []geom.Point{{X: 80, Y: 20}, {X: 5, Y: 45}, {X: 95, Y: 3}} {
		at := shows(want)
		press(w, at.X, at.Y)
		if got := pressedAt(t, r); math.Abs(float64(got.X-want.X)) > 1e-3 || math.Abs(float64(got.Y-want.Y)) > 1e-3 {
			t.Errorf("a tap at %v, where the child's %v shows, reached it at %v", at, want, got)
		}
	}
	got, ok := w.ui.Convert(geom.Pt(80, 20), r, w.ui.Root())
	if want := shows(geom.Pt(80, 20)); !ok || math.Abs(float64(got.X-want.X)) > 1e-3 || math.Abs(float64(got.Y-want.Y)) > 1e-3 {
		t.Errorf("the child's 80, 20 converts to %v in the window, want %v, where it shows", got, want)
	}
}

func TestATapMissesWhereATiltedChildWasFlat(t *testing.T) {
	// Turned well away on its right, the child's right end no longer
	// reaches where it lay flat.
	w, r, shows := tilted(t, paint.Tilt{Y: 1.2, Distance: 300})
	if end := shows(geom.Pt(100, 25)); end.X >= 105 {
		t.Fatalf("the right end shows at %v, want it drawn in from 110", end)
	}
	press(w, 105, 35)
	if len(r.events) != 0 {
		t.Fatalf("a tap where the tilted child lay flat reached it: %v", r.events)
	}
}

func TestAOneSidedChildShowingItsBackTakesNoTaps(t *testing.T) {
	w, r, _ := tilted(t, paint.Tilt{Y: math.Pi * 0.8, OneSided: true})
	for x := float32(12); x < 110; x += 8 {
		press(w, x, 35)
	}
	if len(r.events) != 0 {
		t.Fatalf("a one-sided child turned round took taps: %v", r.events)
	}
}
