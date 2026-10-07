package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// newOverviewStage mounts a grid of n rows with an overview beside it,
// 400 and 20 wide, 300 tall.
func newOverviewStage(t *testing.T, n int) (*gunim.Window, *DataGrid, *Overview, func(int)) {
	t.Helper()
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(int) (GridRow, bool) { return GridRow{}, true }
	g.NoBar = true
	g.rows = n
	o := NewOverview(g)
	o.Bands = make([]OverviewBand, 100)
	row := Row(g, o).Grow(g, 1)
	row.Gap = tableNoGap
	row.Cross = CrossStretch
	w, run := stage(t, &frame{child: row, size: geom.Sz(420, 300)})
	return w, g, o, run
}

func TestAPressOnTheOverviewMovesTheViewThere(t *testing.T) {
	w, g, _, run := newOverviewStage(t, 10_000)
	click(w, 410, 150)
	run(1)
	// The box centres on the press: halfway down is halfway through.
	want := (10_000 - g.Visible()) * 0.5
	if d := g.Top() - want; d < -1 || d > 1 {
		t.Fatalf("after a press halfway down, the top is row %v, want about %v", g.Top(), want)
	}
}

func TestDraggingTheBoxKeepsItsGrip(t *testing.T) {
	w, g, o, run := newOverviewStage(t, 10_000)
	top, size, travel := o.box(nil)
	if top != 0 || size != OverviewMinBox.Default() {
		t.Fatalf("at the start the box is at %v, %v tall, want 0 and the least height", top, size)
	}
	now := time.Now()
	w.Input(input.PointerDown{Pos: geom.Pt(410, 2), Button: input.ButtonPrimary, Clicks: 1, Time: now})
	run(1)
	if g.Top() != 0 {
		t.Fatalf("a press inside the box moved the view to row %v", g.Top())
	}
	w.Input(input.PointerMove{Pos: geom.Pt(410, 2+travel/4), Time: now})
	w.Input(input.PointerUp{Pos: geom.Pt(410, 2+travel/4), Button: input.ButtonPrimary, Time: now})
	run(1)
	want := (10_000 - g.Visible()) / 4
	if d := g.Top() - want; d < -1 || d > 1 {
		t.Fatalf("dragged a quarter of the way, the top is row %v, want about %v", g.Top(), want)
	}
}

func TestTheWheelOverTheOverviewScrollsTheGrid(t *testing.T) {
	w, g, _, run := newOverviewStage(t, 10_000)
	w.Input(input.Scroll{Pos: geom.Pt(410, 100), Delta: geom.Pt(0, -22*10)})
	run(120)
	if g.Top() != 10 {
		t.Fatalf("after the wheel moved ten rows' worth, the top is row %v, want 10", g.Top())
	}
}

func TestTheOverviewSaysWhatTheBandUnderThePointerHolds(t *testing.T) {
	w, _, o, run := newOverviewStage(t, 10_000)
	asked := -1
	o.Readout = func(b int) string {
		asked = b
		return "band"
	}
	w.Input(input.PointerMove{Pos: geom.Pt(410, 151), Time: time.Now()})
	run(1)
	if asked != 50 {
		t.Fatalf("the pointer halfway down asked about band %d, want 50", asked)
	}
}

func TestTheOverviewReadoutStaysInTheWindowAtItsLeftEdge(t *testing.T) {
	o := NewOverview(nil)
	o.Bands = make([]OverviewBand, 10)
	o.Readout = func(int) string { return "a readout of some length" }
	o.hover = 5
	box := geom.Sz(20, 300)
	for _, left := range []float32{0, 400} {
		var p paint.Painter
		func() {
			defer p.Push(paint.Translate(geom.Pt(left, 0)))()
			o.Paint(&p, gunim.Frame{Scale: 1}, box, gunim.Children{})
		}()
		p.PaintFloats()
		var card geom.Rect
		for _, op := range p.Ops() {
			if r, ok := op.(*paint.RRectOp); ok && r.Shadow.Blur > 0 {
				card = geom.Rect{Min: r.Transform.Apply(r.Rect.Min), Max: r.Transform.Apply(r.Rect.Max)}
			}
		}
		if card.Empty() || card.Min.X < 0 {
			t.Fatalf("with the strip at x %v, the readout is at %v; want it inside the window", left, card)
		}
		if beside := card.Max.X <= left || card.Min.X >= left+box.W; !beside {
			t.Fatalf("with the strip at x %v, the readout at %v covers the strip", left, card)
		}
	}
}

func TestAnOverviewLeavesRoomAboveItForAHeader(t *testing.T) {
	w, g, o, run := newOverviewStage(t, 1000)
	o.Top = GridHeaderHeight
	run(1)
	click(w, 410, 300-1)
	run(1)
	if want := 1000 - g.Visible(); g.Top() != want {
		t.Fatalf("after a press at the bottom, the top is row %v, want %v", g.Top(), want)
	}
	click(w, 410, GridHeaderHeight.Default()+1)
	run(1)
	if g.Top() != 0 {
		t.Fatalf("after a press at the top of the strip, below the header, the top is row %v, want 0", g.Top())
	}
}
