package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

type gridView struct{ First, Count int }

type gridSelected struct{ Row int }

type gridResized struct {
	Column int
	Width  float32
}

func TestADataGridAsksOnlyForTheRowsInView(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	asked := map[int]int{}
	g.Row = func(i int) (GridRow, bool) {
		asked[i]++
		return GridRow{Cells: [][]GridSpan{{{Text: "row"}}}}, true
	}
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	g.rows = 1_000_000
	run(2)
	for i := range asked {
		if i > 13 {
			t.Fatalf("row %d was asked for, with only rows 0 to 13 in view", i)
		}
	}
	if len(asked) < 13 {
		t.Fatalf("%d rows asked for, want the 13 in view", len(asked))
	}
	_ = w
}

func TestADataGridSaysWhichRowsAreInView(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{}, false }
	g.OnView = func(first, count int) gunim.Intent { return gridView{first, count} }
	g.rows = 10_000
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	got := sent(w)
	if len(got) != 1 || got[0] != (gridView{0, 14}) {
		t.Fatalf("intents %v, want rows 0 to 14 in view", got)
	}
	w.Input(input.Scroll{Pos: geom.Pt(100, 100), Delta: geom.Pt(0, -22*100)})
	run(120)
	got = sent(w)
	if len(got) == 0 || got[len(got)-1] != (gridView{100, 14}) {
		t.Fatalf("after scrolling 100 rows, intents %v, want the last to be rows 100 to 114", got)
	}
}

func TestADataGridKeepsItsPlaceFarDown(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{}, true }
	g.rows = 2_000_000
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	_ = w
	// Far enough down that a float32 offset in pixels moves in steps of 4.
	g.goal, g.top = 1_999_000.25, 1_999_000.25
	run(1)
	w.Input(input.Scroll{Pos: geom.Pt(100, 100), Delta: geom.Pt(0, -11)})
	run(120)
	if want := 1_999_000.75; g.Top() != want {
		t.Fatalf("half a row down from 1999000.25, the top is %v, want %v", g.Top(), want)
	}
}

func TestArrowKeysMoveTheSelectionAndSaySo(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{}, true }
	g.OnSelect = func(row int) gunim.Intent { return gridSelected{row} }
	g.rows = 100
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	click(w, 100, 26+22*3+5)
	run(1)
	if i, ok := g.Selected(); !ok || i != 3 {
		t.Fatalf("a click on the fourth row selected %v %v, want row 3", i, ok)
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyEnd})
	run(60)
	if i, _ := g.Selected(); i != 99 {
		t.Fatalf("after End, row %v is selected, want 99", i)
	}
	if top := g.Top(); top+g.Visible() < 100 {
		t.Fatalf("after End, the view starts at row %v and does not reach row 99", top)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	got := sent(w)
	want := []gunim.Intent{gridSelected{3}, gridSelected{4}, gridSelected{99}, gridSelected{-1}}
	if len(got) != len(want) {
		t.Fatalf("intents %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("intents %v, want %v", got, want)
		}
	}
}

func TestDraggingATitlesEdgeResizesItsColumn(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Time", Width: 100}, GridColumn{Title: "Message"})
	g.OnResize = func(c int, w float32) gunim.Intent { return gridResized{c, w} }
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	now := time.Now()
	w.Input(input.PointerDown{Pos: geom.Pt(99, 10), Button: input.ButtonPrimary, Clicks: 1, Time: now})
	w.Input(input.PointerMove{Pos: geom.Pt(159, 10), Time: now})
	w.Input(input.PointerUp{Pos: geom.Pt(159, 10), Button: input.ButtonPrimary, Time: now})
	run(1)
	if g.Columns[0].Width != 160 {
		t.Fatalf("the column is %v wide, want 160", g.Columns[0].Width)
	}
	if x := g.xs[1][0]; x != 160 {
		t.Fatalf("the next column starts at %v, want 160", x)
	}
	if got := sent(w); len(got) != 1 || got[0] != (gridResized{0, 160}) {
		t.Fatalf("intents %v, want the column resized to 160", got)
	}
}

func TestDraggingTheScrollbarScrolls(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{}, true }
	g.rows = 1000
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	_, thumb, ok := g.barRect(nil)
	if !ok {
		t.Fatal("no scrollbar on a grid of 1000 rows")
	}
	now := time.Now()
	from := thumb.Center()
	w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1, Time: now})
	w.Input(input.PointerMove{Pos: geom.Pt(from.X, 400), Time: now})
	run(1)
	if want := 1000 - g.Visible(); g.Top() != want {
		t.Fatalf("with the thumb dragged to the bottom, the top is %v, want %v", g.Top(), want)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(from.X, 400), Button: input.ButtonPrimary, Time: now})
}

func TestALongCellEndsInAnEllipsis(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message", Width: 80})
	long := "a message far too long to fit in eighty pixels"
	run := g.cutRun(Font.Default().Shape(long, 12), 60)
	if run.Advance > 60 {
		t.Fatalf("cut to 60, the run is %v wide", run.Advance)
	}
	last := run.Glyphs[len(run.Glyphs)-1]
	ell := Font.Default().Shape("…", 12).Glyphs[0]
	if last.ID != ell.ID {
		t.Fatalf("the cut run ends in glyph %d, want the ellipsis %d", last.ID, ell.ID)
	}
}

func TestPendingRowsKeepTheGridDrawing(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{}, false }
	g.rows = 10
	_, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	run(1)
	if !g.Step(time.Second / 60) {
		t.Fatal("a grid showing rows yet to arrive stops animating their placeholders")
	}
}

func TestAMarkLightsTheRunesItCovers(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(int) (GridRow, bool) {
		return GridRow{Cells: [][]GridSpan{{{Text: "find the needle here", Marks: [][2]int{{9, 15}}}}}}, true
	}
	g.rows = 1
	w, _ := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	run := Font.Default().Shape("find the needle here", GridTextSize.Default())
	want0, want1 := run.CaretX(9), run.CaretX(15)
	found := false
	for _, op := range w.Offscreen().Ops() {
		r, ok := op.(*paint.RRectOp)
		if !ok || r.Fill.Solid != GridMark.Default() {
			continue
		}
		found = true
		x := r.Transform.Apply(r.Rect.Min).X
		pad := GridCellPadding.Default()
		if d := x - (pad + want0); d < -0.5 || d > 0.5 || r.Rect.Size().W-(want1-want0) > 0.5 {
			t.Fatalf("the mark is at %v, %v wide; want %v, %v wide", x, r.Rect.Size().W, pad+want0, want1-want0)
		}
	}
	if !found {
		t.Fatal("no mark was drawn")
	}
}

type gridClicked struct{ Row int }

func TestEveryClickOnARowIsSent(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Name"})
	g.Row = func(int) (GridRow, bool) { return GridRow{}, true }
	g.OnClick = func(row int) gunim.Intent { return gridClicked{row} }
	g.rows = 10
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	y := GridHeaderHeight.Default() + GridRowHeight.Default()*2 + 5
	click(w, 50, y)
	run(1)
	click(w, 50, y)
	run(1)
	if got := sent(w); len(got) != 2 || got[0] != (gridClicked{2}) || got[1] != (gridClicked{2}) {
		t.Fatalf("two clicks on row 2 sent %v", got)
	}
}

type gridSorted struct{ Column int }

func TestAClickOnATitleIsSent(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Name", Width: 100, Sort: 1}, GridColumn{Title: "Size", Closable: true})
	g.OnHeader = func(c int) gunim.Intent { return gridSorted{c} }
	g.OnClose = func(c int) gunim.Intent { return gridResized{c, 0} }
	w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	click(w, 40, 10)
	run(1)
	click(w, 150, 10)
	run(1)
	if got := sent(w); len(got) != 2 || got[0] != (gridSorted{0}) || got[1] != (gridSorted{1}) {
		t.Fatalf("clicks on the titles sent %v", got)
	}
}
