package paint

import (
	"image/color"

	"github.com/marrasen/gunim/geom"
)

// A Shape is artwork a driver rasterizes into a coverage mask and tints with a colour, such as an icon. It must be
// comparable, as a driver keys the masks it keeps by the shape and the size in pixels.
type Shape interface {
	// Coverage returns w*h bytes, row by row from the top, of the shape drawn to fill w by h device pixels: 0 outside,
	// 255 inside.
	Coverage(w, h int) []byte
	// Settled reports whether the shape stays as it is, so a driver may keep its coverage for later frames.
	Settled() bool
}

// MaskOp draws a Shape's coverage into Rect, tinted by Color.
type MaskOp struct {
	Shape     Shape
	Rect      geom.Rect
	Color     color.NRGBA
	Transform Transform
}

func (*MaskOp) isOp() {}

// Mask records s drawn into r and tinted by c. A driver snaps r to whole device pixels while the transform in force
// only moves it.
func (p *Painter) Mask(s Shape, r geom.Rect, c color.NRGBA) {
	if s == nil || c.A == 0 || r.Empty() {
		return
	}
	p.record(&MaskOp{Shape: s, Rect: r, Color: c, Transform: p.at()}, r)
}
