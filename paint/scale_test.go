package paint

import (
	"image"
	"image/color"
	"testing"
)

func TestFitSizeKeepsTheShape(t *testing.T) {
	for _, c := range []struct{ w, h, mw, mh, ww, wh int }{
		{400, 200, 100, 100, 100, 50},
		{200, 400, 100, 100, 50, 100},
		{50, 20, 100, 100, 50, 20},
		{3000, 10, 256, 256, 256, 1},
		{0, 10, 100, 100, 0, 0},
	} {
		if w, h := FitSize(c.w, c.h, c.mw, c.mh); w != c.ww || h != c.wh {
			t.Errorf("FitSize(%d, %d, %d, %d) = %d, %d; want %d, %d", c.w, c.h, c.mw, c.mh, w, h, c.ww, c.wh)
		}
	}
}

// stripes returns a picture of one-pixel black and white columns.
func stripes(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if x%2 == 0 {
				m.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				m.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	return m
}

func TestNewImageFitAveragesWhatItShrinks(t *testing.T) {
	m := NewImageFit(stripes(1600, 800), 100, 100)
	if w, h := m.Size(); w != 100 || h != 50 {
		t.Fatalf("the picture shrank to %dx%d, want 100x50", w, h)
	}
	// Every pixel stands for as much white as black: grey, never either.
	for i := 0; i < len(m.Pix()); i += 4 {
		if v := m.Pix()[i]; v < 100 || v > 156 || m.Pix()[i+3] != 255 {
			t.Fatalf("pixel %d is %v, want an opaque grey", i/4, m.Pix()[i:i+4])
		}
	}
}

func TestNewImageFitTakesAnyKindOfPicture(t *testing.T) {
	y := image.NewYCbCr(image.Rect(10, 10, 810, 610), image.YCbCrSubsampleRatio420)
	for i := range y.Y {
		y.Y[i] = 200
	}
	for i := range y.Cb {
		y.Cb[i], y.Cr[i] = 128, 128
	}
	m := NewImageFit(y, 80, 80)
	if w, h := m.Size(); w != 80 || h != 60 {
		t.Fatalf("the picture shrank to %dx%d, want 80x60", w, h)
	}
	if p := m.Pix()[:4]; p[0] < 195 || p[0] > 205 || p[3] != 255 {
		t.Fatalf("the first pixel is %v, want the picture's light grey", p)
	}
}

func TestNewImageFitLeavesASmallPictureAsItIs(t *testing.T) {
	m := NewImageFit(stripes(20, 10), 100, 100)
	if w, h := m.Size(); w != 20 || h != 10 {
		t.Fatalf("a small picture came out %dx%d", w, h)
	}
	if m.Pix()[0] != 255 || m.Pix()[4] != 0 {
		t.Fatal("a small picture's pixels changed")
	}
}
