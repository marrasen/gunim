package gunim

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

// filled returns a w by h picture of one colour.
func filled(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// arrived returns a part whose frame has come, at at.
func arrived(img *image.RGBA, at image.Point) shotPart {
	p := shotPart{frame: make(chan *image.RGBA, 1), at: at}
	p.frame <- img
	return p
}

func TestAShotPutsAPopupOverItsWindowAndGrowsToHoldIt(t *testing.T) {
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	// The window is 10 by 10 at 100,100; the popup 6 by 6 at 107,96, over its top right corner and past it.
	img, err := compose([]shotPart{arrived(filled(10, 10, red), image.Pt(100, 100)), arrived(filled(6, 6, blue), image.Pt(107, 96))})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 13 || b.Dy() != 14 {
		t.Fatalf("shot is %v, want 13 by 14 to hold both", b)
	}
	at := func(x, y int) color.RGBA { return img.RGBAAt(x, y) }
	if at(0, 4) != red || at(8, 5) != blue || at(12, 0) != blue || at(0, 0) != (color.RGBA{}) {
		t.Fatalf("pixels %v %v %v %v, want the window, the popup over it, the popup past it, and nothing outside",
			at(0, 4), at(8, 5), at(12, 0), at(0, 0))
	}
}

func TestAShotFailsWhenAPopupDrawsNothing(t *testing.T) {
	was := shotWait
	shotWait = 50 * time.Millisecond
	defer func() { shotWait = was }()
	silent := shotPart{frame: make(chan *image.RGBA, 1)}
	_, err := compose([]shotPart{arrived(filled(4, 4, color.RGBA{A: 255}), image.Point{}), silent})
	if err == nil || !strings.Contains(err.Error(), "popup 1 of 1") {
		t.Fatalf("error %v, want the popup that drew nothing named", err)
	}
}
