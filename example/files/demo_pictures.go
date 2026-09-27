package main

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

// aurora is green and violet curtains over a night sky.
func aurora(x, y float64) color.NRGBA {
	wave := 0.35 + 0.12*math.Sin(x*9) + 0.05*math.Sin(x*23+1)
	d := (y - wave) * 7
	glow := math.Exp(-d * d)
	c := hsl(0.62, 0.6, 0.08+0.1*y)
	if glow > 0.02 {
		g := hsl(0.36+0.3*y, 0.8, 0.2+0.45*glow)
		c = mix(c, g, glow)
	}
	return c
}

// rings is circles of colour spreading from a point off centre.
func rings(x, y float64) color.NRGBA {
	d := math.Hypot(x-0.4, y-0.55)
	return hsl(d*2.2, 0.75, 0.45+0.15*math.Sin(d*40))
}

// fractal is the Mandelbrot set, coloured by how soon each point escapes.
func fractal(x, y float64) color.NRGBA {
	cr, ci := -2.1+x*2.8, -1.2+y*2.4
	zr, zi := 0.0, 0.0
	for i := range 80 {
		zr, zi = zr*zr-zi*zi+cr, 2*zr*zi+ci
		if zr*zr+zi*zi > 4 {
			return hsl(0.55+float64(i)/60, 0.8, 0.25+0.4*float64(i)/80)
		}
	}
	return color.NRGBA{R: 0x10, G: 0x10, B: 0x20, A: 0xff}
}

// dunes is a desert under a pale sky, in a tall picture.
func dunes(x, y float64) color.NRGBA {
	ridge := 0.55 + 0.08*math.Sin(x*7) + 0.04*math.Sin(x*17+2)
	if y < ridge {
		return hsl(0.58, 0.5, 0.85-0.3*y)
	}
	return hsl(0.08, 0.6, 0.55-0.25*(y-ridge)+0.08*math.Sin(y*60+x*9))
}

// tiles is a checker of hues, a picture for the GIF palette.
func tiles(x, y float64) color.NRGBA {
	i, j := int(x*8), int(y*8)
	return hsl(float64(i+j)/16, 0.7, 0.35+0.25*float64((i+j)%2))
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*t) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: 255}
}

// makePictures fills dir with pictures of several shapes and formats, a large one among them, and a JPEG cut short,
// whose tile shows why it cannot be read.
func makePictures(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	type pic struct {
		name string
		w, h int
		at   func(x, y float64) color.NRGBA
	}
	for _, p := range []pic{
		{"Aurora.jpg", 1600, 1000, aurora},
		{"Rings.png", 720, 720, rings},
		{"Mandelbrot.png", 900, 600, fractal},
		{"Dunes.jpg", 640, 960, dunes},
		{"Tiles.gif", 320, 320, tiles},
		{"Rings small.bmp", 240, 160, rings},
		{"Panorama.jpg", 4800, 1600, aurora},
	} {
		if err := writeImage(filepath.Join(dir, p.name), picture(p.w, p.h, p.at)); err != nil {
			return err
		}
	}
	return writeCut(filepath.Join(dir, "Damaged.jpg"), picture(400, 300, rings))
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

// writeCut writes img as a JPEG with its second half missing.
func writeCut(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if werr := errors.Join(jpeg.Encode(f, img, nil), f.Close()); werr != nil {
		return werr
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.Truncate(path, info.Size()/2)
}
