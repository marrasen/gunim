//go:build linux || windows || darwin

package render

import (
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// drawnTilted draws a card 200 by 200 at 300, 200 on black, its left
// half red and its right half blue, in a layer tilted by t, and returns
// the canvas.
func drawnTilted(r *Renderer, t paint.Tilt) []byte {
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	end := p.Layer(paint.LayerOpts{Bounds: geom.Rc(300, 200, 200, 200), Opacity: 1, Tilt: t})
	p.RRect(geom.Rc(300, 200, 100, 200), 0, paint.Solid(red))
	p.RRect(geom.Rc(400, 200, 100, 200), 0, paint.Solid(blue))
	end()
	r.canvasOK, r.direct = false, false
	w, h := int(benchSize.W), int(benchSize.H)
	r.Draw(p.Ops(), paint.Everything, w, h, 1)
	r.Draw(p.Ops(), geom.Rc(0, 0, 1, 1), w, h, 1)
	return canvas(r)
}

// height returns how many pixels of the column at x are lit.
func height(pix []byte, x int) int {
	n := 0
	for y := range int(benchSize.H) {
		if pixelAt(pix, x, y) != px(black) {
			n++
		}
	}
	return n
}

func TestALayerTurnedAwayShrinksInPerspective(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// The right side turns away, by 60 degrees.
	tilt := paint.Tilt{Y: math.Pi / 3, Distance: 600}
	pix := drawnTilted(r, tilt)
	h := tilt.Homography(geom.Pt(400, 300))
	left, _ := h.Apply(geom.Pt(300, 300))
	right, _ := h.Apply(geom.Pt(500, 300))
	near, far := height(pix, int(left.X)+3), height(pix, int(right.X)-3)
	if near <= far+20 {
		t.Errorf("the near edge is %d pixels tall and the far one %d, want the far one shorter", near, far)
	}
	if near <= 200 {
		t.Errorf("the near edge is %d pixels tall, want it taller than the flat card's 200", near)
	}
	if got := pixelAt(pix, int(left.X)+3, 300); got != px(red) {
		t.Errorf("at the near edge the card is %v, want its left half's red", got)
	}
	if got := pixelAt(pix, int(left.X)-3, 300); got != px(black) {
		t.Errorf("past the near edge the canvas is %v, want it untouched", got)
	}
	if got := pixelAt(pix, int(right.X)-3, 300); got != px(blue) {
		t.Errorf("at the far edge the card is %v, want its right half's blue", got)
	}
}

func TestAHalfTurnShowsTheBackMirroredOrNothing(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawnTilted(r, paint.Tilt{Y: math.Pi})
	if got := pixelAt(pix, 320, 300); !near(got, px(blue), 2) {
		t.Errorf("turned round, the card's left is %v, want the blue from its right", got)
	}
	if got := pixelAt(pix, 480, 300); !near(got, px(red), 2) {
		t.Errorf("turned round, the card's right is %v, want the red from its left", got)
	}
	pix = drawnTilted(r, paint.Tilt{Y: math.Pi * 0.6, OneSided: true})
	for x := 250; x < 550; x += 10 {
		if got := pixelAt(pix, x, 300); got != px(black) {
			t.Fatalf("a one-sided card showing its back lit %v at %d, 300, want nothing drawn", got, x)
		}
	}
}
