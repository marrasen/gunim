package widget

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The pointer's shape over each widget; see [gunim.CursorShaper].

// Cursor implements [gunim.CursorShaper]: the I-beam over text, and an arrow over the X.
func (t *TextField) Cursor(p geom.Point) input.Cursor {
	if t.overClear(p) {
		return input.CursorArrow
	}
	return input.CursorText
}

// Cursor implements [gunim.CursorShaper]: the I-beam, over text.
func (a *TextArea) Cursor(geom.Point) input.Cursor { return input.CursorText }

// Cursor implements [gunim.CursorShaper]: the I-beam, over text.
func (g *CellGrid) Cursor(geom.Point) input.Cursor { return input.CursorText }

// Cursor implements [gunim.CursorShaper]: the I-beam over a selectable
// label, and the shape around it otherwise.
func (l *Label) Cursor(geom.Point) input.Cursor {
	if l.Selectable {
		return input.CursorText
	}
	return input.CursorInherit
}
