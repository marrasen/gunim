package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// The head glides to each new sample, even when samples swing hard and
// come unevenly: from frame to frame it stays at the right edge and moves
// only a little up or down, and the curve slides on at an even pace.
func TestALiveGraphHeadGlidesToEachSample(t *testing.T) {
	g := NewLiveGraph(200*time.Millisecond, 50)
	g.SetRunning(true)
	box := geom.Sz(400, 56)
	const frame = time.Second / 60
	// Samples between 150 and 260 milliseconds apart, swinging from 10
	// to 100 and back.
	gaps := []int{12, 13, 15, 11, 14, 12, 9, 16, 12, 13}
	values := []float64{10, 100, 10, 100, 50, 100, 10, 80, 20, 100}
	for i := range 4 {
		g.Add(values[i])
		for range gaps[i] {
			g.Step(frame)
		}
	}
	was := g.head(box, 14)
	wasPos := g.pos
	var slides []float64
	for range 3 {
		for i, v := range values {
			g.Add(v)
			for range gaps[i] {
				g.Step(frame)
				now := g.head(box, 14)
				if now.X != was.X {
					t.Fatalf("the head moved across from %v to %v", was, now)
				}
				// The plot is 41 pixels tall, and a swing crosses most of
				// it: a jump would cross much of that in one frame.
				if d := now.Y - was.Y; d > 3 || d < -3 {
					t.Fatalf("after sample %d the head jumped from %v to %v in a frame", i, was, now)
				}
				slides = append(slides, g.pos-wasPos)
				was, wasPos = now, g.pos
			}
		}
	}
	for i, s := range slides {
		if s <= 0 {
			t.Fatalf("frame %d: the curve stopped sliding", i)
		}
		if i > 0 {
			if d := s - slides[i-1]; d > 0.02 || d < -0.02 {
				t.Fatalf("frame %d: the slide lurched from %v to %v samples a frame", i, slides[i-1], s)
			}
		}
	}
	g.SetRunning(false)
	for range 300 {
		// Shown, as a paint marks it.
		g.painted = true
		if !g.Step(frame) {
			if g.pos != float64(len(g.samples)-1) {
				t.Fatalf("at rest the head is at sample %v, not on the newest", g.pos)
			}
			return
		}
	}
	t.Fatal("stopped, the graph asks for frames five seconds on")
}

// A running graph slides on every frame, not only as samples come, and
// draws its fill and its head; stopped, it rests.
func TestALiveGraphCutsItsLabelToANarrowBox(t *testing.T) {
	g := NewLiveGraph(100*time.Millisecond, 50)
	g.Label = func(v float64) string { return "123 456 789 bytes a second" }
	for _, v := range []float64{10, 30, 20, 50, 40} {
		g.Add(v)
	}
	g.SetRunning(true)
	w, run := stage(t, &frame{child: g, size: geom.Sz(60, 60)})
	for range 30 {
		run(1)
		texts := 0
		for _, op := range w.Offscreen().Ops() {
			if tx, ok := op.(*paint.TextOp); ok {
				texts++
				for _, gl := range tx.Glyphs {
					if x := tx.Transform.Apply(gl.At).X; x < 0 || x > 60 {
						t.Fatalf("a glyph of the label is at x %v, outside the graph's 60 px", x)
					}
				}
			}
		}
		if texts == 0 {
			t.Fatal("the graph drew no label")
		}
	}
}

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
	g.Add(30)
	w.Frame(time.Second / 60)
	headAt()
	a := g.pos
	w.Frame(time.Second / 60)
	if g.pos <= a {
		t.Fatalf("a frame later the head is at sample %v, from %v: the graph did not slide", g.pos, a)
	}
	g.SetRunning(false)
	for range 300 {
		if !g.Step(time.Second / 60) {
			return
		}
	}
	t.Fatal("stopped, the graph asks for frames five seconds on")
}
