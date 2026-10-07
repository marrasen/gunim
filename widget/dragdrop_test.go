package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type dropped struct {
	Data  any
	Paths []string
}

type clicked struct{}

// dropStage holds a draggable block on the left and a drop target on
// the right, side by side in one window.
func dropStage(t *testing.T) (*gunim.Window, *Draggable, *DropTarget, func(int)) {
	t.Helper()
	d := NewDraggable(&block{h: 50}, "apple")
	d.OnClick = Sends(clicked{})
	target := NewDropTarget(&block{h: 200})
	target.Accept = func(data any, paths []string) bool {
		return data == "apple" || slices.Contains(paths, "/tmp/sun.png")
	}
	target.OnDrop = func(e input.Drop, u *gunim.UI) gunim.Intent { return dropped{e.Data, e.Paths} }
	row := Row(d, target).Grow(d, 1).Grow(target, 1)
	w, run := stage(t, &frame{child: row, size: geom.Sz(400, 200)})
	return w, d, target, run
}

func TestDraggingOntoATargetDropsItsData(t *testing.T) {
	w, d, target, run := dropStage(t)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 50), Time: time.Now()})
	run(20)
	if target.glow.Value() < 0.5 || d.away.Value() < 0.5 {
		t.Fatalf("over the target: glow %v, away %v; want both lit", target.glow.Value(), d.away.Value())
	}
	w.Input(input.PointerUp{Pos: geom.Pt(300, 50), Time: time.Now()})
	run(60)
	got := sent(w)
	if len(got) != 1 {
		t.Fatalf("intents %v, want one drop of apple", got)
	}
	if e, ok := got[0].(dropped); !ok || e.Data != "apple" {
		t.Fatalf("intent %v, want a drop of apple", got[0])
	}
	if d.away.Value() > 0.01 || target.glow.Value() > 0.01 {
		t.Fatal("the draggable stayed dim or the target stayed lit after the drop")
	}
}

func TestAClickOnADraggableIsAClick(t *testing.T) {
	w, _, _, run := dropStage(t)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(5, 10)})
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (clicked{}) {
		t.Fatalf("intents %v, want a click", got)
	}
}

func TestATargetTakesFilesItAccepts(t *testing.T) {
	w, _, _, run := dropStage(t)
	w.Input(input.Drop{Pos: geom.Pt(300, 50), Paths: []string{"/tmp/notes.txt"}})
	w.Input(input.Drop{Pos: geom.Pt(300, 50), Paths: []string{"/tmp/sun.png"}})
	run(1)
	got := sent(w)
	if len(got) != 1 {
		t.Fatalf("intents %v, want only the picture's drop", got)
	}
	if d, ok := got[0].(dropped); !ok || !slices.Equal(d.Paths, []string{"/tmp/sun.png"}) {
		t.Fatalf("intent %v, want the picture's drop", got[0])
	}
}
