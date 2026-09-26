package main

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// iconSizes are the sizes the window's icon is drawn at, for the title
// bar, the taskbar and the switcher to pick from.
var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

// icons draws the calculator's icon at each size.
func icons() []image.Image {
	out := make([]image.Image, len(iconSizes))
	var wg sync.WaitGroup
	for i, n := range iconSizes {
		wg.Go(func() { out[i] = drawIcon(n) })
	}
	wg.Wait()
	return out
}

// drawIcon draws the icon n pixels square: a rounded tile shading from
// blue to violet, a white curve across it, as the graph draws, and
// three keys along its foot. Each pixel is sampled sixteen times, and
// four at the large sizes, where a pixel is small beside the edges, so
// the edges are smooth at every size.
func drawIcon(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	ss := 4
	if n > 64 {
		ss = 2
	}
	for py := range n {
		for px := range n {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					// The sample in icon units, 0 to 1 across.
					x := (float64(px) + (float64(sx)+0.5)/float64(ss)) / float64(n)
					y := (float64(py) + (float64(sy)+0.5)/float64(ss)) / float64(n)
					cr, cg, cb, ca := iconAt(x, y, float64(n))
					r += cr * ca
					g += cg * ca
					b += cb * ca
					a += ca
				}
			}
			samples := float64(ss * ss)
			if a == 0 {
				continue
			}
			// Premultiplied, as image.RGBA holds it.
			img.SetRGBA(px, py, color.RGBA{
				R: uint8(math.Round(r / samples * 255)),
				G: uint8(math.Round(g / samples * 255)),
				B: uint8(math.Round(b / samples * 255)),
				A: uint8(math.Round(a / samples * 255)),
			})
		}
	}
	return img
}

// iconAt is the icon's colour at x, y, in icon units, as straight
// red, green, blue and alpha from 0 to 1. n is the icon's size in
// pixels, which keeps the curve thick enough to see when small.
func iconAt(x, y, n float64) (r, g, b, a float64) {
	// The tile: a rounded square, a little in from the edge.
	const inset, radius = 0.04, 0.22
	if roundRectDist(x, y, inset, inset, 1-inset, 1-inset, radius) > 0 {
		return 0, 0, 0, 0
	}
	// Blue at the top left to violet at the bottom right.
	t := (x + y) / 2
	r, g, b = mix(0.25, 0.55, t), mix(0.56, 0.36, t), mix(1.0, 0.98, t)
	a = 1
	// A soft light across the top.
	if y < 0.5 {
		lift := (0.5 - y) * 0.25
		r, g, b = r+(1-r)*lift, g+(1-g)*lift, b+(1-b)*lift
	}
	// white lays white over what is there, by ca.
	white := func(ca float64) {
		r, g, b = r+(1-r)*ca, g+(1-g)*ca, b+(1-b)*ca
	}
	// Three keys along the foot, the last lit.
	for i := range 3 {
		x0 := 0.2 + float64(i)*0.22
		if roundRectDist(x, y, x0, 0.7, x0+0.16, 0.82, 0.04) <= 0 {
			if i == 2 {
				white(0.95)
			} else {
				white(0.35)
			}
		}
	}
	// The curve: a sine across the upper half, its glow under it.
	width := max(0.045, 1.6/n)
	d := sineDist(x, y)
	if d < width*2.2 {
		glow := 1 - d/(width*2.2)
		white(0.25 * glow * glow)
	}
	if d < width/2 {
		white(1)
	}
	return r, g, b, a
}

func mix(a, b, t float64) float64 { return a + (b-a)*t }

// roundRectDist is how far x, y lies outside the rounded rectangle,
// negative inside.
func roundRectDist(x, y, x0, y0, x1, y1, radius float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-radius, (y1-y0)/2-radius
	dx, dy := math.Abs(x-cx)-hx, math.Abs(y-cy)-hy
	out := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	return out + math.Min(math.Max(dx, dy), 0) - radius
}

// The icon's curve: a sine across the upper half, as points joined by
// short pieces, worked out once.
const (
	curveX0, curveSpan, curveMid, curveAmp = 0.16, 0.68, 0.4, 0.16
	curvePieces                            = 48
)

var curvePoints = func() (pts [curvePieces + 1][2]float64) {
	for i := range pts {
		u := curveX0 + curveSpan*float64(i)/curvePieces
		pts[i] = [2]float64{u, curveMid - curveAmp*math.Sin((u-curveX0)/curveSpan*2*math.Pi)}
	}
	return pts
}()

// sineDist is how far x, y lies from the icon's curve, found among the
// pieces of it near x. A point outside the band the curve keeps to is
// far enough to say so at once.
func sineDist(x, y float64) float64 {
	if math.Abs(y-curveMid) > curveAmp+0.15 || x < curveX0-0.15 || x > curveX0+curveSpan+0.15 {
		return math.Inf(1)
	}
	at := int((x - curveX0) / curveSpan * curvePieces)
	best := math.Inf(1)
	for i := max(at-9, 0); i <= min(at+9, curvePieces-1); i++ {
		a, b := curvePoints[i], curvePoints[i+1]
		best = math.Min(best, segmentDist(x, y, a[0], a[1], b[0], b[1]))
	}
	return best
}

// segmentDist is how far x, y lies from the segment a to b.
func segmentDist(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((x-ax)*dx + (y-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(x-(ax+t*dx), y-(ay+t*dy))
}
