package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

type splitMoved struct{ Share float32 }

// newSplitStage mounts a text field and a block split side by side in
// a 406 by 300 window: 200 each, with 6 between them.
func newSplitStage(t *testing.T) (*gunim.Window, *Split, func(int)) {
	t.Helper()
	s := NewSplit(NewTextField(), &block{h: 10})
	s.OnMove = func(v float32) gunim.Intent { return splitMoved{v} }
	w := gunimtest.New(t, geom.Sz(406, 300), nil)
	gunim.RegisterView(w, "split", func(struct{}) gunim.Node { return s }, nil)
	if err := w.Client().Mount(gunim.Root, "split", "split", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, s, run
}

func TestASplitGivesEachPaneItsShare(t *testing.T) {
	_, s, _ := newSplitStage(t)
	if a := s.firstLength(); a != 200 {
		t.Fatalf("the first pane is %v wide, want 200", a)
	}
	_, s2, run := newSplitStage(t)
	s2.Vertical = true
	s2.SetShare(0.25, nil)
	run(1)
	// 300 tall, less the gap, a quarter of it.
	if a := s2.firstLength(); a != 74 {
		t.Fatalf("the top pane is %v tall, want 74", a)
	}
}

func TestDraggingTheDividerMovesItAndSaysSo(t *testing.T) {
	w, s, run := newSplitStage(t)
	now := time.Now()
	w.Input(input.PointerDown{Pos: geom.Pt(202, 100), Clicks: 1, Time: now})
	for x := float32(186); x >= 100; x -= 16 {
		w.Input(input.PointerMove{Pos: geom.Pt(x, 100), Time: now})
		run(1)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(100, 100), Time: now})
	run(1)
	// Taken hold of 2 into the gap, the divider's gap now starts at 98.
	if a := s.firstLength(); a != 98 {
		t.Fatalf("the first pane is %v wide, want 98", a)
	}
	// It stops short of squeezing a pane below its least.
	w.Input(input.PointerMove{Pos: geom.Pt(5, 100), Time: now})
	run(1)
	if a := s.firstLength(); a != splitMin {
		t.Fatalf("dragged to the edge, the first pane is %v wide, want %v", a, splitMin)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(5, 100), Time: now})
	run(1)
	got := sent(w)
	if len(got) != 1 {
		t.Fatalf("intents %v, want one move", got)
	}
	if m, ok := got[0].(splitMoved); !ok || m.Share != float32(splitMin)/400 {
		t.Fatalf("intent %v, want the share %v", got[0], float32(splitMin)/400)
	}
}

func TestADoubleClickOnTheDividerEvensThePanes(t *testing.T) {
	w, s, run := newSplitStage(t)
	s.SetShare(0.2, nil)
	run(1)
	at := s.firstLength() + 3
	w.Input(input.PointerDown{Pos: geom.Pt(at, 100), Clicks: 2, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(at, 100), Time: time.Now()})
	run(60)
	if a := s.firstLength(); a != 200 {
		t.Fatalf("after a double click the first pane is %v wide, want 200", a)
	}
}

func TestANewPaneSlidesInAsTheDividerSprings(t *testing.T) {
	_, s, run := newSplitStage(t)
	s.SetShare(1, nil)
	run(1)
	if a, g := s.firstLength(), s.gap; a != 406 || g != 0 {
		t.Fatalf("at a share of 1 the first pane is %v wide with a gap of %v, want 406 and 0", a, g)
	}
	s.SetShare(0.5, Settle.Default())
	last := s.firstLength()
	for i := range 90 {
		run(1)
		a := s.firstLength()
		if a > last {
			t.Fatalf("frame %d: the first pane grew back, from %v to %v", i, last, a)
		}
		last = a
	}
	if last != 200 || s.gap != 6 {
		t.Fatalf("the split settled at %v with a gap of %v, want 200 and 6", last, s.gap)
	}
}

func TestThePointerTakesTheShapeOfWhatItIsOver(t *testing.T) {
	w, _, run := newSplitStage(t)
	off := w.Offscreen()
	for _, c := range []struct {
		x    float32
		want input.Cursor
	}{
		{202, input.CursorResizeH},
		{300, input.CursorArrow},
		{100, input.CursorText},
	} {
		w.Input(input.PointerMove{Pos: geom.Pt(c.x, 18), Time: time.Now()})
		run(1)
		if got := off.Cursor(); got != c.want {
			t.Fatalf("at x %v the pointer is %v, want %v", c.x, got, c.want)
		}
	}
	// Held by the divider, the arrows stay wherever the pointer goes.
	w.Input(input.PointerMove{Pos: geom.Pt(202, 18), Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(202, 18), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 18), Time: time.Now()})
	run(1)
	if got := off.Cursor(); got != input.CursorResizeH {
		t.Fatalf("dragging the divider, the pointer is %v, want the resize arrows", got)
	}
}

func TestAFixedSplitKeepsItsFirstPaneAsTheWindowResizes(t *testing.T) {
	w, s, run := newSplitStage(t)
	s.Fixed = true
	s.SetShare(120, nil)
	run(1)
	if a := s.firstLength(); a != 120 {
		t.Fatalf("the first pane is %v wide, want 120", a)
	}
	w.Offscreen().Resize(geom.Sz(700, 300))
	run(1)
	if a := s.firstLength(); a != 120 {
		t.Fatalf("after the window widened the first pane is %v wide, want 120", a)
	}
	// Folded away, it takes the divider with it.
	s.SetShare(0, nil)
	run(1)
	if a, g := s.firstLength(), s.gap; a != 0 || g != 0 {
		t.Fatalf("folded, the first pane is %v wide with a gap of %v, want 0 and 0", a, g)
	}
}
