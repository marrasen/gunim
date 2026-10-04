package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

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
// pixel, in a square of w by h. It reads the shape's distance field,
// made once, so a mask of any size costs a lookup a pixel.
func (m candyMask) Coverage(w, h int) []byte {
	out := make([]byte, w*h)
	half := float64(min(w, h)) / 2
	f := fieldOf(m.shape)
	for y := range h {
		for x := range w {
			px := (float64(x) + 0.5 - float64(w)/2) / half
			py := (float64(y) + 0.5 - float64(h)/2) / half
			d := f.at(px, py) - float64(m.grow)
			a := 0.5 - d*half
			out[y*w+x] = uint8(255 * max(0, min(1, a)))
		}
	}
	return out
}

// A field is a shape's distance, sampled on a grid over the square from
// -fieldSpan to fieldSpan, read between its points by blending the four
// round it.
type field []float32

const (
	fieldN    = 128
	fieldSpan = 1.05
)

var (
	fieldsOnce [9]sync.Once
	fields     [9]field
)

// fieldOf returns shape's field, made the first time it is asked for,
// from whichever goroutine draws first.
func fieldOf(shape int) field {
	fieldsOnce[shape].Do(func() {
		f := make(field, fieldN*fieldN)
		for j := range fieldN {
			for i := range fieldN {
				x := (float64(i)/(fieldN-1)*2 - 1) * fieldSpan
				y := (float64(j)/(fieldN-1)*2 - 1) * fieldSpan
				f[j*fieldN+i] = float32(shapeDistance(shape, x, y))
			}
		}
		fields[shape] = f
	})
	return fields[shape]
}

// at returns the distance at (x, y).
func (f field) at(x, y float64) float64 {
	gx := (x/fieldSpan + 1) / 2 * (fieldN - 1)
	gy := (y/fieldSpan + 1) / 2 * (fieldN - 1)
	gx = max(0, min(gx, fieldN-1.001))
	gy = max(0, min(gy, fieldN-1.001))
	i, j := int(gx), int(gy)
	fx, fy := float32(gx-float64(i)), float32(gy-float64(j))
	a, b := f[j*fieldN+i], f[j*fieldN+i+1]
	c, d := f[(j+1)*fieldN+i], f[(j+1)*fieldN+i+1]
	top := a + (b-a)*fx
	bottom := c + (d-c)*fx
	return float64(top + (bottom-top)*fy)
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

// shapeAt draws shape's mask for r, scaled by k about r's middle and
// moved by off. A rim, a middle or a shine is the same mask drawn
// larger, smaller or aside, so each shape and size makes one mask,
// which the renderer keeps.
func shapeAt(p *paint.Painter, shape int, r geom.Rect, k float32, off geom.Point, c color.NRGBA) {
	mid := geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if off != (geom.Point{}) {
		defer p.Push(paint.Translate(off))()
	}
	if k != 1 {
		defer p.Push(paint.Scale(k, mid))()
	}
	p.Mask(candyMask{shape: shape}, r, c)
}

// paintCandy draws digit d's candy filling r, at alpha, with its digit
// on it when numbered, on a screen of scale device pixels to the
// logical one. The candy is a sprite, its shadow, rim, body, middle and
// shine made once in one image for its size, so the GPU fills its
// pixels once rather than once a layer.
func paintCandy(p *paint.Painter, d int8, r geom.Rect, alpha float32, numbered bool, scale float32) {
	if alpha <= 0.01 {
		return
	}
	s := r.Size().W
	sheet, src := candySprite(d, int(math.Ceil(float64(s*scale*(1+2*spriteMargin)))))
	m := s * spriteMargin
	p.Image(sheet, geom.Rect{Min: r.Min.Sub(geom.Pt(m, m)), Max: r.Max.Add(geom.Pt(m, m))}, paint.ImageOpts{Src: src, Opacity: alpha})
	if numbered {
		paintDigit(p, d, r, alpha)
	}
}

// spriteMargin is the room round a candy's sprite, a fraction of the
// candy, for its rim and its shadow.
const spriteMargin = 0.12

// sheets holds the candies' sprites, the nine of each size in one
// image, so a board of them draws from one texture in one batch: by
// size, rounded up to 8 pixels. It is reached from the UI goroutine
// alone.
var sheets = map[int]*paint.Image{}

// sheetPad is the empty room round each sprite on its sheet, so a
// sprite drawn smoothly never takes in its neighbour's edge.
const sheetPad = 2

// candySprite returns the sheet holding digit d's candy of about px
// pixels square, made the first time, and the part of it the candy is.
func candySprite(d int8, px int) (*paint.Image, geom.Rect) {
	px = max(8, (px+7)/8*8)
	cell := px + 2*sheetPad
	i := int(max(1, min(d, 9)) - 1)
	x, y := float32(i%3*cell+sheetPad), float32(i/3*cell+sheetPad)
	src := geom.Rc(x, y, float32(px), float32(px))
	if img := sheets[px]; img != nil {
		return img, src
	}
	if len(sheets) > 24 {
		clear(sheets)
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, 3*cell, 3*cell))
	for k := range 9 {
		at := image.Pt(k%3*cell+sheetPad, k/3*cell+sheetPad)
		draw.Draw(sheet, image.Rectangle{Min: at, Max: at.Add(image.Pt(px, px))}, drawCandy(int8(k+1), px), image.Point{}, draw.Src)
	}
	img := paint.NewImage(sheet)
	sheets[px] = img
	return img, src
}

// drawCandy draws digit d's candy into an image px pixels square, the
// candy filling all but spriteMargin of it on each side: its shadow, a
// rim darker than its body, the body, a lighter middle raised a
// little, and the shine.
func drawCandy(d int8, px int) *image.NRGBA {
	k := candyOf(d)
	f := fieldOf(k.shape)
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	// The candy's side, in pixels, and its half in its own units.
	side := float64(px) / (1 + 2*spriteMargin)
	half := side / 2
	at := func(c color.NRGBA) [4]float64 {
		return [4]float64{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255}
	}
	shadow := at(faded(rgb(0x1a, 0x05, 0x30), 0.35))
	rim, body, middle := at(darker(k.color, 0.35)), at(k.color), at(lighter(k.color, 0.22))
	shine, glint := at(faded(rgb(0xff, 0xff, 0xff), 0.55)), at(faded(rgb(0xff, 0xff, 0xff), 0.75))
	// layer returns how much of the shape, drawn k times its size and
	// moved down by oy in its units, covers (x, y).
	layer := func(x, y, k, oy float64) float64 {
		dist := f.at(x/k, (y-oy)/k) * k
		return max(0, min(1, 0.5-dist*half))
	}
	c0, c1, r0 := math.Cos(0.5), math.Sin(0.5), 0.065
	for py := range px {
		for qx := range px {
			// The pixel in the candy's units, -1 to 1 across it.
			x := (float64(qx) + 0.5 - float64(px)/2) / half
			y := (float64(py) + 0.5 - float64(px)/2) / half
			var out [4]float64 // premultiplied
			over := func(c [4]float64, cov float64) {
				a := c[3] * cov
				for i := range 3 {
					out[i] = c[i]*a + out[i]*(1-a)
				}
				out[3] = a + out[3]*(1-a)
			}
			over(shadow, layer(x, y, 1, 0.14))
			over(rim, layer(x, y, 1.06, 0))
			over(body, layer(x, y, 1, 0))
			over(middle, layer(x, y, 0.76, -0.08))
			// The shine: a soft streak at the top left, turned, as a
			// rounded box in the candy's 0-to-1 square.
			u, v := (x+1)/2, (y+1)/2
			du, dv := u-0.36, v-0.26
			ru, rv := c0*du-c1*dv+0.36, c1*du+c0*dv+0.26
			ds := roundBox(ru-0.37, rv-0.265, 0.15, 0.065, r0)
			over(shine, max(0, min(1, 0.5-ds*side)))
			dg := math.Hypot(u-0.695, v-0.335) - 0.035
			over(glint, max(0, min(1, 0.5-dg*side)))
			if out[3] <= 0 {
				continue
			}
			img.SetNRGBA(qx, py, color.NRGBA{
				R: uint8(255*out[0]/out[3] + 0.5), G: uint8(255*out[1]/out[3] + 0.5),
				B: uint8(255*out[2]/out[3] + 0.5), A: uint8(255*out[3] + 0.5),
			})
		}
	}
	return img
}

// paintDigit draws d in the middle of r, white over a dark edge.
func paintDigit(p *paint.Painter, d int8, r geom.Rect, alpha float32) {
	s := r.Size().W
	run := shaped(string(rune('0'+d)), s*0.42, true)
	at := geom.Pt(r.Min.X+(s-run.Advance)/2, r.Min.Y+(s-run.Ascent-run.Descent)/2+s*0.03)
	shadow := faded(darker(candyOf(d).color, 0.55), 0.8*alpha)
	run.Paint(p, at.Add(geom.Pt(0, s*0.035)), shadow)
	run.Paint(p, at, faded(rgb(0xff, 0xff, 0xff), alpha))
}
