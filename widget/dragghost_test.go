package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// ghostStage is dropStage with the draggable carrying a ghost, and the
// target saying what a drop does.
func ghostStage(t *testing.T, hint DropHint) (w *gunim.Window, ghost func() *DragGhost, run func(int)) {
	t.Helper()
	w, d, target, run := dropStage(t)
	var g *DragGhost
	d.Ghost = func() gunim.Node {
		g = NewDragGhost(&block{h: 30}, geom.Pt(5, 10))
		g.Badge, g.Stack = "3", 2
		return g
	}
	target.Hint = func(input.DragOver) any { return hint }
	return w, func() *DragGhost { return g }, run
}

func TestTheGhostShowsWhatTheTargetSays(t *testing.T) {
	want := DropHint{Text: "Move to Documents", Effect: DropMove}
	w, ghost, run := ghostStage(t, want)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	run(5)
	if _, on := ghost().Hint(); on {
		t.Fatal("the ghost shows a hint over the draggable itself")
	}
	w.Input(input.PointerMove{Pos: geom.Pt(300, 50), Time: time.Now()})
	run(30)
	if h, on := ghost().Hint(); !on || h != want {
		t.Fatalf("over the target the ghost shows %v, %v; want %v", h, on, want)
	}
	if ghost().lag[0] != 0 {
		t.Fatalf("the ghost still trails the pointer by %v once it rests", ghost().lag[0])
	}
}

func TestTheGhostTrailsThePointerAndCatchesUp(t *testing.T) {
	w, ghost, run := ghostStage(t, DropHint{})
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	run(1)
	w.Input(input.PointerMove{Pos: geom.Pt(40, 10), Time: time.Now()})
	if lag := ghost().lag[0]; lag >= 0 {
		t.Fatalf("moving right, the ghost trails by %v, want it behind, to the left", lag)
	}
	run(90)
	if ghost().lag[0] != 0 || ghost().vel[0] != 0 {
		t.Fatalf("the ghost did not catch up: lag %v, speed %v", ghost().lag[0], ghost().vel[0])
	}
}

// refuser takes a drag over it, says a drop would do nothing, and
// takes no drop.
type refuser struct{ block }

func (r *refuser) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.DragOver); ok {
		u.AnswerDrag(DropHint{Text: "Cannot go here", Effect: DropRefused})
		return true
	}
	return false
}

func TestARefusedDropShakesTheGhost(t *testing.T) {
	d := NewDraggable(&block{h: 50}, "apple")
	var g *DragGhost
	d.Ghost = func() gunim.Node {
		g = NewDragGhost(&block{h: 30}, geom.Pt(5, 10))
		return g
	}
	r := &refuser{block{h: 200}}
	row := Row(d, r).Grow(d, 1).Grow(r, 1)
	w, run := stage(t, &frame{child: row, size: geom.Sz(400, 200)})
	w.Input(input.PointerDown{Pos: geom.Pt(5, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 10), Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 50), Time: time.Now()})
	run(10)
	w.Input(input.PointerUp{Pos: geom.Pt(300, 50), Time: time.Now()})
	run(1)
	if !g.refused() {
		t.Fatalf("let go where the drop is refused, the ghost ended %v, taken %v, hint %v", g.ended, g.taken, g.hint)
	}
	run(18)
	if g.out.Value() <= 0 || g.out.Value() >= 1 {
		t.Fatalf("a third of a second on the shake is at %v, want part way", g.out.Value())
	}
	run(40)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a refused drop sent %v", got)
	}
}
