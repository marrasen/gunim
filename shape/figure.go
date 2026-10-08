package shape

import (
	"image/color"
	"math"

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
}

// Part is one path of a figure: filled, stroked, or both, the fill first.
type Part struct {
	Path *Path
	// Fill colours the inside; FillGradient, where set, colours it in Fill's place, in the view box's units. The
	// inside is drawn when Fill has some alpha or FillGradient is set.
	Fill         color.NRGBA
	FillGradient *paint.Gradient
	// EvenOdd fills by the even-odd rule in place of nonzero.
	EvenOdd bool
	// Stroke colours a stroke Width units wide, with round caps and joins, drawn when both Stroke's alpha and
	// Width are above zero.
	Stroke color.NRGBA
	Width  float32
}

// Paint draws the figure with its view box stretched over r; [Fit] gives the place in r that keeps its shape.
// Paint only reads the figure, so one figure may be painted at many places, and from many goroutines at once.
func (f *Figure) Paint(p *paint.Painter, r geom.Rect) {
	if f == nil || r.Empty() || f.ViewBox.Empty() {
		return
	}
	kx, ky := r.Size().W/f.ViewBox.Size().W, r.Size().H/f.ViewBox.Size().H
	m := func(q geom.Point) geom.Point {
		return geom.Pt(r.Min.X+(q.X-f.ViewBox.Min.X)*kx, r.Min.Y+(q.Y-f.ViewBox.Min.Y)*ky)
	}
	for _, pt := range f.Parts {
		if pt.Path == nil {
			continue
		}
		fill := pt.Path.Fill()
		fill.EvenOdd = pt.EvenOdd
		switch {
		case pt.FillGradient != nil:
			// The gradient, in the view box's units, is mapped into r afresh: a copy for this paint alone.
			g := *pt.FillGradient
			if g.Radial && kx != ky {
				g.Aspect = stretched(g, kx, ky)
			}
			g.From, g.To = m(g.From), m(g.To)
			p.MaskFill(fill, fill.In(f.ViewBox, r), paint.Fill{Gradient: &g})
		case pt.Fill.A > 0:
			p.Mask(fill, fill.In(f.ViewBox, r), pt.Fill)
		}
		if pt.Stroke.A > 0 && pt.Width > 0 {
			s := pt.Path.Stroke(pt.Width)
			p.Mask(s, s.In(f.ViewBox, r), pt.Stroke)
		}
	}
}

// stretched returns the Aspect of the radial gradient g once stretched by
// kx across and ky down: the length of its axis across the line to To
// over the length along it. An ellipse turned to the stretch keeps its
// axes square; one turned against it is drawn with axes of the new
// lengths, set square.
func stretched(g paint.Gradient, kx, ky float32) float32 {
	asp := g.Aspect
	if asp <= 0 {
		asp = 1
	}
	d := g.To.Sub(g.From)
	along := geom.Pt(d.X*kx, d.Y*ky)
	across := geom.Pt(-d.Y*asp*kx, d.X*asp*ky)
	n := float32(math.Hypot(float64(along.X), float64(along.Y)))
	if n <= 0 {
		return g.Aspect
	}
	return float32(math.Hypot(float64(across.X), float64(across.Y))) / n
}
