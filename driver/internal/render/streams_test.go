//go:build linux || windows || darwin

package render

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// solid is a stream's picture of one colour, w by h.
func solid(s *paint.Stream, w, h int, c color.NRGBA) *paint.Image {
	pix := make([]byte, 0, 4*w*h)
	for range w * h {
		pix = append(pix, c.R, c.G, c.B, c.A)
	}
	return s.Image(w, h, pix)
}

// A stream's pictures are drawn each in turn from one texture, two of them in one frame too, and one that is larger or
// smaller, and the one before again.
func TestAStreamsPicturesShareATexture(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	var s paint.Stream
	red, green, blue := color.NRGBA{R: 0xff, A: 0xff}, color.NRGBA{G: 0xff, A: 0xff}, color.NRGBA{B: 0xff, A: 0xff}
	a, b, c := solid(&s, 8, 8, red), solid(&s, 8, 8, green), solid(&s, 4, 16, blue)
	at := func(pix []byte, x int, want color.NRGBA, what string) {
		t.Helper()
		if got := pixelAt(pix, x, 116); !near(got, px(want), 2) {
			t.Errorf("%s is %v, want %v", what, got, want)
		}
	}
	for i, img := range []*paint.Image{a, b, c, a} {
		pix := drawn(r, func(p *paint.Painter) { p.Image(img, geom.Rc(100, 100, 32, 32), paint.ImageOpts{Opacity: 1}) })
		at(pix, 116, []color.NRGBA{red, green, blue, red}[i], "a picture alone")
	}
	pix := drawn(r, func(p *paint.Painter) {
		p.Image(a, geom.Rc(100, 100, 32, 32), paint.ImageOpts{Opacity: 1})
		p.Image(b, geom.Rc(200, 100, 32, 32), paint.ImageOpts{Opacity: 1})
		p.Image(c, geom.Rc(300, 100, 32, 32), paint.ImageOpts{Opacity: 1})
		p.Image(a, geom.Rc(400, 100, 32, 32), paint.ImageOpts{Opacity: 1})
	})
	at(pix, 116, red, "the first of four in a frame")
	at(pix, 216, green, "the second of four")
	at(pix, 316, blue, "the third of four")
	at(pix, 416, red, "the first again")
	if len(r.streams) != 1 || len(r.images) != 0 {
		t.Errorf("%d textures of streams and %d of images are kept, want one and none", len(r.streams), len(r.images))
	}
}
