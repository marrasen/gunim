package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// fadeMatches fails unless what s paints in a box of size fades by the first stretch of the content past each edge.
func fadeMatches(t *testing.T, s *scrolling, ops func() (geom.Insets, bool), when string) {
	t.Helper()
	full := ScrollFade.Default()
	at := s.Offset()
	want := geom.Insets{Top: fadeFor(at, full), Bottom: fadeFor(s.end()-at, full)}
	got, _ := ops()
	if got != want {
		t.Fatalf("%s: scrolled %v of %v, the content fades by %v, want %v", when, at, s.end(), got, want)
	}
}

func TestAListFadesWhereMoreRowsLiePastItsEdgesOnEveryFrameOfAFling(t *testing.T) {
	w, l, run := newDragList(t)
	fades := func() (geom.Insets, bool) { return fadeIn(painted(l, geom.Sz(300, 400))) }
	fadeMatches(t, &l.scrolling, fades, "at the top")
	if got, _ := fades(); got.Top != 0 || got.Bottom != ScrollFade.Default() {
		t.Fatalf("at the top the list fades by %v, want only at the bottom", got)
	}
	drag(w, 300, 100, 80*time.Millisecond)
	for f := range 120 {
		run(1)
		fadeMatches(t, &l.scrolling, fades, fmt.Sprint("a fling, frame ", f))
	}
	// Dragged past the top, the list stretches, and fades at the top no more.
	l.JumpTo(0)
	run(2)
	start := time.Now()
	w.Input(input.PointerDown{Pos: geom.Pt(100, 100), Clicks: 1, Time: start})
	w.Input(input.PointerMove{Pos: geom.Pt(100, 300), Time: start.Add(time.Second)})
	run(1)
	if l.Offset() >= 0 {
		t.Fatalf("a drag down from the top left the list at %v, want stretched past it", l.Offset())
	}
	fadeMatches(t, &l.scrolling, fades, "stretched past the top")
}

func TestNoFadeKeepsAListFromFading(t *testing.T) {
	_, l, run := newDragList(t)
	l.NoFade = true
	l.ScrollTo(400, Quick.Default())
	run(40)
	if got, ok := fadeIn(painted(l, geom.Sz(300, 400))); ok {
		t.Fatalf("a list with NoFade fades by %v", got)
	}
}

func TestAScrollFadesWhereItsContentLiesPastItsEdges(t *testing.T) {
	s := NewScroll(newSpot(300, 2000))
	w, run := stage(t, &frame{child: s, size: geom.Sz(300, 400)})
	w.Input(input.Scroll{Pos: geom.Pt(150, 200), Delta: geom.Pt(0, -300)})
	fades := func() (geom.Insets, bool) { return fadeIn(w.Offscreen().Ops()) }
	for f := range 40 {
		run(1)
		fadeMatches(t, &s.scrolling, fades, fmt.Sprint("a wheel, frame ", f))
	}
	if got, _ := fades(); got.Top != ScrollFade.Default() || got.Bottom != ScrollFade.Default() {
		t.Fatalf("in the middle the scroll fades by %v, want both edges", got)
	}
	// Content that fits does not fade.
	short := NewScroll(newSpot(300, 100))
	sw, _ := stage(t, &frame{child: short, size: geom.Sz(300, 400)})
	if got, ok := fadeIn(sw.Offscreen().Ops()); ok {
		t.Fatalf("a scroll whose content fits fades by %v", got)
	}
}
