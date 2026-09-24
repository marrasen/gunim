package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// stage lays its one child out at (10, 10), 100 by 50, and paints it
// under a transform of its own, the way a dialog scales its contents or
// a drawer slides its panel.
type stage struct {
	t    paint.Transform
	skip bool
	// clip, when set, clips the child through a layer, rounded by
	// radius, in the stage's space under t.
	clip   geom.Rect
	radius float32
}

func (s *stage) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	for kid := range kids.All {
		kid.Layout(Tight(geom.Sz(100, 50)))
		kid.Place(geom.Pt(10, 10))
	}
	return c.Max
}

func (s *stage) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	if s.skip {
		return
	}
	defer p.Push(s.t)()
	if !s.clip.Empty() {
		defer p.Layer(paint.LayerOpts{Bounds: s.clip, Clip: true, Radius: s.radius, Opacity: 1})()
	}
	for kid := range kids.All {
		kid.Paint(p)
	}
}

func newStage(t *testing.T, tr paint.Transform) (*Window, *stage, *recorder) {
	t.Helper()
	w := newTestWindow()
	st := &stage{t: tr}
	r := &recorder{}
	w.ui.Insert(w.ui.Root(), st)
	w.ui.Insert(st, r)
	run(w, 1)
	return w, st, r
}

func press(w *Window, x, y float32) {
	w.ui.handlePlatform(input.PointerDown{Pos: geom.Pt(x, y), Time: time.Now()})
}

// pressedAt returns where the recorder last saw a press, in its own
// space.
func pressedAt(t *testing.T, r *recorder) geom.Point {
	t.Helper()
	for i := len(r.events) - 1; i >= 0; i-- {
		if d, ok := r.events[i].(input.PointerDown); ok {
			return d.Pos
		}
	}
	t.Fatal("the node saw no press")
	return geom.Point{}
}

func near(a, b geom.Point) bool {
	d := a.Sub(b)
	return d.X*d.X+d.Y*d.Y < 1e-6
}

func TestClickLandsWhereAScaledChildIsDrawn(t *testing.T) {
	// Drawn at twice its size, the child covers (20, 20) to (220, 120).
	// (150, 100) lies inside that, and outside where layout put it.
	w, _, r := newStage(t, paint.Scale(2, geom.Point{}))
	press(w, 150, 100)
	if got, want := pressedAt(t, r), geom.Pt(65, 40); !near(got, want) {
		t.Fatalf("press at %v in the child's space, want %v", got, want)
	}
}

func TestClickFollowsAChildSlidAside(t *testing.T) {
	w, _, r := newStage(t, paint.Translate(geom.Pt(300, 0)))
	press(w, 50, 30)
	if len(r.events) != 0 {
		t.Fatalf("a click where the child was laid out reached it: %v", r.events)
	}
	press(w, 350, 30)
	if got, want := pressedAt(t, r), geom.Pt(40, 20); !near(got, want) {
		t.Fatalf("press at %v in the child's space, want %v", got, want)
	}
}

func TestClickFollowsARotatedChild(t *testing.T) {
	// A quarter turn about the origin maps (x, y) to (-y, x), and a
	// translation brings it back on screen: the child covers x 190..240
	// and y 10..110.
	quarter := paint.Transform{A: 0, B: -1, C: 250, D: 1, E: 0, F: 0}
	w, _, r := newStage(t, quarter)
	press(w, 230, 60)
	if got, want := pressedAt(t, r), geom.Pt(50, 10); !near(got, want) {
		t.Fatalf("press at %v in the child's space, want %v", got, want)
	}
}

func TestUnpaintedChildTakesNoClicks(t *testing.T) {
	w, st, r := newStage(t, paint.Identity)
	st.skip = true
	run(w, 1)
	press(w, 50, 30)
	if len(r.events) != 0 {
		t.Fatalf("a child left unpainted took a click: %v", r.events)
	}
}

func TestChildScaledToNothingTakesNoClicks(t *testing.T) {
	w, _, r := newStage(t, paint.Scale(0, geom.Pt(60, 35)))
	press(w, 60, 35)
	if len(r.events) != 0 {
		t.Fatalf("a child scaled to nothing took a click: %v", r.events)
	}
}

func TestHoverEntersAtThePointOnTheTransformedChild(t *testing.T) {
	w, _, r := newStage(t, paint.Scale(2, geom.Point{}))
	w.ui.handlePlatform(input.PointerMove{Pos: geom.Pt(150, 100), Time: time.Now()})
	for _, e := range r.events {
		if in, ok := e.(input.PointerEnter); ok {
			if !near(in.Pos, geom.Pt(65, 40)) {
				t.Fatalf("PointerEnter at %v, want (65, 40)", in.Pos)
			}
			return
		}
	}
	t.Fatalf("no PointerEnter; saw %v", r.events)
}

func newClippedStage(t *testing.T, tr paint.Transform, clip geom.Rect, radius float32) (*Window, *recorder) {
	t.Helper()
	w, st, r := newStage(t, tr)
	st.clip, st.radius = clip, radius
	run(w, 1)
	return w, r
}

func TestClickOutsideAClipMisses(t *testing.T) {
	// The clip shows the child's left half, x 10..60.
	w, r := newClippedStage(t, paint.Identity, geom.Rc(10, 10, 50, 50), 0)
	press(w, 90, 30)
	if len(r.events) != 0 {
		t.Fatalf("a click on the clipped-away half reached the child: %v", r.events)
	}
	press(w, 40, 30)
	if got, want := pressedAt(t, r), geom.Pt(30, 20); !near(got, want) {
		t.Fatalf("press at %v in the child's space, want %v", got, want)
	}
}

func TestClickInARoundedClipsCornerMisses(t *testing.T) {
	w, r := newClippedStage(t, paint.Identity, geom.Rc(10, 10, 100, 50), 20)
	press(w, 12, 12)
	if len(r.events) != 0 {
		t.Fatalf("a click in the rounded-off corner reached the child: %v", r.events)
	}
	press(w, 60, 35)
	if len(r.events) == 0 {
		t.Fatal("a click in the middle of the clip missed the child")
	}
}

func TestClipFollowsTheTransformItWasDrawnUnder(t *testing.T) {
	// At double scale the clip covers window x 20..120; the child
	// covers 20..220.
	w, r := newClippedStage(t, paint.Scale(2, geom.Point{}), geom.Rc(10, 10, 50, 50), 0)
	press(w, 150, 50)
	if len(r.events) != 0 {
		t.Fatalf("a click outside the scaled clip reached the child: %v", r.events)
	}
	press(w, 100, 50)
	if got, want := pressedAt(t, r), geom.Pt(40, 15); !near(got, want) {
		t.Fatalf("press at %v in the child's space, want %v", got, want)
	}
}
