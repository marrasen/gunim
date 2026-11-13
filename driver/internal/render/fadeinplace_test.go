//go:build linux || windows || darwin

package render

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// fadingList paints a list scrolled by at, in a layer over bounds that
// fades toward its top and bottom as a scroll's does, over a page. The
// rows reach past the layer on every side.
func fadingList(bounds geom.Rect, at float32, fade geom.Insets) []paint.Op {
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(white))
	end := p.Layer(paint.LayerOpts{Bounds: bounds, Opacity: 1, Clip: true, Fade: fade})
	func() {
		defer p.Push(paint.Translate(geom.Pt(0, -at)))()
		p.RRect(geom.Rc(0, -100, 600, 1300), 0, paint.Solid(color.NRGBA{R: 0x30, G: 0x40, B: 0x60, A: 0xff}))
		for k := range 30 {
			y := float32(k) * 37.3
			c := color.NRGBA{R: uint8(40 * k), G: uint8(255 - 7*k), B: 120, A: 0xff}
			p.RRect(geom.Rc(30, 40+y, 480, 70+y), 8, paint.Solid(c))
			text.Default().Shape("Row of a list that scrolls", 15).Paint(&p, geom.Pt(40, 50+y), black)
		}
	}()
	end()
	return p.Ops()
}

// drawCanvas draws ops whole, at scale, and returns the canvas's pixels.
func drawCanvas(r *Renderer, ops []paint.Op, scale float32) []byte {
	w, h := int(benchSize.W*scale), int(benchSize.H*scale)
	r.canvasOK, r.direct, r.big = false, false, false
	r.Draw(ops, paint.Everything, w, h, scale)
	// A whole frame after a whole frame goes straight to the window: the
	// second, of a pixel, leaves the canvas as the first drew it.
	r.Draw(ops, geom.Rc(0, 0, 0.5, 0.5), w, h, scale)
	return canvas(r)
}

// A layer that fades only toward its top and bottom draws in place, with
// what lay under its fading bands laid back over them: the same picture as
// the layer drawn whole, at a fractional scale, scrolled to a fractional
// place, with fades of fractional lengths. A layer whose bounds are off
// whole device pixels is drawn whole.
func TestAFadeInPlaceDrawsAsTheLayerWhole(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	r.SetText(text.Rendering{Smoothing: text.Greyscale, Gamma: 1.8}, false)
	wasDebug := framesDebug
	framesDebug = true // for Stats.FadesInPlace
	defer func() { noFadeInPlace, framesDebug = false, wasDebug }()
	onPixels := geom.Rc(20, 40, 500, 420)
	// At 1.75 a logical pixel is 1.75 device ones: 40 and 420 are on
	// device pixels (70 and 735), 20 and 500 too (35 and 875).
	offPixels := geom.Rc(20.3, 40.6, 500.2, 420.7)
	for _, c := range []struct {
		name    string
		bounds  geom.Rect
		at      float32
		fade    geom.Insets
		scale   float32
		inPlace bool
	}{
		{"both edges", onPixels, 13.7, geom.Insets{Top: 17.3, Bottom: 23.9}, 1, true},
		{"both edges at 1.75", onPixels, 101.35, geom.Insets{Top: 17.3, Bottom: 23.9}, 1.75, true},
		{"the bottom edge only, at the top", onPixels, 0, geom.Insets{Bottom: 24}, 1.25, true},
		{"the top edge only, at the end", onPixels, 760.4, geom.Insets{Top: 9.6}, 1, true},
		{"both edges, off pixels across", geom.Rc(20.3, 40.6, 500.2, 420.7), 13.7, geom.Insets{Top: 17.3, Bottom: 23.9}, 1, false},
		{"the top edge only, the bottom off a pixel", offPixels, 760.4, geom.Insets{Top: 9.6}, 1, false},
		{"both edges, off pixels down", geom.Rc(20, 40.6, 500, 420.7), 13.7, geom.Insets{Top: 17.3, Bottom: 23.9}, 1, false},
	} {
		ops := fadingList(c.bounds, c.at, c.fade)
		noFadeInPlace = true
		r.draws = 0
		whole := drawCanvas(r, ops, c.scale)
		wholeDraws := r.draws
		noFadeInPlace = false
		r.draws = 0
		r.Stats.FadesInPlace = 0
		inPlace := drawCanvas(r, ops, c.scale)
		if got := r.Stats.FadesInPlace > 0; got != c.inPlace {
			t.Errorf("%s: in place %v, want %v", c.name, got, c.inPlace)
		}
		worst, off := 0, 0
		for i := range whole {
			d := int(whole[i]) - int(inPlace[i])
			if d < 0 {
				d = -d
			}
			worst = max(worst, d)
			if d > 2 {
				off++
			}
		}
		if off > 0 {
			t.Errorf("%s: %d channels differ by more than 2 of 255, at most by %d", c.name, off, worst)
		}
		t.Logf("%s: at most %d of 255 apart; %d draw calls whole, %d in place", c.name, worst, wholeDraws, r.draws)
	}
}
