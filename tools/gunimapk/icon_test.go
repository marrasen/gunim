package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plate draws a picture size pixels square: a plate of colour c inset
// by margin, clear round it, with a white dot in its middle.
func plate(size, margin int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := margin; y < size-margin; y++ {
		for x := margin; x < size-margin; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	img.SetNRGBA(size/2, size/2, color.NRGBA{0xff, 0xff, 0xff, 0xff})
	return img
}

func TestTheIconsMarginIsCutAndItsPlateFillsWhatTheLauncherShows(t *testing.T) {
	blue := color.NRGBA{0x20, 0x40, 0xc0, 0xff}
	src := plate(256, 32, blue)
	art := opaqueBounds(src)
	if want := image.Rect(32, 32, 224, 224); art != want {
		t.Fatalf("the art lies in %v, want %v", art, want)
	}
	fg := foreground(src, art)
	if side := layerDP * xxxhdpi; fg.Bounds() != image.Rect(0, 0, side, side) {
		t.Fatalf("the foreground is %v, want %d pixels square", fg.Bounds(), side)
	}
	// The launcher shows the middle 72 of 108 dp: 288 of 432 pixels,
	// from 72 to 360.
	for _, p := range []image.Point{{74, 74}, {358, 358}, {74, 358}, {216, 74}} {
		if c := fg.NRGBAAt(p.X, p.Y); c.A < 0xf0 || c.B < 0xa0 {
			t.Errorf("at %v the foreground is %v, want the blue plate, out to what the launcher shows", p, c)
		}
	}
	for _, p := range []image.Point{{60, 60}, {216, 20}, {420, 216}} {
		if c := fg.NRGBAAt(p.X, p.Y); c.A != 0 {
			t.Errorf("at %v the foreground is %v, want it clear, beyond what the launcher shows", p, c)
		}
	}
	if got := edgeColour(src, art); got != blue {
		t.Errorf("the edge's colour is %v, want the plate's, %v", got, blue)
	}
}

func TestAnIconWithNoPlateGoesOnWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	// A ring of soft pixels, as a picture with no plate under it has.
	for x := 8; x < 56; x++ {
		src.SetNRGBA(x, 8, color.NRGBA{0, 0, 0, 0x80})
		src.SetNRGBA(x, 55, color.NRGBA{0, 0, 0, 0x80})
	}
	if got := edgeColour(src, opaqueBounds(src)); got != (color.NRGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Fatalf("the background is %v, want white", got)
	}
}

func TestAnAdaptiveIconIsWritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, plate(192, 16, color.NRGBA{0x10, 0x20, 0x30, 0xff}))
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	res := filepath.Join(dir, "res")
	if err = adaptiveIcon(res, path, "#ff8000"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mipmap-xxxhdpi/ic_launcher.png", "mipmap-xxxhdpi/ic_launcher_foreground.png"} {
		if _, statErr := os.Stat(filepath.Join(res, name)); statErr != nil {
			t.Error(statErr)
		}
	}
	xml, err := os.ReadFile(filepath.Join(res, "mipmap-anydpi-v26", "ic_launcher.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(xml), "<adaptive-icon") || !strings.Contains(string(xml), "@mipmap/ic_launcher_foreground") {
		t.Errorf("the icon's XML is\n%s\nwant an adaptive icon over the foreground", xml)
	}
	bg, err := os.ReadFile(filepath.Join(res, "values", "ic_launcher_background.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bg), "#ff8000") {
		t.Errorf("the background is\n%s\nwant the colour asked for, #ff8000", bg)
	}
}

func TestAColourIsWrittenAsHex(t *testing.T) {
	if c, err := parseColour("#1e88e5"); err != nil || c != (color.NRGBA{0x1e, 0x88, 0xe5, 0xff}) {
		t.Errorf("#1e88e5 reads as %v, %v", c, err)
	}
	for _, bad := range []string{"1e88e5", "#1e88e", "#gggggg", "blue"} {
		if _, err := parseColour(bad); err == nil {
			t.Errorf("%q reads as a colour", bad)
		}
	}
}
