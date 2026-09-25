package widget

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The pointer's shape over each widget; see [gunim.CursorShaper].

// Cursor implements [gunim.CursorShaper]: the I-beam, over text.
func (t *TextField) Cursor(geom.Point) input.Cursor { return input.CursorText }

// Cursor implements [gunim.CursorShaper]: the I-beam, over text.
func (a *TextArea) Cursor(geom.Point) input.Cursor { return input.CursorText }

// Cursor implements [gunim.CursorShaper]: the I-beam, over text.
func (g *CellGrid) Cursor(geom.Point) input.Cursor { return input.CursorText }
