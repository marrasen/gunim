package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"sync"
)

// iconSizes are the sizes the window's icon is drawn at, for the title
// bar, the taskbar and the switcher.
var iconSizes = []int{16, 24, 32, 48, 64, 128, 256}

// icons returns the game's icon at each of iconSizes.
func icons() []image.Image {
	out := make([]image.Image, len(iconSizes))
	var wg sync.WaitGroup
	for i, n := range iconSizes {
		wg.Go(func() { out[i] = drawIcon(n) })
	}
	wg.Wait()
	return out
}

// writeIcon writes the icon n pixels square to a PNG file, as the
// launcher's icon on a phone.
func writeIcon(path string, n int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, drawIcon(n)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// The icon's layout, in icon units, 0 to 1 across: the tile's inset and
// corner, and the glass panel's.
const (
	iconInset   = 0.04
	iconRadius  = 0.22
	panelInset  = 0.15
	panelRadius = 0.12
	slotPad     = 0.035
	slotGap     = 0.03
)

// iconSlot returns the side of a slot in the icon's panel, and where
// slot i, from the top left, starts.
func iconSlot(i int) (side, x, y float64) {
	w := 1 - 2*panelInset
	side = (w - 2*slotPad - 2*slotGap) / 3
	at := func(k int) float64 { return panelInset + slotPad + float64(k)*(side+slotGap) }
	return side, at(i % 3), at(i / 3)
}

// drawIcon draws the icon n pixels square: a board of the nine candies
// three by three, on a glass panel over the game's candy dusk. Small,
// where nine would blur, a gold star candy stands alone.
func drawIcon(n int) *image.NRGBA {
	small := n < 48
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	ss := 4
	if n > 64 {
		ss = 2
	}
	for py := range n {
		for px := range n {
			var r, g, b, a float64
			for sy := range ss {
				for sx := range ss {
					x := (float64(px) + (float64(sx)+0.5)/float64(ss)) / float64(n)
					y := (float64(py) + (float64(sy)+0.5)/float64(ss)) / float64(n)
					cr, cg, cb, ca := iconAt(x, y, small)
					r += cr * ca
					g += cg * ca
					b += cb * ca
					a += ca
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(math.Round(r / a * 255)), G: uint8(math.Round(g / a * 255)),
				B: uint8(math.Round(b / a * 255)), A: uint8(math.Round(a / float64(ss*ss) * 255)),
			})
		}
	}
	// The candies, from the game's own sprites, each filling all but an
	// edge of its slot, as on the board.
	if small {
		stamp(img, 3, 0.5, 0.5, 0.78)
		return img
	}
	for i := range 9 {
		side, x, y := iconSlot(i)
		stamp(img, int8(i+1), x+side/2, y+side/2, side*0.9)
	}
	return img
}

// stamp draws digit d's candy side across, centred on x, y, in icon
// units, over img.
func stamp(img *image.NRGBA, d int8, x, y, side float64) {
	n := float64(img.Bounds().Dx())
	px := int(math.Round(side * n * (1 + 2*spriteMargin)))
	at := image.Pt(int(math.Round(x*n))-px/2, int(math.Round(y*n))-px/2)
	draw.Draw(img, image.Rectangle{Min: at, Max: at.Add(image.Pt(px, px))}, drawCandy(d, px), image.Point{}, draw.Over)
}

// iconAt is the icon's colour at x, y, in icon units, 0 to 1 across,
// under the candies, as straight red, green, blue and alpha from 0 to 1.
func iconAt(x, y float64, small bool) (r, g, b, a float64) {
	if roundBox(x-0.5, y-0.5, 0.5-iconInset, 0.5-iconInset, iconRadius) > 0 {
		return 0, 0, 0, 0
	}
	// The dusk, deep violet at the top left to pink at the bottom
	// right, as the game's sky.
	t := max(0, min(1, (x*0.3+y)/1.3))
	r, g, b = mix(0x3b, 0xe8, t), mix(0x12, 0x3e, t), mix(0x8f, 0x9c, t)
	a = 1
	over := func(c [3]float64, ca float64) {
		r, g, b = r+(c[0]-r)*ca, g+(c[1]-g)*ca, b+(c[2]-b)*ca
	}
	glass := [3]float64{1, 1, 1}
	// A warm glow low down.
	if d := math.Hypot((x-0.5)/1.2, y-0.95); d < 0.6 {
		k := 1 - d/0.6
		over([3]float64{1, 0.7, 0.28}, 0.45*k*k)
	}
	if small {
		return r, g, b, a
	}
	// The glass panel, its rim, and its slots.
	pd := roundBox(x-0.5, y-0.5, 0.5-panelInset, 0.5-panelInset, panelRadius)
	if pd <= 0 {
		over(glass, 0.16)
		if pd > -0.008 {
			over(glass, 0.35)
		}
		for i := range 9 {
			side, sx, sy := iconSlot(i)
			if roundBox(x-sx-side/2, y-sy-side/2, side/2, side/2, side*0.22) <= 0 {
				over(glass, 0.12)
			}
		}
	}
	return r, g, b, a
}

// mix is the channel from a to b, by t, as 0 to 1.
func mix(a, b uint8, t float64) float64 {
	return (float64(a) + (float64(b)-float64(a))*t) / 255
}
