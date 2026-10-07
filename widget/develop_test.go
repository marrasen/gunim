package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestAMonotoneCurveGoesThroughItsPointsWithoutOvershooting(t *testing.T) {
	straight := []geom.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}
	for _, x := range []float32{0, 0.25, 0.5, 1} {
		if y := MonotoneCurve(straight, x); y < x-1e-5 || y > x+1e-5 {
			t.Fatalf("the straight curve at %v is %v", x, y)
		}
	}
	// A steep rise then a plateau: a plain cubic would overshoot past the
	// plateau's height.
	pts := []geom.Point{{X: 0.1, Y: 0.05}, {X: 0.3, Y: 0.8}, {X: 0.6, Y: 0.82}, {X: 1, Y: 1}}
	for _, p := range pts {
		if y := MonotoneCurve(pts, p.X); y < p.Y-1e-5 || y > p.Y+1e-5 {
			t.Fatalf("the curve misses its point %v: %v", p, y)
		}
	}
	prev := float32(-1)
	for i := range 101 {
		x := float32(i) / 100
		y := MonotoneCurve(pts, x)
		if y < prev-1e-6 {
			t.Fatalf("the curve falls at %v, from %v to %v", x, prev, y)
		}
		if x > 0.3 && x < 0.6 && (y < 0.8-1e-5 || y > 0.82+1e-5) {
			t.Fatalf("the curve overshoots between 0.3 and 0.6: %v at %v", y, x)
		}
		prev = y
	}
	if y := MonotoneCurve(pts, 0.02); y != 0.05 {
		t.Fatalf("before the first point the curve is %v, want flat at 0.05", y)
	}
}

// do runs fn on the window's UI goroutine, as a view's patch function
// runs, through a patch for the stage.
type doPatch struct{ fn func(*gunim.UI) }

func do(t *testing.T, w *gunim.Window, fn func(*gunim.UI)) {
	t.Helper()
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, p doPatch, u *gunim.UI) { p.fn(u) })
	if err := w.Client().Patch("stage", doPatch{fn}); err != nil {
		t.Fatal(err)
	}
}

type curved struct{ pts []geom.Point }
type curveDone struct{ pts []geom.Point }

func TestAToneCurveAddsDragsAndRemovesPoints(t *testing.T) {
	c := NewToneCurve()
	c.OnChange = func(pts []geom.Point) gunim.Intent { return curved{pts} }
	c.OnCommit = func(pts []geom.Point) gunim.Intent { return curveDone{pts} }
	w, run := stage(t, &frame{child: c, size: geom.Sz(212, 212)})
	run(1)
	// The plot runs from 6 to 206 each way, 1 up at the top.
	at := func(x, y float32) geom.Point { return geom.Pt(6+x*200, 206-y*200) }
	w.Input(input.PointerDown{Pos: at(0.5, 0.5), Clicks: 1})
	w.Input(input.PointerMove{Pos: at(0.5, 0.7)})
	w.Input(input.PointerUp{Pos: at(0.5, 0.7)})
	run(30)
	pts := c.Points()
	if len(pts) != 3 || pts[1].X < 0.49 || pts[1].X > 0.51 || pts[1].Y < 0.69 || pts[1].Y > 0.71 {
		t.Fatalf("a press and a drag up left the points %v; want one added and lifted to 0.7", pts)
	}
	got := sent(w)
	if _, ok := got[len(got)-1].(curveDone); !ok {
		t.Fatalf("letting go sent %v last; want the commit", got[len(got)-1])
	}
	// An end moves up and down only.
	w.Input(input.PointerDown{Pos: at(1, 1), Clicks: 1})
	w.Input(input.PointerMove{Pos: at(0.8, 0.9)})
	w.Input(input.PointerUp{Pos: at(0.8, 0.9)})
	run(1)
	if end := c.Points()[2]; end.X != 1 || end.Y < 0.89 || end.Y > 0.91 {
		t.Fatalf("the end point went to %v; want it at the end, lowered to 0.9", end)
	}
	// A point between the ends keeps clear of its neighbours across.
	w.Input(input.PointerDown{Pos: at(0.5, 0.7), Clicks: 1})
	w.Input(input.PointerMove{Pos: at(1.2, 0.7)})
	w.Input(input.PointerUp{Pos: at(1.2, 0.7)})
	run(1)
	if mid := c.Points()[1]; mid.X > 0.98+1e-5 {
		t.Fatalf("the middle point went to %v, past its neighbour", mid)
	}
	// A double click on it removes it, and the curve settles without it.
	mid := c.Points()[1]
	w.Input(input.PointerDown{Pos: at(mid.X, mid.Y), Clicks: 1})
	w.Input(input.PointerUp{Pos: at(mid.X, mid.Y)})
	w.Input(input.PointerDown{Pos: at(mid.X, mid.Y), Clicks: 2})
	w.Input(input.PointerUp{Pos: at(mid.X, mid.Y)})
	run(2)
	if n := len(c.Points()); n != 2 {
		t.Fatalf("a double click left %d points, want 2", n)
	}
	if len(c.gone) != 1 {
		t.Fatal("the removed point did not shrink away")
	}
	run(60)
	if len(c.gone) != 0 {
		t.Fatal("the removed point stayed after shrinking")
	}
}

func TestAToneCurveSetFromOutsideMorphs(t *testing.T) {
	c := NewToneCurve()
	w, run := stage(t, &frame{child: c, size: geom.Sz(212, 212)})
	run(1)
	do(t, w, func(u *gunim.UI) { c.SetPoints([]geom.Point{{X: 0, Y: 0}, {X: 0.5, Y: 0.8}, {X: 1, Y: 1}}, u) })
	run(2)
	mid := c.shown()[curveSamples/2]
	if mid <= 0.5+1e-4 || mid >= 0.8-1e-4 {
		t.Fatalf("two frames in, the middle is at %v; want on its way", mid)
	}
	run(60)
	if mid := c.shown()[curveSamples/2]; mid < 0.799 || mid > 0.801 {
		t.Fatalf("the curve settled with its middle at %v, want 0.8", mid)
	}
}

func TestAHistogramRisesToItsCounts(t *testing.T) {
	h := NewHistogram()
	w, run := stage(t, &frame{child: h, size: geom.Sz(256, 64)})
	run(1)
	var counts [3][256]uint32
	for i := range 256 {
		counts[0][i] = uint32(i / 4)
		counts[1][i] = 100
		counts[2][i] = 25
	}
	counts[0][255] = 1e6 // a clipped highlight leaves the rest's scale be
	do(t, w, func(u *gunim.UI) { h.SetCounts(counts, u) })
	run(2)
	if g := h.shown()[1][100]; g <= 0 || g >= 0.99 {
		t.Fatalf("two frames in, green is at %v; want rising", g)
	}
	run(60)
	s := h.shown()
	if g := s[1][100]; g < 0.99 {
		t.Fatalf("green, the tallest away from the ends, settled at %v; want 1", g)
	}
	if b := s[2][100]; b < 0.49 || b > 0.51 {
		t.Fatalf("blue, a quarter of green's count, settled at %v; want its square root, 0.5", b)
	}
	if r := s[0][255]; r != 1 {
		t.Fatalf("the clipped end shows %v; want full height", r)
	}
}

func TestASliderRowCountsAlongAndResets(t *testing.T) {
	s := NewSlider(-100, 100)
	s.Snap = 1
	s.Rest, s.HasRest = 0, true
	s.Set(80)
	s.OnCommit = func(v float32) gunim.Intent { return committed{v} }
	row := NewSliderRow("Contrast", s)
	w, run := stage(t, &frame{child: row, size: geom.Sz(300, 28)})
	run(30)
	if row.value.Text != "80" {
		t.Fatalf("the readout says %q, want 80", row.value.Text)
	}
	// The reset mark is at the right end; a click there glides back.
	c := row.reset.Center()
	w.Input(input.PointerDown{Pos: c, Clicks: 1})
	w.Input(input.PointerUp{Pos: c})
	run(3)
	if s.Value() != 0 {
		t.Fatalf("the reset left %v", s.Value())
	}
	if row.value.Text == "0" || row.value.Text == "80" {
		t.Fatalf("three frames in, the readout says %q; want it counting down", row.value.Text)
	}
	run(60)
	if row.value.Text != "0" {
		t.Fatalf("the readout settled at %q, want 0", row.value.Text)
	}
	got := sent(w)
	if last, ok := got[len(got)-1].(committed); !ok || last.v != 0 {
		t.Fatalf("the reset sent %v, want a commit of 0", got)
	}
}

func TestASliderRowMarkedActiveShowsItGrowingIn(t *testing.T) {
	row := NewSliderRow("Contrast", NewSlider(-1, 1))
	w, run := stage(t, &frame{child: row, size: geom.Sz(300, 28)})
	run(1)
	do(t, w, func(u *gunim.UI) { row.SetActive(true, u) })
	run(2)
	if k := row.active.Value(); k <= 0 || k >= 1 || !row.Active() {
		t.Fatalf("two frames after SetActive the mark is %v; want growing in", k)
	}
	run(60)
	if row.active.Value() < 0.99 {
		t.Fatal("the mark did not settle in")
	}
	do(t, w, func(u *gunim.UI) { row.SetActive(false, u) })
	run(60)
	if row.active.Value() > 0.01 || row.Active() {
		t.Fatal("the mark did not fade out")
	}
}
