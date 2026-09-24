// Package geom holds the float32 geometry types used across gunim.
//
// Everything is float32 because positions are animated: a panel sliding
// in sits at fractional coordinates for most of its travel, and
// rounding each frame is what makes motion look stepped.
package geom

import "math"

// Point is a position in logical pixels.
type Point struct{ X, Y float32 }

// Pt is shorthand for Point{x, y}.
func Pt(x, y float32) Point { return Point{x, y} }

// Add returns p moved by q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p moved back by q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Mul returns p scaled by s.
func (p Point) Mul(s float32) Point { return Point{p.X * s, p.Y * s} }

// Size is a width and height in logical pixels.
type Size struct{ W, H float32 }

// Sz is shorthand for Size{w, h}.
func Sz(w, h float32) Size { return Size{w, h} }

// Point returns s as an offset from the origin.
func (s Size) Point() Point { return Point{s.W, s.H} }

// Rect is an axis-aligned rectangle. Min is the top-left corner.
type Rect struct{ Min, Max Point }

// Rc builds a Rect from a top-left corner and a size.
func Rc(x, y, w, h float32) Rect {
	return Rect{Point{x, y}, Point{x + w, y + h}}
}

// Size returns r's width and height.
func (r Rect) Size() Size { return Size{r.Max.X - r.Min.X, r.Max.Y - r.Min.Y} }

// Center returns the point at the middle of r.
func (r Rect) Center() Point { return Point{(r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2} }

// Add offsets r by p.
func (r Rect) Add(p Point) Rect { return Rect{r.Min.Add(p), r.Max.Add(p)} }

// Inset shrinks r by in on every side.
func (r Rect) Inset(in Insets) Rect {
	return Rect{
		Point{r.Min.X + in.Left, r.Min.Y + in.Top},
		Point{r.Max.X - in.Right, r.Max.Y - in.Bottom},
	}
}

// Contains reports whether p lies inside r. The top-left edge belongs
// to r and the bottom-right edge belongs to the next rectangle along,
// so adjacent rectangles split the boundary cleanly between them.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.X < r.Max.X && p.Y >= r.Min.Y && p.Y < r.Max.Y
}

// Empty reports whether r has zero area.
func (r Rect) Empty() bool { return r.Max.X <= r.Min.X || r.Max.Y <= r.Min.Y }

// Union returns the smallest rectangle containing both r and s. It is
// how the engine accumulates the damage region for a frame.
func (r Rect) Union(s Rect) Rect {
	if r.Empty() {
		return s
	}
	if s.Empty() {
		return r
	}
	return Rect{
		Point{min32(r.Min.X, s.Min.X), min32(r.Min.Y, s.Min.Y)},
		Point{max32(r.Max.X, s.Max.X), max32(r.Max.Y, s.Max.Y)},
	}
}

// Insets is a margin or padding on four sides.
type Insets struct{ Top, Right, Bottom, Left float32 }

// Uniform returns insets with the same value on all four sides.
func Uniform(v float32) Insets { return Insets{v, v, v, v} }

func min32(a, b float32) float32 { return float32(math.Min(float64(a), float64(b))) }
func max32(a, b float32) float32 { return float32(math.Max(float64(a), float64(b))) }

// Normalized returns r with Min and Max swapped where needed, so that
// Min really is the top-left corner. A transform with a negative scale
// can turn a well-formed rectangle inside out.
func (r Rect) Normalized() Rect {
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	return r
}
