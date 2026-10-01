package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A cell grid shows the I-beam, unless Pointer names another shape.
func TestACellGridsPointerCanBeNamed(t *testing.T) {
	g := NewCellGrid()
	if got := g.Cursor(geom.Pt(1, 1)); got != input.CursorText {
		t.Fatalf("a grid shows %v, want the I-beam", got)
	}
	g.Pointer = func(geom.Point) input.Cursor { return input.CursorInherit }
	if got := g.Cursor(geom.Pt(1, 1)); got != input.CursorInherit {
		t.Fatalf("named, the grid shows %v", got)
	}
}
