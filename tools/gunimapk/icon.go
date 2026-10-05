package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

// An adaptive icon is two layers of 108 by 108 dp, a background and a
// foreground over it, which the launcher cuts to its own shape: a
// circle, a squircle, a rounded square. The shape shows the middle 72
// dp, and may move the layers a little within the rest as the icon is
// touched.
const (
	layerDP = 108
	shownDP = 72
	// xxxhdpi is the pixels per dp of the densest screens, which the
	// layers are drawn for; Android scales them down for the rest.
	xxxhdpi = 4
)

// adaptiveIcon writes the launcher's icon into res, the resources
// directory, from the PNG at path: the PNG as it is, for a launcher
// that takes a plain icon, and the two layers of an adaptive one. The
// foreground is the picture with the clear margin round it cut off,
// scaled to fill the part of the icon the launcher shows. The
// background is bg, or where bg is "" the colour of the picture's own
// edge, so a picture drawn on a plate of colour runs out to the
// launcher's shape.
func adaptiveIcon(res, path, bg string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("icon: %w", err)
	}
	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("icon: %s: %w", path, err)
	}
	art := opaqueBounds(src)
	if art.Empty() {
		return fmt.Errorf("icon: %s is clear all over", path)
	}
	var under color.NRGBA
	if bg != "" {
		if under, err = parseColour(bg); err != nil {
			return fmt.Errorf("icon background: %w", err)
		}
	} else {
		under = edgeColour(src, art)
	}

	mipmap := filepath.Join(res, "mipmap-xxxhdpi")
	anydpi := filepath.Join(res, "mipmap-anydpi-v26")
	values := filepath.Join(res, "values")
	for _, d := range []string{mipmap, anydpi, values} {
		if mkErr := os.MkdirAll(d, 0o755); mkErr != nil {
			return mkErr
		}
	}
	plain, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("icon: %w", err)
	}
	if err := os.WriteFile(filepath.Join(mipmap, "ic_launcher.png"), plain, 0o644); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(mipmap, "ic_launcher_foreground.png"), foreground(src, art)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(values, "ic_launcher_background.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<resources>
	<color name="ic_launcher_background">`+hexColour(under)+`</color>
</resources>
`), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(anydpi, "ic_launcher.xml"), []byte(`<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
	<background android:drawable="@color/ic_launcher_background"/>
	<foreground android:drawable="@mipmap/ic_launcher_foreground"/>
</adaptive-icon>
`), 0o644)
}

// foreground draws the part art of src on the foreground layer, scaled
// to fill the middle shownDP of it, and centred along its shorter side.
func foreground(src image.Image, art image.Rectangle) *image.NRGBA {
	side := layerDP * xxxhdpi
	shown := shownDP * xxxhdpi
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	w, h := art.Dx(), art.Dy()
	fw, fh := shown, shown
	if w > h {
		fh = shown * h / w
	} else if h > w {
		fw = shown * w / h
	}
	at := image.Pt((side-fw)/2, (side-fh)/2)
	draw.CatmullRom.Scale(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(fw, fh))}, src, art, draw.Src, nil)
	return dst
}

// opaqueBounds returns the smallest rectangle that holds every pixel
// of img with any opacity.
func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	r := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

// edgeColour returns the colour along the edge of art in img, the mean
// of the pixels on its outermost ring that are all but opaque, or white
// where there are none, as for a picture with no plate under it.
func edgeColour(img image.Image, art image.Rectangle) color.NRGBA {
	var r, g, b, n uint64
	add := func(x, y int) {
		c, _ := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
		if c.A < 0xf0 {
			return
		}
		r, g, b, n = r+uint64(c.R), g+uint64(c.G), b+uint64(c.B), n+1
	}
	// Two pixels in from the edge, past the soft rim a scaled picture
	// leaves.
	in := art.Inset(min(2, art.Dx()/4, art.Dy()/4))
	for x := in.Min.X; x < in.Max.X; x++ {
		add(x, in.Min.Y)
		add(x, in.Max.Y-1)
	}
	for y := in.Min.Y + 1; y < in.Max.Y-1; y++ {
		add(in.Min.X, y)
		add(in.Max.X-1, y)
	}
	if n == 0 {
		return color.NRGBA{0xff, 0xff, 0xff, 0xff}
	}
	return color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), 0xff}
}

// parseColour reads a colour written as #rrggbb.
func parseColour(s string) (color.NRGBA, error) {
	h, ok := strings.CutPrefix(s, "#")
	if !ok || len(h) != 6 {
		return color.NRGBA{}, fmt.Errorf("%q is no colour; write it as #rrggbb", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, errors.New("a colour is written as #rrggbb, in hex")
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, nil
}

func hexColour(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func writePNG(path string, img image.Image) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	return png.Encode(f, img)
}
