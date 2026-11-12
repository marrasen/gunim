package widget

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A graph filled with a span of samples at once shows them at once: the head is on the newest, at the right edge,
// the history is drawn to its left across the width, and the label says the newest second. As samples then come at
// their pace, the head keeps up with them rather than replaying the history.
func TestAFilledLiveGraphStartsAtTheNewestSample(t *testing.T) {
	const every = 300 * time.Millisecond
	g := NewLiveGraph(every, 60)
	var said float64
	g.Label = func(v float64) string { said = v; return "rate" }
	// Two minutes of old traffic, busy at first, quiet in the last few seconds.
	history := make([]float64, 400)
	for i := range history {
		history[i] = 100
		if i >= len(history)-10 {
			history[i] = 5
		}
	}
	g.Fill(history)
	g.SetRunning(true)
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 60)})
	run(1)

	newest := float64(len(g.samples) - 1)
	if g.pos != newest {
		t.Fatalf("after Fill the head is at sample %v of %v; want it on the newest", g.pos, newest)
	}
	if math.Abs(said-5) > 1e-9 {
		t.Fatalf("after Fill the label says %v, want the newest second's 5, not the old traffic", said)
	}
	left, right := float32(math.Inf(1)), float32(math.Inf(-1))
	var head geom.Point
	for _, op := range w.Offscreen().Ops() {
		r, ok := op.(*paint.RRectOp)
		if !ok {
			continue
		}
		if r.Fill.Gradient != nil {
			left = min(left, r.Transform.Apply(r.Rect.Min).X)
			right = max(right, r.Transform.Apply(r.Rect.Max).X)
		}
		if r.Radius == 2 {
			head = r.Transform.Apply(r.Rect.Center())
		}
	}
	if left > 1 || right < 390 {
		t.Fatalf("the history is drawn from x %v to %v; want it across the 400 px width at once", left, right)
	}
	if head.X < 390 {
		t.Fatalf("the head is at x %v; want it at the right edge", head.X)
	}

	// Ten seconds of samples at their pace: the head stays within a couple of samples of the newest.
	for range 33 {
		g.Add(5)
		for range 18 {
			w.Frame(time.Second / 60)
		}
	}
	if lag := float64(len(g.samples)-1) - g.pos; lag > 2 {
		t.Fatalf("ten seconds on, the head is %v samples behind the newest; want it to keep up", lag)
	}
}

// Fill takes the place of what the graph held, keeps only what fits, and clears the graph when empty.
func TestFillReplacesALiveGraphsHistory(t *testing.T) {
	g := NewLiveGraph(100*time.Millisecond, 50)
	for _, v := range []float64{1, 2, 3} {
		g.Add(v)
	}
	g.Fill([]float64{-4, 7, 9})
	if len(g.samples) != 3 || g.samples[0] != 0 || g.samples[2] != 9 {
		t.Fatalf("after Fill the graph holds %v; want 0, 7, 9", g.samples)
	}
	big := make([]float64, 500)
	g.Fill(big)
	if len(g.samples) != g.Span+2 {
		t.Fatalf("filled with 500 samples the graph keeps %d; want the %d that fit", len(g.samples), g.Span+2)
	}
	g.Fill(nil)
	if len(g.samples) != 0 || g.pos != 0 {
		t.Fatalf("an empty Fill leaves %d samples, the head at %v", len(g.samples), g.pos)
	}
}
