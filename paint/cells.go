package paint

import (
	"image/color"
	"slices"

	"github.com/marrasen/gunim/geom"
)

// CellsOp draws a row of character cells in one go: each cell's
// background, and over it a pattern in its foreground colour, such as a
// half block. It is for a terminal's grid, where an animation changes
// every cell of every row in every frame, and drawing each cell as
// rectangles would send the GPU tens of megabytes a frame.
//
// A driver draws the rows of one grid that follow each other, as a
// grid paints them, together.
type CellsOp struct {
	// At is the row's top left corner, and Size a cell's size, in
	// logical pixels. A cell is a whole number of device pixels.
	At   geom.Point
	Size geom.Size
	// Cells are the row's cells, from the left. The op keeps the slice,
	// which must not change until the frame has been drawn.
	Cells []CellPaint
	// Patterns holds the patterns the cells name.
	Patterns *Patterns
	// Grid tells one grid's rows from another's, and Row is this row's
	// number in it, so a driver knows which rows follow each other.
	Grid uintptr
	Row  int

	Transform Transform
}

// CellPaint is how one cell of a [CellsOp] is drawn: BG fills the cell,
// and Pattern, when not zero, is the number of a mask in the op's
// Patterns, counting from one, drawn over it in FG.
type CellPaint struct {
	BG, FG  color.NRGBA
	Pattern uint16
}

// Patterns are the masks the cells of a [CellsOp] draw in their
// foreground colour, each W by H device pixels: a cell's size. Masks[i]
// is pattern i+1, W*H bytes of coverage row by row from the top. A grid
// adds masks as it meets new characters, and makes new Patterns when
// its cells change size; a driver keeps what it has uploaded of each.
type Patterns struct {
	W, H  int
	Masks [][]byte
}

func (*CellsOp) isOp() {}

// Cells records a row of cells, of a cell size and with a grid and row
// as [CellsOp] describes, its top left corner at at.
func (p *Painter) Cells(at geom.Point, size geom.Size, cells []CellPaint, pats *Patterns, grid uintptr, row int) {
	if len(cells) == 0 {
		return
	}
	r := geom.Rect{Min: at, Max: geom.Pt(at.X+size.W*float32(len(cells)), at.Y+size.H)}
	p.record(&CellsOp{At: at, Size: size, Cells: cells, Patterns: pats, Grid: grid, Row: row, Transform: p.at()}, r)
}

// sameCells reports whether two rows of cells draw the same thing.
func sameCells(a, b *CellsOp) bool {
	return a.At == b.At && a.Size == b.Size && a.Patterns == b.Patterns && a.Grid == b.Grid && a.Row == b.Row &&
		a.Transform == b.Transform && slices.Equal(a.Cells, b.Cells)
}
