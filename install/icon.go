package install

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

// square is img scaled to fit size by size pixels, centred, with clear
// pixels round a picture that is not square.
func square(img image.Image, size int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	b := img.Bounds()
	if b.Empty() {
		return out
	}
	w, h := size, size
	if b.Dx() > b.Dy() {
		h = size * b.Dy() / b.Dx()
	} else {
		w = size * b.Dx() / b.Dy()
	}
	r := image.Rect((size-w)/2, (size-h)/2, (size-w)/2+w, (size-h)/2+h)
	if b.Dx() == w && b.Dy() == h {
		draw.Draw(out, r, img, b.Min, draw.Src)
		return out
	}
	xdraw.CatmullRom.Scale(out, r, img, b, draw.Src, nil)
	return out
}

// accentOf is the most vivid colour of img that is neither too dark
// nor too pale to glow with: its pixels weighed by how saturated and
// opaque they are, by hue. ok is false for a picture with no such
// colour, as a grey one.
func accentOf(img image.Image) (c color.NRGBA, ok bool) {
	if img == nil {
		return color.NRGBA{}, false
	}
	small := square(img, 48)
	const bins = 24
	var weight [bins]float64
	var sum [bins][3]float64
	for i := 0; i+3 < len(small.Pix); i += 4 {
		r, g, b, a := small.Pix[i], small.Pix[i+1], small.Pix[i+2], small.Pix[i+3]
		if a < 128 {
			continue
		}
		h, s, v := hsv(r, g, b)
		if s < 0.25 || v < 0.25 {
			continue
		}
		w := s * s * v * float64(a) / 255
		bin := int(h/360*bins) % bins
		weight[bin] += w
		sum[bin][0] += w * float64(r)
		sum[bin][1] += w * float64(g)
		sum[bin][2] += w * float64(b)
	}
	best := -1
	for i := range bins {
		// A hue's neighbours count too, so a gradient across two bins is
		// one colour.
		total := weight[i] + 0.5*(weight[(i+1)%bins]+weight[(i+bins-1)%bins])
		if total > 0 && (best < 0 || total > weight[best]+0.5*(weight[(best+1)%bins]+weight[(best+bins-1)%bins])) {
			best = i
		}
	}
	if best < 0 || weight[best] < 1 {
		return color.NRGBA{}, false
	}
	w := weight[best]
	c = color.NRGBA{R: uint8(sum[best][0] / w), G: uint8(sum[best][1] / w), B: uint8(sum[best][2] / w), A: 0xff}
	// Bright enough to glow on a dark window.
	h, s, v := hsv(c.R, c.G, c.B)
	if v < 0.75 {
		c = fromHSV(h, s, 0.75)
	}
	return c, true
}

// hsv is a colour's hue in degrees and its saturation and value from 0
// to 1.
func hsv(r8, g8, b8 uint8) (h, s, v float64) {
	r, g, b := float64(r8)/255, float64(g8)/255, float64(b8)/255
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	d := mx - mn
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = math.Mod((g-b)/d, 6)
	case mx == g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// fromHSV is the colour of hue h and saturation and value s and v.
func fromHSV(h, s, v float64) color.NRGBA {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.NRGBA{R: uint8((r + m) * 255), G: uint8((g + m) * 255), B: uint8((b + m) * 255), A: 0xff}
}
