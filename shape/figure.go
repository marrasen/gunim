package shape

import (
	"image/color"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Figure is vector art: paths filled and stroked in order, on a view box, as an SVG file draws them. [ParseSVG]
// reads one. Each part is a mask rasterized once per size, so a figure of a dozen parts costs a dozen quads a
// frame once drawn.
type Figure struct {
	// ViewBox is the box of the parts' units drawn into the rect [Figure.Paint] is given.
	ViewBox geom.Rect
	// Parts are drawn in order, the first at the back.
	Parts []Part

	// at and grads are the gradients last mapped, into at, so a figure drawn in one place maps them once.
	at    geom.Rect
	grads []*paint.Gradient
}

// Part is one path of a figure: filled, stroked, or both, the fill first.
type Part struct {
	Path *Path
	// Fill colours the inside; FillGradient, where set, in its place, in the view box's units. A fill with
	// neither is not drawn.
	Fill         color.NRGBA
	FillGradient *paint.Gradient
	// EvenOdd fills by the even-odd rule in place of nonzero.
	EvenOdd bool
	// Stroke colours a stroke Width units wide, with round caps and joins. A stroke with no colour or width is
	// not drawn.
	Stroke color.NRGBA
	Width  float32
}

// Paint draws the figure with its view box stretched over r; [Fit] gives the place in r that keeps its shape.
func (f *Figure) Paint(p *paint.Painter, r geom.Rect) {
	if f == nil || r.Empty() || f.ViewBox.Empty() {
		return
	}
	if r != f.at || len(f.grads) != len(f.Parts) {
		f.mapGradients(r)
	}
	for i, pt := range f.Parts {
		if pt.Path == nil {
			continue
		}
		fill := pt.Path.Fill()
		fill.EvenOdd = pt.EvenOdd
		switch {
		case f.grads[i] != nil:
			p.MaskFill(fill, fill.In(f.ViewBox, r), paint.Fill{Gradient: f.grads[i]})
		case pt.Fill.A > 0:
			p.Mask(fill, fill.In(f.ViewBox, r), pt.Fill)
		}
		if pt.Stroke.A > 0 && pt.Width > 0 {
			s := pt.Path.Stroke(pt.Width)
			p.Mask(s, s.In(f.ViewBox, r), pt.Stroke)
		}
	}
}

// mapGradients maps each part's gradient from the view box into r.
func (f *Figure) mapGradients(r geom.Rect) {
	f.at = r
	f.grads = make([]*paint.Gradient, len(f.Parts))
	kx, ky := r.Size().W/f.ViewBox.Size().W, r.Size().H/f.ViewBox.Size().H
	m := func(q geom.Point) geom.Point {
		return geom.Pt(r.Min.X+(q.X-f.ViewBox.Min.X)*kx, r.Min.Y+(q.Y-f.ViewBox.Min.Y)*ky)
	}
	for i, pt := range f.Parts {
		if pt.FillGradient == nil {
			continue
		}
		g := *pt.FillGradient
		g.From, g.To = m(g.From), m(g.To)
		f.grads[i] = &g
	}
}
