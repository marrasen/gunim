// Package shape draws vector art from SVG: a path filled or stroked as a coverage mask a driver tints, and a
// [Figure], many paths with their colours and gradients, read from an SVG file.
//
// A shape is rasterized once for each size in pixels it is drawn at, and kept, as an icon is: drawing it again,
// moved, turned, scaled by a transform, faded or tinted another colour, costs a quad. Drawing it at a new size in
// pixels rasterizes it again, so an animation moves and scales a shape by a transform rather than by its rect.
//
// A driver keeps each mask at most 256 device pixels across, or down, and stretches a bigger one from that, as it
// does any settled mask, so a mask past 256 pixels draws softer the bigger it is. The limit holds for each path,
// and so for each part of a [Figure], not for the figure as a whole: a figure drawn 400 pixels wide whose largest
// part is 250 pixels across draws sharp. On a screen of two pixels to a unit, 256 pixels are 128 units.
//
//	leaf, err := shape.NewPath("M12 2C6 8 6 16 12 22C18 16 18 8 12 2Z")
//	if err != nil {
//		return err
//	}
//	box := geom.Rc(0, 0, 24, 24)
//	p.Mask(leaf.Fill(), leaf.Fill().In(box, r), green) // the 24-unit grid drawn into r
//	p.Mask(leaf.Stroke(1.5), leaf.Stroke(1.5).In(box, r), darkGreen)
package shape

import (
	"errors"
	"fmt"
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/vecpath"
)

// Path is SVG path data, in whatever units its numbers are: lines, cubic and quadratic Béziers and arcs, in one or
// more subpaths. Its data is fixed once read.
type Path struct {
	subs   []vecpath.Subpath
	lo, hi vecpath.Pt
}

// NewPath reads SVG path data, as the d attribute of an SVG path holds it. Path data whose points, or the width
// and height of the box they lie in, reach past float32 is an error.
func NewPath(d string) (*Path, error) {
	subs, err := vecpath.Parse(d)
	if err != nil {
		return nil, fmt.Errorf("shape: %w", err)
	}
	return pathOf(subs)
}

// errTooFar is the error for a path whose points, or the distances between them, reach past float32.
var errTooFar = errors.New("shape: the path reaches past the range of float32")

// pathOf is the path of subpaths already read. Its bounds, and their width and height, must be finite numbers;
// errTooFar says they reach past float32.
func pathOf(subs []vecpath.Subpath) (*Path, error) {
	p := &Path{subs: subs}
	p.lo, p.hi, _ = vecpath.Bounds(subs)
	if !finite(p.lo.X, p.lo.Y, p.hi.X, p.hi.Y, p.hi.X-p.lo.X, p.hi.Y-p.lo.Y) {
		return nil, errTooFar
	}
	return p, nil
}

// finite reports whether every v is a finite number.
func finite(v ...float32) bool {
	for _, x := range v {
		if x-x != 0 {
			return false
		}
	}
	return true
}

// Bounds is the box the path lies in, in its own units, control points included.
func (p *Path) Bounds() geom.Rect {
	return geom.Rect{Min: geom.Pt(p.lo.X, p.lo.Y), Max: geom.Pt(p.hi.X, p.hi.Y)}
}

// Fill is the path filled by the nonzero rule.
func (p *Path) Fill() Fill { return Fill{Path: p} }

// Stroke is the path stroked width units wide.
func (p *Path) Stroke(width float32) Stroke { return Stroke{Path: p, Width: width} }

// Fill is a path filled, its edges smoothed, as a [paint.Shape]. Its coverage covers the path's bounds; [Fill.In]
// says where to draw it.
type Fill struct {
	Path *Path
	// EvenOdd fills by the even-odd rule in place of nonzero: a subpath inside another is a hole whichever way it
	// runs.
	EvenOdd bool
}

// Settled implements [paint.Shape]: a fill is always settled, as its path stays as it was read.
func (Fill) Settled() bool { return true }

// In is where the fill draws when the box of its path's units, such as an SVG's view box, is drawn into r.
func (f Fill) In(box, r geom.Rect) geom.Rect { return mapRect(f.Path.Bounds(), box, r) }

// Coverage implements [paint.Shape]: the path's bounds drawn into w by h pixels.
func (f Fill) Coverage(w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}
	if f.Path == nil || len(f.Path.subs) == 0 {
		return make([]byte, w*h)
	}
	m, _ := toPixels(f.Path.Bounds(), w, h)
	return vecpath.Fill(vecpath.Flatten(f.Path.subs, m, true, nil), w, h, f.EvenOdd)
}

// Stroke is a path stroked with round caps and joins, as a [paint.Shape]. Its coverage covers the path's bounds
// grown to hold the stroke; [Stroke.In] says where to draw it.
type Stroke struct {
	Path  *Path
	Width float32
}

// Settled implements [paint.Shape]: a stroke whose width is a number is settled. A NaN width equals nothing, the
// last frame's included, so such a stroke is drawn again each frame.
func (s Stroke) Settled() bool { return s.Width == s.Width }

// In is where the stroke draws when the box of its path's units is drawn into r.
func (s Stroke) In(box, r geom.Rect) geom.Rect { return mapRect(s.bounds(), box, r) }

// bounds is the path's bounds grown by the stroke, and a little more for its smoothed edge.
func (s Stroke) bounds() geom.Rect {
	g := max(s.Width, 0) * 0.75
	b := s.Path.Bounds()
	return geom.Rect{Min: geom.Pt(b.Min.X-g, b.Min.Y-g), Max: geom.Pt(b.Max.X+g, b.Max.Y+g)}
}

// Coverage implements [paint.Shape]: the stroke's bounds drawn into w by h pixels. Drawn wider than its units'
// shape, the stroke keeps one width, the mean of the two scales.
func (s Stroke) Coverage(w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}
	if s.Path == nil || len(s.Path.subs) == 0 || !(s.Width > 0) {
		return make([]byte, w*h)
	}
	b := s.bounds()
	if !finite(b.Min.X, b.Min.Y, b.Max.X, b.Max.Y) {
		return make([]byte, w*h)
	}
	m, k := toPixels(b, w, h)
	dist := vecpath.NewDistances(w, h)
	hw := s.Width * k / 2
	for _, l := range vecpath.Flatten(s.Path.subs, m, false, nil) {
		for i := 1; i < len(l.Pts); i++ {
			vecpath.Segment(dist, w, h, l.Pts[i-1], l.Pts[i], hw)
		}
		if len(l.Pts) == 1 {
			vecpath.Segment(dist, w, h, l.Pts[0], l.Pts[0], hw)
		}
	}
	return vecpath.Coverage(dist, make([]bool, w*h), hw)
}

// toPixels maps b onto w by h pixels, and gives the mean scale.
func toPixels(b geom.Rect, w, h int) (m func(vecpath.Pt) vecpath.Pt, k float32) {
	bw, bh := b.Size().W, b.Size().H
	kx, ky := float32(1), float32(1)
	// A side too thin for its scale to be a float32 counts as no side at all.
	if k := float32(w) / bw; bw > 0 && finite(k) {
		kx = k
	} else {
		bw = 0
	}
	if k := float32(h) / bh; bh > 0 && finite(k) {
		ky = k
	} else {
		bh = 0
	}
	switch {
	case bw <= 0:
		kx = ky
	case bh <= 0:
		ky = kx
	}
	m = func(p vecpath.Pt) vecpath.Pt { return vecpath.Pt{X: (p.X - b.Min.X) * kx, Y: (p.Y - b.Min.Y) * ky} }
	return m, float32(math.Sqrt(float64(kx * ky)))
}

// mapRect is where b, in box's units, lies when box is drawn into r. Where box is empty, or the place is no
// finite rect, it is the empty rect at r's corner, which draws nothing.
func mapRect(b, box, r geom.Rect) geom.Rect {
	nothing := geom.Rect{Min: r.Min, Max: r.Min}
	if box.Empty() {
		return nothing
	}
	kx, ky := r.Size().W/box.Size().W, r.Size().H/box.Size().H
	at := geom.Rect{
		Min: geom.Pt(r.Min.X+(b.Min.X-box.Min.X)*kx, r.Min.Y+(b.Min.Y-box.Min.Y)*ky),
		Max: geom.Pt(r.Min.X+(b.Max.X-box.Min.X)*kx, r.Min.Y+(b.Max.Y-box.Min.Y)*ky),
	}
	if !finite(at.Min.X, at.Min.Y, at.Max.X, at.Max.Y) {
		return nothing
	}
	return at
}

// Fit is the largest rect of box's shape centred in r, as an SVG's view box is drawn into a place of another
// shape.
func Fit(box, r geom.Rect) geom.Rect {
	if box.Size().W <= 0 || box.Size().H <= 0 {
		return r
	}
	k := min(r.Size().W/box.Size().W, r.Size().H/box.Size().H)
	w, h := box.Size().W*k, box.Size().H*k
	c := r.Center()
	return geom.Rc(c.X-w/2, c.Y-h/2, w, h)
}
