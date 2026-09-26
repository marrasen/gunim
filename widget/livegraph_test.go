package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A running graph slides on every frame, not only as samples come, and
// draws its fill and its head; stopped, it rests.
func TestALiveGraphSlidesEveryFrame(t *testing.T) {
	g := NewLiveGraph(100*time.Millisecond, 50)
	g.Label = func(v float64) string { return "fast" }
	for _, v := range []float64{10, 30, 20, 50, 40} {
		g.Add(v)
	}
	g.SetRunning(true)
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 60)})
	run(30)
	headAt := func() geom.Point {
		t.Helper()
		var at geom.Point
		fills := 0
		for _, op := range w.Offscreen().Ops() {
			r, ok := op.(*paint.RRectOp)
			if !ok {
				continue
			}
			if r.Fill.Gradient != nil {
				fills++
			}
			if r.Radius == 2 {
				at = r.Transform.Apply(r.Rect.Center())
			}
		}
		if fills == 0 {
			t.Fatal("the graph draws no fill")
		}
		return at
	}
	g.since = 0
	w.Frame(time.Second / 60)
	a := headAt()
	w.Frame(time.Second / 60)
	b := headAt()
	if b.X >= a.X {
		t.Fatalf("a frame later the head is at %v, from %v: the graph did not slide", b, a)
	}
	g.SetRunning(false)
	for range 300 {
		if !g.Step(time.Second / 60) {
			return
		}
	}
	t.Fatal("stopped, the graph asks for frames five seconds on")
}
