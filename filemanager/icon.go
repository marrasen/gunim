package filemanager

import (
	"image"
	"image/color"
	"math"
)

// iconSizes are the sizes the window's icon is drawn at.
var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

// icons draws the icon at each size.
func icons() []image.Image {
	out := make([]image.Image, len(iconSizes))
	for i, n := range iconSizes {
		out[i] = drawIcon(n)
	}
	return out
}

// drawIcon draws a folder n pixels square: a back panel with a tab, and a
// lighter front panel over it, each pixel sampled sixteen times.
func drawIcon(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	back := color.NRGBA{R: 0xd9, G: 0x96, B: 0x2b, A: 0xff}
	front := color.NRGBA{R: 0xf5, G: 0xc2, B: 0x42, A: 0xff}
	const ss = 4
	for py := range n {
		for px := range n {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(n)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(n)
					var c color.NRGBA
					switch {
					case inRound(x, y, 0.06, 0.3, 0.94, 0.88, 0.07):
						c = front
					case inRound(x, y, 0.06, 0.14, 0.94, 0.8, 0.07), inRound(x, y, 0.06, 0.1, 0.46, 0.3, 0.06):
						c = back
					default:
						continue
					}
					r += float64(c.R)
					g += float64(c.G)
					b += float64(c.B)
					a += 255
				}
			}
			if a == 0 {
				continue
			}
			k := float64(ss * ss)
			img.Set(px, py, color.NRGBA{R: uint8(r / (a / 255)), G: uint8(g / (a / 255)), B: uint8(b / (a / 255)), A: uint8(a / k)})
		}
	}
	return img
}

// inRound reports whether x, y lies in the rectangle from x0, y0 to x1, y1
// with corners rounded by rad.
func inRound(x, y, x0, y0, x1, y1, rad float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+rad, math.Min(x, x1-rad))
	cy := math.Max(y0+rad, math.Min(y, y1-rad))
	return math.Hypot(x-cx, y-cy) <= rad
}
