package anim

import (
	"image/color"

	"github.com/marrasen/gunim/geom"
)

// These aliases name the values a widget animates, so a field
// declaration reads as `hover *anim.Float`.

// Float is an animated number: an opacity, an angle, a 0..1 progress.
type Float = Animated[float32]

// Point is an animated position.
type Point = Animated[geom.Point]

// Size is an animated width and height.
type Size = Animated[geom.Size]

// Rect is an animated rectangle.
type Rect = Animated[geom.Rect]

// Color is an animated colour.
type Color = Animated[color.NRGBA]

// FloatCodec animates a plain number: an opacity, an angle, a 0..1
// progress value driving everything else.
var FloatCodec = Codec[float32]{
	N:      1,
	Encode: func(v float32, d *[MaxScalars]float32) { d[0] = v },
	Decode: func(s *[MaxScalars]float32) float32 { return s[0] },
}

// PointCodec animates a position as two independent springs.
var PointCodec = Codec[geom.Point]{
	N: 2,
	Encode: func(v geom.Point, d *[MaxScalars]float32) {
		d[0], d[1] = v.X, v.Y
	},
	Decode: func(s *[MaxScalars]float32) geom.Point {
		return geom.Point{X: s[0], Y: s[1]}
	},
}

// SizeCodec animates a width and height.
var SizeCodec = Codec[geom.Size]{
	N: 2,
	Encode: func(v geom.Size, d *[MaxScalars]float32) {
		d[0], d[1] = v.W, v.H
	},
	Decode: func(s *[MaxScalars]float32) geom.Size {
		return geom.Size{W: s[0], H: s[1]}
	},
}

// RectCodec animates a rectangle's two corners, which is what you want
// for a focus ring or a selection that slides between targets.
var RectCodec = Codec[geom.Rect]{
	N: 4,
	Encode: func(v geom.Rect, d *[MaxScalars]float32) {
		d[0], d[1], d[2], d[3] = v.Min.X, v.Min.Y, v.Max.X, v.Max.Y
	},
	Decode: func(s *[MaxScalars]float32) geom.Rect {
		return geom.Rect{Min: geom.Point{X: s[0], Y: s[1]}, Max: geom.Point{X: s[2], Y: s[3]}}
	},
}

// ColorCodec animates a colour channel by channel.
//
// It interpolates in sRGB with the values as they are stored, which is
// cheap and wrong: a fade between saturated colours passes through a
// muddy middle. It is good enough for tinting and highlights. A linear
// or Oklab codec belongs here too, once there is something to look at.
var ColorCodec = Codec[color.NRGBA]{
	N: 4,
	Encode: func(v color.NRGBA, d *[MaxScalars]float32) {
		d[0], d[1], d[2], d[3] = float32(v.R), float32(v.G), float32(v.B), float32(v.A)
	},
	Decode: func(s *[MaxScalars]float32) color.NRGBA {
		return color.NRGBA{R: clampByte(s[0]), G: clampByte(s[1]), B: clampByte(s[2]), A: clampByte(s[3])}
	},
}

// NewFloat returns an animated number holding v, at rest.
func NewFloat(v float32) *Float { return New(v, FloatCodec) }

// NewPoint returns an animated position holding v, at rest.
func NewPoint(v geom.Point) *Point { return New(v, PointCodec) }

// NewSize returns an animated size holding v, at rest.
func NewSize(v geom.Size) *Size { return New(v, SizeCodec) }

// NewRect returns an animated rectangle holding v, at rest.
func NewRect(v geom.Rect) *Rect { return New(v, RectCodec) }

// NewColor returns an animated colour holding v, at rest.
func NewColor(v color.NRGBA) *Color { return New(v, ColorCodec) }

func clampByte(v float32) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	default:
		return uint8(v + 0.5)
	}
}

// Mix returns the value t of the way from a to b, blending each scalar
// the codec takes apart. t may leave 0..1, so a spring's overshoot
// carries through.
//
// It is how a widget turns an animated state into a look: animate the
// hover from 0 to 1, and Mix the theme's idle and hover colours by it
// each frame, so the colours can change underneath without the
// animation noticing.
func Mix[T any](c Codec[T], a, b T, t float32) T {
	var x, y [MaxScalars]float32
	c.Encode(a, &x)
	c.Encode(b, &y)
	for i := range c.N {
		x[i] += (y[i] - x[i]) * t
	}
	return c.Decode(&x)
}

// InsetsCodec animates the four sides of a padding or margin.
var InsetsCodec = Codec[geom.Insets]{
	N: 4,
	Encode: func(v geom.Insets, d *[MaxScalars]float32) {
		d[0], d[1], d[2], d[3] = v.Top, v.Right, v.Bottom, v.Left
	},
	Decode: func(s *[MaxScalars]float32) geom.Insets {
		return geom.Insets{Top: s[0], Right: s[1], Bottom: s[2], Left: s[3]}
	},
}

// SpringCodec animates a spring's response and damping, so a theme can
// make every motion gradually snappier or softer.
var SpringCodec = Codec[Spring]{
	N: 2,
	Encode: func(v Spring, d *[MaxScalars]float32) {
		d[0], d[1] = v.Response, v.Damping
	},
	Decode: func(s *[MaxScalars]float32) Spring {
		return Spring{Response: s[0], Damping: s[1]}
	},
}
