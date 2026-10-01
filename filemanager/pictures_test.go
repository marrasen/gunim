package filemanager

import (
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/bmp"
)

// The tests draw their pictures as the example's demo folder does.

// picture draws a picture w by h pixels, each from where it lies, from 0 to 1 across and down.
func picture(w, h int, at func(x, y float64) color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, at(float64(x)/float64(w), float64(y)/float64(h)))
		}
	}
	return img
}

// rings is circles of colour spreading from a point off centre.
func rings(x, y float64) color.NRGBA {
	d := math.Hypot(x-0.4, y-0.55)
	return hsl(d*2.2, 0.75, 0.45+0.15*math.Sin(d*40))
}

// writeImage writes img to path in the format its extension names.
func writeImage(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(encodeFor(path, f, img), f.Close())
}

func encodeFor(path string, w io.Writer, img image.Image) error {
	switch filepath.Ext(path) {
	case ".jpg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 88})
	case ".gif":
		return gif.Encode(w, img, nil)
	case ".bmp":
		return bmp.Encode(w, img)
	}
	return png.Encode(w, img)
}

// hsl turns a hue, saturation and lightness, each from 0 to 1, into a
// colour.
func hsl(h, s, l float64) color.NRGBA {
	h -= math.Floor(h)
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	ch := func(t float64) uint8 {
		t -= math.Floor(t)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(min(max(v, 0), 1) * 255))
	}
	return color.NRGBA{R: ch(h + 1.0/3), G: ch(h), B: ch(h - 1.0/3), A: 255}
}
