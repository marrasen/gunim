package gunim

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
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

// A drag held on a tilted child goes on past the child's horizon, where
// no point of it shows: at every step the child hears a point it can
// use, as far out as it was or further, the way the pointer went.
func TestADragRunsOnPastATiltedChildsHorizon(t *testing.T) {
	for _, c := range []struct {
		name string
		tilt paint.Tilt
		// step is the pointer's move each frame, and along the child's
		// own axis it should follow.
		step  geom.Point
		along func(geom.Point) float32
	}{
		{"turned on its side, dragged right", paint.Tilt{Y: 1.0, Distance: 300}, geom.Pt(10, 0), func(p geom.Point) float32 { return p.X }},
		{"tipped back, dragged up", paint.Tilt{X: 1.0, Distance: 300}, geom.Pt(0, -10), func(p geom.Point) float32 { return -p.Y }},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, r, shows := tilted(t, c.tilt)
			at := shows(geom.Pt(50, 25))
			press(w, at.X, at.Y)
			last := c.along(pressedAt(t, r))
			for i := range 60 {
				at = at.Add(c.step)
				w.ui.handlePlatform(input.PointerMove{Pos: at, Time: time.Now()})
				run(w, 1)
				m, ok := r.events[len(r.events)-1].(input.PointerMove)
				if !ok {
					t.Fatalf("step %d, to %v: the child heard %T, want a move", i, at, r.events[len(r.events)-1])
				}
				got := c.along(m.Pos)
				if math.IsInf(float64(m.Pos.X), 0) || math.IsInf(float64(m.Pos.Y), 0) || math.IsNaN(float64(got)) {
					t.Fatalf("step %d, to %v: the child heard %v", i, at, m.Pos)
				}
				if got < last-1e-4*max(1, float32(math.Abs(float64(last)))) {
					t.Fatalf("step %d, to %v: the child heard %v, back from %v", i, at, m.Pos, last)
				}
				last = got
			}
			if last < 1000 {
				t.Errorf("past the horizon the drag reached only %v along the child", last)
			}
		})
	}
}
