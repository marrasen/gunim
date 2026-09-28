//go:build linux || windows || darwin

package desktop

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// countedSquare is a Shape that covers every pixel and counts how often it is rasterized.
type countedSquare struct {
	n       *int
	settled bool
}

func (s countedSquare) Coverage(w, h int) []byte {
	*s.n++
	b := make([]byte, w*h)
	for i := range b {
		b[i] = 255
	}
	return b
}

func (s countedSquare) Settled() bool { return s.settled }

// drawMask draws s into r on black and returns the canvas, its rows from the bottom.
func drawMask(rr *renderer, s paint.Shape, r geom.Rect, c color.NRGBA) []byte {
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	p.Mask(s, r, c)
	rr.canvasOK, rr.direct = false, false
	w, h := int(benchSize.W), int(benchSize.H)
	rr.draw(p.Ops(), paint.Everything, w, h, 1)
	rr.draw(p.Ops(), geom.Rc(0, 0, 1, 1), w, h, 1)
	return canvas(rr)
}

// pixelAt returns the canvas's pixel at x, y from the top left.
func pixelAt(pix []byte, x, y int) [4]byte {
	w, h := int(benchSize.W), int(benchSize.H)
	i := ((h-1-y)*w + x) * 4
	return [4]byte(pix[i : i+4])
}

func TestASettledMaskIsRasterizedOnceWhateverItsColour(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := countedSquare{n: &n, settled: true}
	red := color.NRGBA{R: 0xff, A: 0xff}
	pix := drawMask(r, s, geom.Rc(10.3, 20, 16, 16), red)
	if got := pixelAt(pix, 10, 20); got != [4]byte{0xff, 0, 0, 0xff} {
		t.Errorf("the mask's first pixel is %v, want red", got)
	}
	if got := pixelAt(pix, 25, 35); got != [4]byte{0xff, 0, 0, 0xff} {
		t.Errorf("the mask's last pixel is %v, want red", got)
	}
	if got := pixelAt(pix, 9, 20); got != [4]byte{0, 0, 0, 0xff} {
		t.Errorf("the pixel left of the mask is %v, want it untouched, the mask snapped to the pixel", got)
	}
	if got := pixelAt(pix, 26, 20); got != [4]byte{0, 0, 0, 0xff} {
		t.Errorf("the pixel right of the mask is %v, want it untouched", got)
	}
	pix = drawMask(r, s, geom.Rc(10, 20, 16, 16), color.NRGBA{B: 0xff, A: 0xff})
	if got := pixelAt(pix, 12, 22); got != [4]byte{0, 0, 0xff, 0xff} {
		t.Errorf("the recoloured mask is %v, want blue", got)
	}
	if n != 1 {
		t.Errorf("the settled mask was rasterized %d times over four frames and two colours, want once", n)
	}
	drawMask(r, s, geom.Rc(10, 20, 24, 24), red)
	if n != 2 {
		t.Errorf("the mask at a new size was rasterized %d times in all, want twice", n)
	}
}

func TestAMaskStillChangingIsRasterizedEveryFrame(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := countedSquare{n: &n}
	was := r.scratches
	pix := drawMask(r, s, geom.Rc(10, 20, 300, 16), color.NRGBA{G: 0xff, A: 0xff})
	if n != 2 || r.scratches-was != 2 {
		t.Errorf("an unsettled mask was rasterized %d times and put in the scratch strip %d times over two frames, "+
			"want twice each", n, r.scratches-was)
	}
	// Too wide for the strip, it is drawn smaller and stretched to its box.
	if got := pixelAt(pix, 305, 28); got[1] < 0xf0 {
		t.Errorf("the far end of the wide mask is %v, want green", got)
	}
}

// A settled mask too big for the atlas is kept smaller and stretched to its box, soft only at its very edge, and
// the atlas keeps the rest.
func TestABigSettledMaskDrawsStretched(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := countedSquare{n: &n, settled: true}
	was := r.shared.atlas.epoch
	pix := drawMask(r, s, geom.Rc(10, 5, 500, 590), color.NRGBA{R: 0xff, A: 0xff})
	for _, p := range [][2]int{{13, 8}, {506, 591}, {250, 300}} {
		if got := pixelAt(pix, p[0], p[1]); got[0] < 0xf0 {
			t.Errorf("the big mask's pixel at %v is %v, want red", p, got)
		}
	}
	drawMask(r, countedSquare{n: &n, settled: true}, geom.Rc(10, 5, 501, 590), color.NRGBA{R: 0xff, A: 0xff})
	if r.shared.atlas.epoch != was {
		t.Error("two big masks emptied the atlas, and the text of every window with it")
	}
}
