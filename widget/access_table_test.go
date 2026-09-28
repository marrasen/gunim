package widget

import (
	"strconv"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type gridHeaderPressed struct{ Column int }

func TestADataGridReadsAsATable(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Name"}, GridColumn{Title: "Size", Width: 80})
	g.Row = func(i int) (GridRow, bool) {
		return GridRow{Cells: [][]GridSpan{{{Text: "file" + strconv.Itoa(i)}}, {{Text: strconv.Itoa(i) + " KB"}}}}, true
	}
	g.OnSelect = func(row int) gunim.Intent { return gridSelected{row} }
	g.OnHeader = func(c int) gunim.Intent { return gridHeaderPressed{c} }
	g.rows = 50
	g.selected = 2
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	w.Offscreen().ListenForAccess()
	run(2)
	tree := w.Offscreen().AccessTree()
	table := find(tree.Root, access.RoleTable, "")
	if table == nil {
		t.Fatal("no table in the tree")
	}
	var b strings.Builder
	dump(table, 0, &b)
	got := b.String()
	for _, want := range []string{"row Name, Size", "column header Name", "column header Size", "row file0, 0 KB", "cell file0", "cell 0 KB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the table lacks %q:\n%s", want, got)
		}
	}
	row2 := find(table, access.RoleRow, "file2, 2 KB")
	if row2 == nil || !row2.State.Has(access.StateSelected) || row2.Description != "row 3 of 50" {
		t.Fatalf("the selected row says %+v", row2)
	}
	// Pressing a cell of another row selects that row; pressing a title
	// sorts by it.
	cell := find(table, access.RoleCell, "file5")
	w.Input(access.Request{ID: cell.ID, Action: access.ActionPress})
	size := find(table, access.RoleColumnHeader, "Size")
	w.Input(access.Request{ID: size.ID, Action: access.ActionPress})
	run(2)
	if sel, _ := g.Selected(); sel != 5 {
		t.Fatalf("pressing a cell of row 5 selected row %d", sel)
	}
	if got := sent(w); len(got) != 2 || got[0] != (gridSelected{5}) || got[1] != (gridHeaderPressed{1}) {
		t.Fatalf("the presses sent %v", got)
	}
}

func TestAMenubarAndAProgressBarSayWhatTheyAre(t *testing.T) {
	bar := NewMenubar(BarMenu{Title: "File", Items: []string{"Open"}}, BarMenu{Title: "Edit", Items: []string{"Copy"}})
	p := NewProgressBar()
	w, run := stage(t, &frame{child: Column(bar, p), size: geom.Sz(400, 300)})
	w.Offscreen().ListenForAccess()
	p.value.Jump(0.25)
	run(2)
	tree := w.Offscreen().AccessTree()
	file := find(tree.Root, access.RoleMenuItem, "File")
	if find(tree.Root, access.RoleMenuBar, "") == nil || file == nil || file.Bounds.Empty() {
		t.Fatal("the menu bar and its titles are not in the tree")
	}
	w.Input(access.Request{ID: file.ID, Action: access.ActionPress})
	run(10)
	if !bar.IsOpen() {
		t.Fatal("pressing File left its menu shut")
	}
	progress := find(tree.Root, access.RoleProgressBar, "")
	if progress == nil || progress.Range == nil || progress.Range.Value != 0.25 {
		t.Fatalf("the progress bar says %+v", progress)
	}
}

// A screen reader holding a row keeps that row as the grid scrolls: a
// press on it after a scroll selects the row it read, and does nothing
// once that row has scrolled out of view.
func TestARowAScreenReaderHoldsStaysThatRowAsTheGridScrolls(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Name"})
	g.Row = func(i int) (GridRow, bool) {
		return GridRow{Cells: [][]GridSpan{{{Text: "file" + strconv.Itoa(i)}}}}, true
	}
	g.OnSelect = func(row int) gunim.Intent { return gridSelected{row} }
	g.rows = 500
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	w.Offscreen().ListenForAccess()
	run(2)
	row5 := find(w.Offscreen().AccessTree().Root, access.RoleRow, "file5")
	if row5 == nil {
		t.Fatal("row 5 is not in the tree")
	}
	scroll := func(top float64) {
		g.top, g.goal, g.vel = top, top, 0
		w.Input(input.PointerMove{Pos: geom.Pt(10, 10)})
		run(2)
	}
	scroll(3)
	if find(w.Offscreen().AccessTree().Root, access.RoleRow, "file5") == nil {
		t.Fatal("row 5 left the tree after a scroll of three rows")
	}
	w.Input(access.Request{ID: row5.ID, Action: access.ActionPress})
	run(1)
	if sel, _ := g.Selected(); sel != 5 {
		t.Fatalf("pressing the row read as file5 after a scroll selected row %d", sel)
	}
	scroll(100)
	sent(w)
	w.Input(access.Request{ID: row5.ID, Action: access.ActionPress})
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("pressing a row scrolled out of view sent %v", got)
	}
}
