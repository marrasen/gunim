package widget

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The row under a point is found, and a row is where RowRect says.
func TestATableFindsTheRowUnderAPoint(t *testing.T) {
	_, tbl, _, _ := newTableStage(t, 100)
	r, ok := tbl.RowRect("3")
	if !ok || r.Min.Y != tbl.Header()+3*tbl.rowH {
		t.Fatalf("row 3 is at %v", r)
	}
	if k, ok := tbl.RowAt(r.Center()); !ok || k != "3" {
		t.Fatalf("the row at row 3's middle is %q", k)
	}
	if _, ok := tbl.RowAt(geom.Pt(10, 2)); ok {
		t.Fatal("a row was found in the header")
	}
}

// A press on a row and a move drags that row, or the rows marked when
// it is one of them; a click without moving is still a click.
func TestATableDragsItsRows(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 100)
	var dragged [][]Key
	tbl.DragRows = func(keys []Key, at geom.Point) (any, gunim.Node, geom.Point) {
		dragged = append(dragged, slices.Clone(keys))
		return "rows", NewLabel("rows"), geom.Point{}
	}
	ended := 0
	tbl.DragEnded = func(input.DragEnd, *gunim.UI) { ended++ }
	r, _ := tbl.RowRect("2")
	at := r.Center()
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(1)
	if len(dragged) != 0 {
		t.Fatal("a click dragged")
	}
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerMove{Pos: at.Add(geom.Pt(0, 20))})
	run(1)
	if len(dragged) != 1 || !slices.Equal(dragged[0], []Key{"2"}) || !tbl.lift.dragging {
		t.Fatalf("dragged %v", dragged)
	}
	w.Input(input.PointerUp{Pos: at.Add(geom.Pt(0, 20)), Button: input.ButtonPrimary})
	run(2)
	if ended != 1 || tbl.lift.dragging {
		t.Fatalf("the drag ended %d times, still dragging %v", ended, tbl.lift.dragging)
	}
	tbl.marked["5"], tbl.marked["6"] = true, true
	r, _ = tbl.RowRect("6")
	w.Input(input.PointerDown{Pos: r.Center(), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerMove{Pos: r.Center().Add(geom.Pt(30, 0))})
	run(1)
	if len(dragged) != 2 || !slices.Equal(dragged[1], []Key{"5", "6"}) {
		t.Fatalf("dragging a marked row dragged %v", dragged)
	}
}

// Marks set from outside mark those rows alone, and only rows the
// table holds.
func TestATableTakesMarksFromOutside(t *testing.T) {
	_, tbl, _, _ := newTableStage(t, 10)
	tbl.marked["1"] = true
	tbl.SetMarked([]Key{"3", "4", "nope"})
	if got := tbl.Marked(); !slices.Equal(got, []Key{"3", "4"}) {
		t.Fatalf("the rows marked are %v", got)
	}
}
