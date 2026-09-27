package paint

import (
	"image"
	stddraw "image/draw"

	"golang.org/x/image/draw"
)

// FitSize returns the size of a picture w by h pixels scaled down to fit within maxW by maxH, keeping its shape. A
// picture that fits already keeps its size.
func FitSize(w, h, maxW, maxH int) (fw, fh int) {
	if w <= 0 || h <= 0 || maxW <= 0 || maxH <= 0 {
		return 0, 0
	}
	if w <= maxW && h <= maxH {
		return w, h
	}
	s := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	return max(1, int(float64(w)*s+0.5)), max(1, int(float64(h)*s+0.5))
}

// NewImageFit makes an Image of m scaled down to fit within maxW by maxH pixels, keeping its shape. It averages
// whole blocks of pixels first and finishes with a Catmull-Rom filter, so a large photo shrinks quickly and without
// shimmering edges.
func NewImageFit(m image.Image, maxW, maxH int) *Image {
	b := m.Bounds()
	w, h := FitSize(b.Dx(), b.Dy(), maxW, maxH)
	if w == b.Dx() && h == b.Dy() || w == 0 {
		return NewImage(m)
	}
	// Blocks of k by k pixels leave at least twice the size wanted for the filter.
	k := max(1, min(b.Dx()/(2*w), b.Dy()/(2*h)))
	src := m
	if k > 1 {
		src = boxShrink(m, k)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return &Image{w: w, h: h, pix: dst.Pix}
}

// boxShrink averages each k by k block of m into one pixel, reading m a band of k rows at a time.
func boxShrink(m image.Image, k int) *image.RGBA {
	b := m.Bounds()
	w, h := b.Dx()/k, b.Dy()/k
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	band := image.NewRGBA(image.Rect(0, 0, b.Dx(), k))
	sums := make([]uint32, 4*w)
	n := uint32(k * k)
	for y := range h {
		stddraw.Draw(band, band.Bounds(), m, image.Pt(b.Min.X, b.Min.Y+y*k), stddraw.Src)
		clear(sums)
		for row := range k {
			line := band.Pix[row*band.Stride:]
			for x := range w {
				px := line[4*x*k : 4*(x+1)*k]
				s := sums[4*x : 4*x+4]
				for i := 0; i < len(px); i += 4 {
					s[0] += uint32(px[i])
					s[1] += uint32(px[i+1])
					s[2] += uint32(px[i+2])
					s[3] += uint32(px[i+3])
				}
			}
		}
		dst := out.Pix[y*out.Stride:]
		for i, v := range sums {
			dst[i] = uint8((v + n/2) / n)
		}
	}
	return out
}
