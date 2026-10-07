package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type spotOpened struct{ key any }

type spotDropped struct {
	key   any
	data  any
	paths []string
}

// zoneStage holds a draggable on the left and a zone on the right whose
// top half is a spot that opens and whose bottom half refuses drops.
func zoneStage(t *testing.T) (w *gunim.Window, z *DropZone, run func(int)) {
	t.Helper()
	d := NewDraggable(&block{h: 50}, "apple")
	z = NewDropZone(&block{h: 300})
	z.Spot = func(e input.Drop, _ *gunim.UI) (DropSpot, bool) {
		if e.Pos.Y < 100 {
			return DropSpot{Key: "top", Rect: geom.Rc(0, 0, 400, 100), Opens: true,
				Hint: DropHint{Text: "Move to top", Effect: DropMove}}, true
		}
		return DropSpot{Key: "bottom", Rect: geom.Rc(0, 100, 400, 200), Refused: true}, true
	}
	z.OnDrop = func(s DropSpot, e input.Drop, u *gunim.UI) gunim.Intent { return spotDropped{s.Key, e.Data, e.Paths} }
	z.OnOpen = func(s DropSpot, u *gunim.UI) gunim.Intent { return spotOpened{s.Key} }
	w, run = stage(t, &halves{left: d, right: z})
	return w, z, run
}

func TestADragRestingOnASpotOpensIt(t *testing.T) {
	w, z, run := zoneStage(t)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(500, 50), Time: time.Now()})
	run(20)
	if s, ok := z.Over(); !ok || s.Key != "top" {
		t.Fatalf("the zone is over %v, %v; want the top spot", s.Key, ok)
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a third of a second in, the zone sent %v", got)
	}
	run(62)
	if got := sent(w); !slices.Contains(got, gunim.Intent(spotOpened{"top"})) {
		t.Fatalf("after resting a second the zone sent %v, want the top spot opened", got)
	}
	run(62)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("resting on, the zone opened the spot again: %v", got)
	}
}

func TestARefusedSpotTakesNoDrop(t *testing.T) {
	w, _, run := zoneStage(t)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(500, 200), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(500, 200), Time: time.Now()})
	run(5)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a drop on the refused spot sent %v", got)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(500, 50), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(500, 50), Time: time.Now()})
	run(5)
	if got := sent(w); len(got) != 1 || !droppedOn(got[0], "top", "apple") {
		t.Fatalf("a drop on the top spot sent %v, want one drop of apple", got)
	}
}

func TestFilesFromAnotherProgramLandOnTheSpotUnderThem(t *testing.T) {
	w, _, run := zoneStage(t)
	w.Input(input.Drop{Pos: geom.Pt(500, 40), Paths: []string{"/tmp/a.txt"}, Time: time.Now()})
	run(1)
	got := sent(w)
	if len(got) != 1 {
		t.Fatalf("the files dropped sent %v, want one drop", got)
	}
	if d, ok := got[0].(spotDropped); !ok || d.key != "top" || !slices.Equal(d.paths, []string{"/tmp/a.txt"}) {
		t.Fatalf("the files dropped sent %v, want them on the top spot", got[0])
	}
}

// droppedOn reports whether v is a drop of data on the spot key.
func droppedOn(v gunim.Intent, key, data any) bool {
	d, ok := v.(spotDropped)
	return ok && d.key == key && d.data == data
}
