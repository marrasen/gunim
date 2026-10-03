package main

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A candy is how a digit looks: each its own colour and shape, so the
// board reads by colour as well as by number.
type candy struct {
	color color.NRGBA
	shape int
}

// The shapes a candy takes.
const (
	shapeCircle = iota
	shapeSquare
	shapeStar
	shapeDrop
	shapeDiamond
	shapeHexagon
	shapeHeart
	shapeBean
	shapeTriangle
)

// candies are the digits 1 to 9's candies, at their index less one.
var candies = [9]candy{
	{rgb(0xff, 0x3b, 0x5c), shapeCircle},
	{rgb(0xff, 0x93, 0x1f), shapeSquare},
	{rgb(0xff, 0xd2, 0x1f), shapeStar},
	{rgb(0x3d, 0xd6, 0x5a), shapeDrop},
	{rgb(0x2f, 0x8c, 0xff), shapeDiamond},
	{rgb(0x9b, 0x5c, 0xff), shapeHexagon},
	{rgb(0xff, 0x5c, 0xc8), shapeHeart},
	{rgb(0x1f, 0xd6, 0xd6), shapeBean},
	{rgb(0xa8, 0x6a, 0x3c), shapeTriangle},
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 0xff} }

// candyOf returns digit d's candy.
func candyOf(d int8) candy { return candies[max(1, min(d, 9))-1] }

// candyMask is a candy's shape as a mask: grown by grow, a fraction of
// its half-width, or shrunk where grow is negative. It is comparable,
// so the driver keeps each size's mask once made.
type candyMask struct {
	shape int
	grow  float32
}

// Settled implements [paint.Shape].
func (candyMask) Settled() bool { return true }

// Coverage implements [paint.Shape]: the shape, edges smoothed over a
// pixel, in a square of w by h.
func (m candyMask) Coverage(w, h int) []byte {
	out := make([]byte, w*h)
	half := float64(min(w, h)) / 2
	for y := range h {
		for x := range w {
			px := (float64(x) + 0.5 - float64(w)/2) / half
			py := (float64(y) + 0.5 - float64(h)/2) / half
			d := shapeDistance(m.shape, px, py) - float64(m.grow)
			a := 0.5 - d*half
			out[y*w+x] = uint8(255 * max(0, min(1, a)))
		}
	}
	return out
}

// shapeDistance returns how far (x, y) lies outside shape, in units of
// the candy's half-width, y down: negative inside.
func shapeDistance(shape int, x, y float64) float64 {
	switch shape {
	case shapeCircle:
		return math.Hypot(x, y) - 0.86
	case shapeSquare:
		return roundBox(x, y, 0.8, 0.8, 0.28)
	case shapeStar:
		return polygonDistance(starPoints, x, y-0.04) - 0.1
	case shapeDrop:
		return polygonDistance(dropPoints, x, y) - 0.04
	case shapeDiamond:
		return polygonDistance(diamondPoints, x, y) - 0.12
	case shapeHexagon:
		return polygonDistance(hexagonPoints, x, y) - 0.1
	case shapeHeart:
		return polygonDistance(heartPoints, x, y) - 0.04
	case shapeBean:
		// A capsule, tilted.
		c, s := math.Cos(-0.45), math.Sin(-0.45)
		rx, ry := c*x-s*y, s*x+c*y
		return segmentDistance(rx, ry, -0.42, 0, 0.42, 0) - 0.48
	}
	return polygonDistance(trianglePoints, x, y) - 0.14
}

// roundBox is the distance to a box of half-sizes hx and hy, its
// corners rounded by r.
func roundBox(x, y, hx, hy, r float64) float64 {
	qx, qy := math.Abs(x)-hx+r, math.Abs(y)-hy+r
	return math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - r
}

func segmentDistance(x, y, ax, ay, bx, by float64) float64 {
	px, py := x-ax, y-ay
	ex, ey := bx-ax, by-ay
	t := max(0, min(1, (px*ex+py*ey)/(ex*ex+ey*ey)))
	return math.Hypot(px-ex*t, py-ey*t)
}

// polygonDistance is the signed distance to a polygon, negative inside.
func polygonDistance(pts [][2]float64, x, y float64) float64 {
	d := math.Inf(1)
	inside := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		a, b := pts[j], pts[i]
		d = min(d, segmentDistance(x, y, a[0], a[1], b[0], b[1]))
		if (a[1] > y) != (b[1] > y) && x < (b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0] {
			inside = !inside
		}
	}
	if inside {
		return -d
	}
	return d
}

var (
	starPoints     = starPolygon(5, 0.9, 0.42)
	diamondPoints  = [][2]float64{{0, -0.86}, {0.74, 0}, {0, 0.86}, {-0.74, 0}}
	hexagonPoints  = regular(6, 0.84, math.Pi/6)
	trianglePoints = [][2]float64{{0, -0.74}, {0.78, 0.62}, {-0.78, 0.62}}
	heartPoints    = heartPolygon()
	dropPoints     = dropPolygon()
)

// regular returns the corners of a regular polygon of n sides, radius
// r, turned by turn.
func regular(n int, r, turn float64) [][2]float64 {
	pts := make([][2]float64, n)
	for i := range pts {
		a := turn + 2*math.Pi*float64(i)/float64(n)
		pts[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	return pts
}

// starPolygon returns a star of n points, from radius outer to inner,
// a point at the top.
func starPolygon(n int, outer, inner float64) [][2]float64 {
	pts := make([][2]float64, 2*n)
	for i := range pts {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		a := -math.Pi/2 + math.Pi*float64(i)/float64(n)
		pts[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	return pts
}

// heartPolygon returns a heart, from the curve x = 16 sin³t,
// y = 13 cos t − 5 cos 2t − 2 cos 3t − cos 4t.
func heartPolygon() [][2]float64 {
	const n = 64
	pts := make([][2]float64, n)
	for i := range pts {
		t := 2 * math.Pi * float64(i) / n
		s := math.Sin(t)
		x := 16 * s * s * s
		y := 13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)
		pts[i] = [2]float64{x / 18.5, -y/18.5 - 0.06}
	}
	return pts
}

// dropPolygon returns a drop, round at the bottom and pointed at the
// top.
func dropPolygon() [][2]float64 {
	const n = 40
	pts := make([][2]float64, 0, n+2)
	pts = append(pts, [2]float64{0, -0.92})
	cy, r := 0.26, 0.6
	// The round part, from the right side round the bottom to the left.
	from, to := -0.18*math.Pi, 1.18*math.Pi
	for i := range n + 1 {
		a := from + (to-from)*float64(i)/n
		pts = append(pts, [2]float64{r * math.Cos(a), cy + r*math.Sin(a)})
	}
	return pts
}

// lighter is c mixed toward white by t, and darker toward black.
func lighter(c color.NRGBA, t float32) color.NRGBA { return mixC(c, rgb(0xff, 0xff, 0xff), t) }
func darker(c color.NRGBA, t float32) color.NRGBA  { return mixC(c, rgb(0, 0, 0), t) }

func mixC(a, b color.NRGBA, t float32) color.NRGBA {
	t = max(0, min(1, t))
	m := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: m(a.A, b.A)}
}

// faded is c at alpha a, from 0 to 1.
func faded(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * max(0, min(1, a)))
	return c
}

// paintCandy draws digit d's candy filling r, at alpha, with its digit
// on it when numbered. Scale it with the painter's transform, so its
// masks are made once at r's size.
func paintCandy(p *paint.Painter, d int8, r geom.Rect, alpha float32, numbered bool) {
	if alpha <= 0.01 {
		return
	}
	k := candyOf(d)
	s := r.Size().W
	// A shadow under it, an edge darker than its body, the body, and a
	// lighter middle, raised a little, to round it.
	p.Mask(candyMask{shape: k.shape}, r.Add(geom.Pt(0, s*0.07)), faded(rgb(0x1a, 0x05, 0x30), 0.35*alpha))
	p.Mask(candyMask{shape: k.shape, grow: 0.05}, r, faded(darker(k.color, 0.35), alpha))
	p.Mask(candyMask{shape: k.shape}, r, faded(k.color, alpha))
	inner := geom.Rc(r.Min.X+s*0.12, r.Min.Y+s*0.08, s*0.76, s*0.76)
	p.Mask(candyMask{shape: k.shape, grow: -0.08}, inner, faded(lighter(k.color, 0.22), alpha))
	// The shine: a soft white streak at the top left, and a glint.
	func() {
		defer p.Push(paint.Rotate(-0.5, geom.Pt(r.Min.X+s*0.36, r.Min.Y+s*0.26)))()
		p.RRect(geom.Rc(r.Min.X+s*0.22, r.Min.Y+s*0.2, s*0.3, s*0.13), s*0.065, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.55*alpha)))
	}()
	p.RRect(geom.Rc(r.Min.X+s*0.66, r.Min.Y+s*0.3, s*0.07, s*0.07), s*0.035, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.75*alpha)))
	if numbered {
		paintDigit(p, d, r, alpha)
	}
}

// paintDigit draws d in the middle of r, white over a dark edge.
func paintDigit(p *paint.Painter, d int8, r geom.Rect, alpha float32) {
	s := r.Size().W
	run := shaped(string(rune('0'+d)), s*0.42, true)
	at := geom.Pt(r.Min.X+(s-run.Advance)/2, r.Min.Y+(s-run.Ascent-run.Descent)/2+s*0.03)
	shadow := faded(darker(candyOf(d).color, 0.55), 0.8*alpha)
	for _, o := range []geom.Point{{X: 0, Y: s * 0.03}, {X: s * 0.02, Y: s * 0.02}, {X: -s * 0.02, Y: s * 0.02}} {
		run.Paint(p, at.Add(o), shadow)
	}
	run.Paint(p, at, faded(rgb(0xff, 0xff, 0xff), alpha))
}
