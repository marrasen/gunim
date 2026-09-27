package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// makeDemo fills root with a folder of sample files to browse, and
// returns the folder.
func makeDemo(root string) (string, error) {
	dir := filepath.Join(root, "Sample")
	files := map[string]string{
		"Projects/gunim/README.md":      "# gunim\n\nAn animation-first GUI framework for Go.\n",
		"Projects/gunim/main.go":        "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		"Projects/Release notes.md":     "# Old release notes\n",
		"Projects/notes.txt":            "Ideas for the file manager.\n",
		"Music/playlist.m3u":            "track1.mp3\ntrack2.mp3\n",
		"Documents/budget 2026.csv":     "month,amount\njan,120\nfeb,95\n",
		"Documents/letter.txt":          "Dear reader,\n\nThis is a letter.\n",
		"Documents/report.pdf":          "%PDF-1.4 not really a PDF\n",
		"Archive/old-photos.zip":        "PK not really a zip",
		"Empty folder/":                 "",
		"Shopping list.txt":             "Milk\nBread\nCoffee\nApples\n",
		"Release notes.md":              releaseNotes,
		"config.json":                   "{\n  \"theme\": \"dark\",\n  \"zoom\": 1.25\n}\n",
		".hidden-settings":              "secret=1\n",
		"install.sh":                    "#!/bin/sh\necho installing\n",
		"file2.txt":                     "two\n",
		"file10.txt":                    "ten\n",
		"data.bin":                      string(make([]byte, 3<<20)),
		"Photos/Holiday/.keep":          "",
		"Downloads/setup-installer.exe": "MZ not really a program",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return "", err
		}
	}
	for i, hue := range []float64{0.58, 0.08, 0.33, 0.8} {
		p := filepath.Join(dir, "Photos", fmt.Sprintf("sunset-%d.png", i+1))
		if err := writePicture(p, hue); err != nil {
			return "", err
		}
	}
	if err := writePicture(filepath.Join(dir, "Wallpaper.png"), 0.62); err != nil {
		return "", err
	}
	if err := makePictures(filepath.Join(dir, "Photos")); err != nil {
		return "", err
	}
	// Spread the times so sorting by date means something.
	es, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	now := time.Now()
	for i, e := range es {
		when := now.Add(-time.Duration(i*37) * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, e.Name()), when, when); err != nil {
			return "", err
		}
	}
	return dir, nil
}

const releaseNotes = `# Release notes

## 0.3

- Folders slide in the way you go.
- The preview crossfades between items.
- Copies show their speed.

## 0.2

- Undo for rename, move, copy and trash.
`

// makeBig makes a file of size bytes at path, without writing them.
func makeBig(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Truncate(size), f.Close())
}

// writePicture draws a sunset over the sea in hue, and writes it as a PNG.
func writePicture(path string, hue float64) error {
	const w, h = 480, 320
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			t := float64(y) / h
			var c color.NRGBA
			if t < 0.62 {
				c = hsl(hue+0.1*t, 0.7, 0.35+0.4*t)
			} else {
				c = hsl(hue+0.5, 0.5, 0.25+0.2*(1-t))
			}
			// The sun, low on the horizon.
			dx, dy := float64(x)-w*0.66, float64(y)-h*0.58
			if d := math.Hypot(dx, dy); d < 46 {
				c = hsl(0.12, 0.9, 0.7+0.2*(1-d/46))
			}
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
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
