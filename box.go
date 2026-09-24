package gunim

import (
	"image/color"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Box is a container that stacks its children on top of one another,
// each filling the space the box was given, painted in order.
//
// It is the default root of a window, and the right parent for anything
// that floats: a dialog, a toast, a drag preview. Because the engine
// keeps exiting children in the tree, a Box is also where the old
// contents and the new contents of a screen coexist for the length of a
// cross-fade.
type Box struct {
	// Padding is inset from every edge before children are placed.
	Padding geom.Insets
	// Fill paints the whole box behind its children. The zero colour
	// paints nothing.
	Fill color.NRGBA
}

// Layout implements [Node].
func (b *Box) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	size := c.Max
	inner := geom.Rect{Max: size.Point()}.Inset(b.Padding)
	for kid := range kids.All {
		kid.Layout(Tight(inner.Size()))
		kid.Place(inner.Min)
	}
	return size
}

// Paint implements [Node].
func (b *Box) Paint(p *paint.Painter, _ Frame, box geom.Size, kids Children) {
	if b.Fill.A > 0 {
		p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(b.Fill))
	}
	for kid := range kids.All {
		kid.Paint(p)
	}
}
