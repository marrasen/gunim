package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// rowsDragged is what a drag of a grid's rows carries in these tests.
type rowsDragged struct{ sel [][2]int }

// dragGrid is multiGrid with its rows draggable, and a drop target
// right of it that takes them.
func dragGrid(t *testing.T) (g *DataGrid, w *gunim.Window, run func(int), rowY func(int) float32, drops *[]any) {
	t.Helper()
	g = NewDataGrid(GridColumn{Title: "Name"})
	g.Multi = true
	g.Row = func(int) (GridRow, bool) { return GridRow{Cells: [][]GridSpan{{{Text: "row"}}}}, true }
	g.OnSelectRows = func(sel [][2]int, cursor int, u *gunim.UI) gunim.Intent { return gridRows{sel, cursor} }
	g.DragRows = func(sel [][2]int, _ geom.Point) (any, gunim.Node, geom.Point) {
		return rowsDragged{sel}, NewDragGhost(&block{h: 20}, geom.Pt(4, 4)), geom.Pt(4, 4)
	}
	g.rows = 100
	var got []any
	target := NewDropTarget(&block{h: 300})
	target.OnDrop = func(e input.Drop, u *gunim.UI) gunim.Intent { got = append(got, e.Data); return nil }
	w, run = stage(t, &halves{left: g, right: target})
	rowY = func(i int) float32 { return GridHeaderHeight.Default() + GridRowHeight.Default()*float32(i) + 5 }
	return g, w, run, rowY, &got
}

func TestDraggingASelectedRowDragsTheWholeSelection(t *testing.T) {
	g, w, run, rowY, drops := dragGrid(t)
	g.runs, g.selected, g.anchor = [][2]int{{2, 3}, {5, 7}}, 5, 5
	w.Input(input.PointerDown{Pos: geom.Pt(50, rowY(5)), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(60, rowY(5)), Time: time.Now()})
	run(10)
	if !g.lift.dragging {
		t.Fatal("a press on a selected row and a move did not start a drag")
	}
	if got := g.SelectedRows(); !slices.Equal(got, [][2]int{{2, 3}, {5, 7}}) {
		t.Fatalf("starting the drag left rows %v selected, want all three", got)
	}
	if g.away.Value() < 0.5 {
		t.Fatal("the rows dragged did not dim")
	}
	w.Input(input.PointerMove{Pos: geom.Pt(600, 100), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(600, 100), Button: input.ButtonPrimary, Time: time.Now()})
	run(60)
	if len(*drops) != 1 || !slices.Equal(dragged((*drops)[0]), [][2]int{{2, 3}, {5, 7}}) {
		t.Fatalf("the target got %v, want the three rows", *drops)
	}
	if g.away.Value() > 0.01 || g.lift.armed {
		t.Fatal("the rows stayed dim, or the grid still holds the press, after the drop")
	}
}

func TestAClickOnASelectedRowSelectsItAloneOnRelease(t *testing.T) {
	g, w, run, rowY, _ := dragGrid(t)
	g.runs, g.selected, g.anchor = [][2]int{{2, 6}}, 2, 2
	w.Input(input.PointerDown{Pos: geom.Pt(50, rowY(4)), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	run(1)
	if got := g.SelectedRows(); !slices.Equal(got, [][2]int{{2, 6}}) {
		t.Fatalf("the press alone changed the selection to %v", got)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(50, rowY(4)), Button: input.ButtonPrimary, Time: time.Now()})
	run(1)
	if got := lastRows(t, w); !slices.Equal(got, [][2]int{{4, 5}}) {
		t.Fatalf("after the release rows %v are selected, want row 4 alone", got)
	}
}

func TestDraggingARowNotSelectedSelectsAndDragsIt(t *testing.T) {
	g, w, run, rowY, _ := dragGrid(t)
	g.runs, g.selected, g.anchor = [][2]int{{2, 3}}, 2, 2
	w.Input(input.PointerDown{Pos: geom.Pt(50, rowY(8)), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(50, rowY(8)+10), Time: time.Now()})
	run(1)
	if !g.lift.dragging || !slices.Equal(g.SelectedRows(), [][2]int{{8, 9}}) {
		t.Fatalf("dragging row 8 left rows %v selected, dragging %v", g.SelectedRows(), g.lift.dragging)
	}
}

func TestRowAtAndRowRectAgree(t *testing.T) {
	g, _, _, rowY, _ := dragGrid(t)
	r, ok := g.RowRect(3)
	if !ok || !r.Contains(geom.Pt(50, rowY(3))) {
		t.Fatalf("row 3 shows at %v, %v, which misses its middle", r, ok)
	}
	if got := g.RowAt(geom.Pt(50, rowY(3))); got != 3 {
		t.Fatalf("RowAt found row %d, want 3", got)
	}
	if got := g.RowAt(geom.Pt(50, 5)); got != -1 {
		t.Fatalf("RowAt over the titles found row %d", got)
	}
}

// halves puts left in the left 400 by 300 and right beside it.
type halves struct{ left, right gunim.Node }

func (h *halves) Children() []gunim.Node { return []gunim.Node{h.left, h.right} }

func (h *halves) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for i := range kids.Len() {
		k := kids.At(i)
		k.Layout(gunim.Tight(geom.Sz(400, 300)))
		k.Place(geom.Pt(float32(i)*400, 0))
	}
	return c.Max
}

func (h *halves) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// dragged returns the rows a drag carried, or nil for anything else.
func dragged(v any) [][2]int {
	if d, ok := v.(rowsDragged); ok {
		return d.sel
	}
	return nil
}

func TestCtrlShiftCIsLeftToTheKeysAroundTheGrid(t *testing.T) {
	g, w, run, rowY, _ := dragGrid(t)
	g.OnCopy = func([][2]int, *gunim.UI) gunim.Intent { return rowsDragged{} }
	w.Input(input.PointerDown{Pos: geom.Pt(50, rowY(1)), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(50, rowY(1)), Button: input.ButtonPrimary, Time: time.Now()})
	run(1)
	sent(w)
	w.Input(input.KeyPress{Key: input.KeyC, Mods: input.ModControl | input.ModShift, Time: time.Now()})
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("Ctrl+Shift+C sent %v", got)
	}
	w.Input(input.KeyPress{Key: input.KeyC, Mods: input.ModControl, Time: time.Now()})
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("Ctrl+C sent %v, want the copy", got)
	}
}
