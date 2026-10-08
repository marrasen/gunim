package widget

import (
	"fmt"
	"testing"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/theme"
)

// A theme that makes the caret spring instant, as a theme editor's
// "Jumps" does, moves the cursor along its row at once: the frame after
// it moves shows it in its new cell, with nothing drawn in between.
func TestAnInstantCaretMakesTheCursorJump(t *testing.T) {
	g := NewCellGrid()
	g.Size = 14
	g.Resize(40, 4)
	for y := range 4 {
		g.SetRow(y, cellsOf(fmt.Sprintf("row %d: the quick brown fox", y)))
	}
	jumps := theme.Make("jumps", theme.Set(Caret, anim.Spring{Response: 0, Damping: 1}))
	_, run := stage(t, &frame{child: NewThemed(g, jumps), size: geom.Sz(400, 100)})
	g.SetCursor(Cursor{Col: 0, Row: 1, Visible: true})
	run(10)
	cell := g.CellSize()
	for _, col := range []int{6, 7, 20, 3} {
		g.SetCursor(Cursor{Col: col, Row: 1, Visible: true})
		run(1)
		if x := g.at.Value().X; x != float32(col)*cell.W {
			t.Fatalf("a frame after the cursor moved to column %d it is at x %v, want %v", col, x, float32(col)*cell.W)
		}
		if g.at.Active() {
			t.Fatalf("at column %d the cursor still moves", col)
		}
	}
}
