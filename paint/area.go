package paint

import (
	"image/color"
	"slices"

	"github.com/marrasen/gunim/geom"
)

// AreaPoint is a point along an area: a point on its line, the point on
// its base across from it, and the area's colour at each.
type AreaPoint struct {
	// At is the point on the area's line, and Base the one across from
	// it on its base: under it, for the fill under a graph's curve.
	At, Base geom.Point
	// Color is the area's colour at At, and BaseColor its colour at
	// Base.
	Color, BaseColor color.NRGBA
}

// AreaOp fills the area between two lines: the line through its points'
// At and the base through their Base, as the fill under a graph's curve
// down to its foot. The colour is each point's Color along the line and
// its BaseColor along the base, changing evenly both along the area and
// across it, as a fill that fades toward its foot does. The colour
// changes premultiplied by its alpha, so a colour of alpha 0 fades the
// area out without darkening it. Its edges and its ends are smooth.
type AreaOp struct {
	Points    []AreaPoint
	Transform Transform
	// Blend is how the area meets what is beneath it, set from the blend
	// in force as the op is recorded; see [Painter.Blend].
	Blend Blend
}

func (*AreaOp) isOp() {}

// Area records the area between the line through the At of pts and the
// base through their Base, the caller having sampled the line finely
// enough to look smooth: about a point for each 3 px of it. An area of
// fewer than two points draws nothing.
//
// It is one command, of one quad for each stretch between two points,
// however tall the area and whatever the scale: draw the fill under a
// curve with it, never as a shape for each column of pixels.
//
// The line and the base should not cross, and the lines from each At to
// its Base should not cross each other: where they do, the area folds
// over itself, and a see-through one is darker there.
//
// The op holds its own copy of pts, so the caller may change its slice
// once Area returns. The copy is made into a buffer the painter keeps
// from frame to frame, so an area drawn every frame allocates nothing
// once the buffer has grown to the frame's points.
func (p *Painter) Area(pts []AreaPoint) {
	if len(pts) < 2 {
		return
	}
	op := p.areas.take()
	*op = AreaOp{Points: p.copyEdges(pts), Transform: p.at(), Blend: p.blendNow()}
	p.record(op, areaBounds(pts))
}

// takeArea takes an AreaOp from the painter's blocks, a copy of op with
// its points copied into this frame's buffer of them.
func (p *Painter) takeArea(op *AreaOp, pts []AreaPoint) *AreaOp {
	v := p.areas.take()
	*v = *op
	v.Points = p.copyEdges(pts)
	return v
}

// copyEdges copies pts to the end of this frame's buffer of areas'
// points and returns the copy, as copyPoints does a ribbon's.
func (p *Painter) copyEdges(pts []AreaPoint) []AreaPoint {
	at := len(p.edges)
	p.edges = append(p.edges, pts...)
	return p.edges[at:len(p.edges):len(p.edges)]
}

// areaBounds is the box an area through pts can touch, in its own
// space: the box round its points. record grows it for the smoothing of
// its edges.
func areaBounds(pts []AreaPoint) geom.Rect {
	b := geom.Rect{Min: pts[0].At, Max: pts[0].At}
	for _, q := range pts {
		for _, at := range [...]geom.Point{q.At, q.Base} {
			b.Min = geom.Pt(min(b.Min.X, at.X), min(b.Min.Y, at.Y))
			b.Max = geom.Pt(max(b.Max.X, at.X), max(b.Max.Y, at.Y))
		}
	}
	return b
}

// sameArea reports whether two area ops draw the same.
func sameArea(a, b *AreaOp) bool {
	return a.Transform == b.Transform && a.Blend == b.Blend && slices.Equal(a.Points, b.Points)
}
