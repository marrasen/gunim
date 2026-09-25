package widget

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// cellsOf turns s into a row of plain cells.
func cellsOf(s string) []Cell {
	out := make([]Cell, 0, len(s))
	for _, r := range s {
		out = append(out, Cell{Rune: r})
	}
	return out
}

// newCellStage mounts a grid of 40 by 12 cells in a 400 by 300 space,
// with a line of text in every row.
func newCellStage(t testing.TB) (*gunim.Window, *CellGrid, func(int)) {
	g := NewCellGrid()
	g.Size = 14
	g.Resize(40, 12)
	for y := range 12 {
		g.SetRow(y, cellsOf(fmt.Sprintf("row %d: the quick brown fox", y)))
	}
	w := gunim.NewOffscreen(geom.Sz(400, 300), nil)
	gunim.RegisterView(w, "cells", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "cells", "cells", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(16_666_667)
		}
	}
	run(2)
	return w, g, run
}

func TestAGridFitsWholeCellsToItsSpace(t *testing.T) {
	_, g, _ := newCellStage(t)
	cell := g.CellSize()
	if cell.W <= 0 || cell.H <= 0 {
		t.Fatalf("cells are %v", cell)
	}
	if cell.W != float32(math.Round(float64(cell.W))) || cell.H != float32(math.Round(float64(cell.H))) {
		t.Fatalf("cells are %v, want whole pixels at scale 1", cell)
	}
	cols, rows := g.Fit()
	if cols != int(400/cell.W) || rows != int(300/cell.H) {
		t.Fatalf("fit %d by %d cells of %v in 400 by 300", cols, rows, cell)
	}
}

func TestAGridRedrawsOnlyTheRowThatChanged(t *testing.T) {
	w, g, run := newCellStage(t)
	off := w.Offscreen()
	if d := off.Damage(); !d.Empty() {
		t.Fatalf("a frame with nothing new damaged %v", d)
	}
	// The same cells again change nothing.
	g.SetRow(5, cellsOf("row 5: the quick brown fox"))
	run(1)
	if d := off.Damage(); !d.Empty() {
		t.Fatalf("setting a row to what it held damaged %v", d)
	}
	g.SetRow(5, cellsOf("row 5: jumps over the lazy dog"))
	run(1)
	h := g.CellSize().H
	d := off.Damage()
	if d.Empty() || d.Min.Y < 5*h-2 || d.Max.Y > 6*h+2 {
		t.Fatalf("changing row 5, %v to %v down, damaged %v", 5*h, 6*h, d)
	}
}

func TestTheCursorGlidesAlongItsRowAndJumpsToAnother(t *testing.T) {
	_, g, run := newCellStage(t)
	g.SetCursor(Cursor{Col: 0, Row: 2, Visible: true})
	run(30)
	cell := g.CellSize()
	g.SetCursor(Cursor{Col: 6, Row: 2, Visible: true})
	last := g.at.Value().X
	for i := range 30 {
		run(1)
		x := g.at.Value().X
		if x < last {
			t.Fatalf("frame %d: the cursor moved back, from %v to %v", i, last, x)
		}
		last = x
	}
	if last != 6*cell.W {
		t.Fatalf("the cursor settled at %v, want %v", last, 6*cell.W)
	}
	g.SetCursor(Cursor{Col: 1, Row: 7, Visible: true})
	run(1)
	if at := g.at.Value(); at != geom.Pt(cell.W, 7*cell.H) {
		t.Fatalf("a frame after moving rows the cursor is at %v, want %v", at, geom.Pt(cell.W, 7*cell.H))
	}
}

func TestABlockCursorShowsItsCharacterInTheCellsBackground(t *testing.T) {
	w, g, run := newCellStage(t)
	bg := color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}
	row := cellsOf("abc")
	row[1].BG = bg
	g.SetRow(0, row)
	g.SetCursor(Cursor{Col: 1, Row: 0, Visible: true, Color: color.NRGBA{R: 0xff, A: 0xff}})
	run(30)
	ops := w.Offscreen().Ops()
	last, ok := ops[len(ops)-1].(*paint.TextOp)
	if !ok || last.Color != bg || len(last.Glyphs) != 1 {
		t.Fatalf("the last op is %+v, want b in the cell's background", ops[len(ops)-1])
	}
}

func TestAWideCharacterCoversTwoCells(t *testing.T) {
	w, g, run := newCellStage(t)
	red := color.NRGBA{R: 0xff, A: 0xff}
	g.SetRow(0, []Cell{{Rune: '中', Wide: true, BG: red}, {}, {Rune: 'x'}})
	run(1)
	cell := g.CellSize()
	for _, op := range w.Offscreen().Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Fill.Solid == red {
			if r.Rect.Size().W != 2*cell.W {
				t.Fatalf("the wide character's background is %v wide, want %v", r.Rect.Size().W, 2*cell.W)
			}
			return
		}
	}
	t.Fatal("no background drawn for the wide character")
}

// BenchmarkAGridScrollingAFullScreen draws a 200 by 60 terminal whose
// every row changes each frame, as it does while text scrolls past.
func BenchmarkAGridScrollingAFullScreen(b *testing.B) {
	g := NewCellGrid()
	g.Size = 14
	g.Resize(200, 60)
	w := gunim.NewOffscreen(geom.Sz(1700, 1100), nil)
	gunim.RegisterView(w, "cells", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "cells", "cells", nil); err != nil {
		b.Fatal(err)
	}
	lines := make([][]Cell, 61)
	for i := range lines {
		row := cellsOf(fmt.Sprintf("%4d %-190s", i, "func (g *CellGrid) drawRow(cells []Cell, ink color.NRGBA) rowPaint { m := g.metrics"))
		for x := range row {
			row[x].FG = color.NRGBA{R: uint8(40 * (x / 20)), G: 0xc0, B: 0x80, A: 0xff}
		}
		lines[i] = row
	}
	w.Frame(16_666_667)
	b.ResetTimer()
	for i := range b.N {
		for y := range 60 {
			g.SetRow(y, lines[(y+i)%61])
		}
		w.Frame(16_666_667)
	}
}
