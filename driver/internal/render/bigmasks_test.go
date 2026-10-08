//go:build linux || windows || darwin

package render

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// stripes is a settled Shape of one-pixel columns, covered and clear by turns, that counts how often it is
// rasterized: drawn stretched, its columns blur.
type stripes struct{ n *int }

func (s stripes) Coverage(w, h int) []byte {
	*s.n++
	b := make([]byte, w*h)
	for i := range b {
		if i%w%2 == 0 {
			b[i] = 255
		}
	}
	return b
}

func (s stripes) Settled() bool { return true }

// sharp reports whether the canvas shows stripes' columns crisp, red then black, along the row y for n pixels
// from x.
func sharp(pix []byte, x, y, n int) bool {
	for i := range n {
		want := byte(0)
		if i%2 == 0 {
			want = 0xff
		}
		if pixelAt(pix, x+i, y)[0] != want {
			return false
		}
	}
	return true
}

// frameOf draws one frame of s at each of rects, red on black, within damage, at the time now, and returns the
// canvas.
func frameOf(rr *Renderer, s paint.Shape, rects []geom.Rect, damage geom.Rect, now time.Time) []byte {
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	for _, r := range rects {
		p.Mask(s, r, color.NRGBA{R: 0xff, A: 0xff})
	}
	rr.clock = func() time.Time { return now }
	// Drawn to the canvas, which a frame after a big one would skip.
	rr.big = false
	rr.Draw(p.Ops(), damage, int(benchSize.W), int(benchSize.H), 1)
	return canvas(rr)
}

// A settled mask too big for the atlas has a texture of its own: it draws pixel for pixel, and the atlas keeps the
// text of every window.
func TestABigSettledMaskDrawsSharp(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	was := r.shared.atlas.epoch
	pix := drawMask(r, stripes{&n}, geom.Rc(10, 5, 500, 590), color.NRGBA{R: 0xff, A: 0xff})
	if !sharp(pix, 10, 300, 500) {
		t.Errorf("a big mask's first columns are %v %v %v, want red, black, red", pixelAt(pix, 10, 300),
			pixelAt(pix, 11, 300), pixelAt(pix, 12, 300))
	}
	if n != 1 {
		t.Errorf("the big mask was rasterized %d times over two frames, want once", n)
	}
	drawMask(r, stripes{&n}, geom.Rc(10, 5, 501, 590), color.NRGBA{R: 0xff, A: 0xff})
	if r.shared.atlas.epoch != was {
		t.Error("two big masks emptied the atlas, and the text of every window with it")
	}
}

// A big mask whose size changes draws its old size stretched, at no cost, and sharp from the frame that finds the
// size held.
func TestABigMaskResizedDrawsSharpOnceItsSizeHolds(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := stripes{&n}
	at := time.Now()
	frameOf(r, s, []geom.Rect{geom.Rc(10, 5, 400, 500)}, paint.Everything, at)
	at = at.Add(16 * time.Millisecond)
	pix := frameOf(r, s, []geom.Rect{geom.Rc(10, 5, 420, 500)}, paint.Everything, at)
	if n != 1 {
		t.Errorf("the mask was rasterized %d times by the frame that resized it, want once, before it", n)
	}
	if sharp(pix, 10, 300, 420) {
		t.Error("the resized mask drew sharp at once, want its old size stretched")
	}
	if want := geom.Rc(10, 5, 420, 500); r.Stretched.Union(want) != r.Stretched || !r.SharpAt.Equal(at.Add(bigMaskSettle)) {
		t.Errorf("the frame asks for %v again in %v, want all of %v in %v", r.Stretched, r.SharpAt.Sub(at), want,
			bigMaskSettle)
	}
	// The driver draws the last frame again, with nothing changed in it.
	pix = frameOf(r, s, []geom.Rect{geom.Rc(10, 5, 420, 500)}, geom.Rect{}, r.SharpAt)
	if n != 2 || !sharp(pix, 10, 300, 420) {
		t.Errorf("the frame after the size held drew the mask sharp: %v, rasterized %d times in all; want sharp, "+
			"twice", sharp(pix, 10, 300, 420), n)
	}
	if !r.Stretched.Empty() {
		t.Errorf("a frame with every mask sharp asks for %v again, want nothing", r.Stretched)
	}
}

// A big mask under a window being resized, a new size every frame, is rasterized every bigMaskStale and stretched
// between, so the frames keep their pace, and each frame asks to be drawn again.
func TestABigMaskResizedEveryFrameIsRasterizedEveryStale(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := stripes{&n}
	at := time.Now()
	for i := range 60 {
		frameOf(r, s, []geom.Rect{geom.Rc(10, 5, float32(300+i), 500)}, paint.Everything, at)
		if i > 0 && n == 1 && r.Stretched.Empty() {
			t.Fatalf("frame %d drew the mask stretched and asks for nothing again", i)
		}
		at = at.Add(16 * time.Millisecond)
	}
	// 60 frames of 16 ms is 960 ms: the first frame rasterizes, then one every 250 ms.
	if n < 4 || n > 5 {
		t.Errorf("a mask resized every frame for 960 ms was rasterized %d times, want 4 or 5", n)
	}
}

// One shape drawn at two sizes in one frame, as a picture large and small, settles with a texture for each.
func TestABigMaskAtTwoSizesKeepsBoth(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	s := stripes{&n}
	both := []geom.Rect{geom.Rc(0, 0, 300, 300), geom.Rc(310, 0, 400, 500)}
	at := time.Now()
	var pix []byte
	for range 6 {
		pix = frameOf(r, s, both, paint.Everything, at)
		at = at.Add(16 * time.Millisecond)
	}
	if n != 2 || !sharp(pix, 0, 100, 300) || !sharp(pix, 310, 100, 400) {
		t.Errorf("one shape at two sizes was rasterized %d times over six frames, sharp %v and %v; want twice, "+
			"both sharp", n, sharp(pix, 0, 100, 300), sharp(pix, 310, 100, 400))
	}
}

// A big mask takes a gradient as a small one does.
func TestABigMaskTakesAGradient(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	p.MaskFill(countedSquare{n: &n, settled: true}, geom.Rc(0, 0, 600, 300), paint.Fill{Gradient: &paint.Gradient{
		From: geom.Pt(0, 0), To: geom.Pt(600, 0),
		Start: color.NRGBA{R: 0xff, A: 0xff}, End: color.NRGBA{B: 0xff, A: 0xff},
	}})
	r.Draw(p.Ops(), paint.Everything, int(benchSize.W), int(benchSize.H), 1)
	pix := canvas(r)
	if l, rt := pixelAt(pix, 5, 100), pixelAt(pix, 594, 100); l[0] < 0xf0 || l[2] > 0x10 || rt[2] < 0xf0 || rt[0] > 0x10 {
		t.Errorf("a big mask's gradient runs %v to %v, want red to blue", l, rt)
	}
}
