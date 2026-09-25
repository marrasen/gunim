//go:build linux || windows || darwin

package desktop

import (
	"fmt"
	"image/color"
	"runtime"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/internal/glfw"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// benchSize is the frame the benchmarks draw.
var benchSize = geom.Sz(800, 600)

// hiddenGL makes a hidden window's context current on the calling
// goroutine, locked to its thread, and builds a renderer on it, so a
// benchmark can time the renderer with no swap or vsync in the way.
func hiddenGL(tb testing.TB) (r *renderer, done func()) {
	tb.Helper()
	if display == nil {
		tb.Skip("no display")
	}
	var gw *glfw.Window
	open := func() error {
		if err := glfw.DefaultWindowHints(); err != nil {
			return err
		}
		if err := display.setContextHints(); err != nil {
			return err
		}
		if err := glfw.WindowHint(glfw.Visible, glfw.False); err != nil {
			return err
		}
		share, err := display.shareGroup()
		if err != nil {
			return err
		}
		gw, err = glfw.CreateWindow(int(benchSize.W), int(benchSize.H), "gunim bench", nil, share)
		return err
	}
	if err := display.call(open); err != nil {
		tb.Fatal(err)
	}
	runtime.LockOSThread()
	if err := gw.MakeContextCurrent(); err != nil {
		tb.Fatal(err)
	}
	ctx, err := gl.NewDefaultContext()
	if err != nil {
		tb.Fatal(err)
	}
	if err = ctx.LoadFunctions(); err != nil {
		tb.Fatal(err)
	}
	if r, err = newRenderer(ctx, display.isES, &display.shared); err != nil {
		tb.Fatal(err)
	}
	return r, func() {
		r.release()
		_ = (*glfw.Window)(nil).MakeContextCurrent()
		runtime.UnlockOSThread()
		_ = display.call(gw.Destroy)
	}
}

// galleryOps records a frame like the widgets example's: a header, and
// rows cards each holding a line of text and a button, in a clipping
// scroll layer.
func galleryOps(rows int) []paint.Op {
	var p paint.Painter
	recordGallery(&p, rows, 0)
	return p.Ops()
}

// galleryRuns is the gallery's text, shaped once, as widgets keep it.
type galleryRuns struct {
	title, open text.Run
	labels      []text.Run
}

var runs = func() galleryRuns {
	face := text.Default()
	g := galleryRuns{title: face.Shape("Widgets", 22), open: face.Shape("Open", 14)}
	for i := range 100 {
		g.labels = append(g.labels, face.Shape(fmt.Sprintf("Item %d. A line of text in a card, and a button beside it.", i+1), 14))
	}
	return g
}()

// recordGallery records the gallery into p, with the first button's
// fill warmed by hover, from 0 to 1.
func recordGallery(p *paint.Painter, rows int, hover float32) {
	p.Reset()
	ink := color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff}
	card := color.NRGBA{R: 0x22, G: 0x26, B: 0x30, A: 0xff}
	button := color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff}
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(color.NRGBA{R: 0x16, G: 0x18, B: 0x1e, A: 0xff}))
	title := runs.title
	title.Paint(p, geom.Pt(16, 16), ink)
	endScroll := p.Layer(paint.LayerOpts{Bounds: geom.Rc(16, 60, benchSize.W-32, benchSize.H-76), Opacity: 1, Clip: true, Radius: 4})
	for i := range rows {
		y := 60 + float32(i)*56
		p.RRect(geom.Rc(16, y, benchSize.W-32, 50), 10, paint.Solid(card))
		label := runs.labels[i]
		label.Paint(p, geom.Pt(30, y+16), ink)
		fill := button
		if i == 0 {
			fill.G += uint8(40 * hover)
		}
		p.RRectStroke(geom.Rc(benchSize.W-110, y+8, 80, 34), 8, paint.Solid(fill), paint.Stroke{Width: 1, Color: card})
		open := runs.open
		open.Paint(p, geom.Pt(benchSize.W-92, y+16), ink)
	}
	endScroll()
}

// BenchmarkRenderHover draws the gallery with one button easing into
// its hover colour: every frame differs from the last in that button
// alone, so each redraws that button and copies the canvas to the
// window. The frames are recorded beforehand, so this times drawing
// alone; compare it with BenchmarkRenderGallery, which draws the whole
// frame straight to the window.
func BenchmarkRenderHover(b *testing.B) {
	r, done := hiddenGL(b)
	defer done()
	var still, lit, diff paint.Painter
	recordGallery(&still, 40, 0)
	recordGallery(&lit, 40, 1)
	recordGallery(&diff, 40, 0)
	recordGallery(&diff, 40, 1)
	damage := diff.Damage()
	w, h := int(benchSize.W), int(benchSize.H)
	// Fill the canvas, so every frame after draws in part.
	r.draw(still.Ops(), paint.Everything, w, h, 1)
	r.draw(still.Ops(), damage, w, h, 1)
	r.gl.Finish()
	b.ResetTimer()
	for i := range b.N {
		ops := still.Ops()
		if i%2 == 1 {
			ops = lit.Ops()
		}
		r.draw(ops, damage, w, h, 1)
		r.gl.Finish()
	}
	b.ReportMetric(float64(r.redrawn.Size().W*r.redrawn.Size().H), "px/frame")
}

// BenchmarkRecordGallery times what the engine does for each frame on
// the CPU before the driver draws it: recording the gallery's ops and
// finding what changed since the frame before.
func BenchmarkRecordGallery(b *testing.B) {
	var p paint.Painter
	recordGallery(&p, 40, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		recordGallery(&p, 40, float32(i%2))
		_ = p.Damage()
	}
}

// canvas reads the renderer's canvas.
func canvas(r *renderer) []byte {
	pix := make([]byte, r.fbW*r.fbH*4)
	r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.layers[0].fbo)
	r.gl.ReadPixels(pix, 0, 0, int32(r.fbW), int32(r.fbH), gl.RGBA, gl.UNSIGNED_BYTE)
	return pix
}

func TestPartialRedrawMatchesAFullOne(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	var p paint.Painter
	w, h := int(benchSize.W), int(benchSize.H)
	// The first frame goes straight to the window, and the second
	// fills the canvas, so the third is the first to redraw in part.
	for _, hover := range []float32{0, 1, 0} {
		recordGallery(&p, 40, hover)
		r.draw(p.Ops(), p.Damage(), w, h, 1)
	}
	if s := r.redrawn.Size(); s.W > 100 || s.H > 50 {
		t.Fatalf("a button's hover redrew %v, want about the button", r.redrawn)
	}
	partial := canvas(r)
	r.canvasOK = false
	r.draw(p.Ops(), p.Damage(), w, h, 1)
	if r.redrawn.Size() != benchSize {
		t.Fatalf("a frame after the canvas went stale redrew %v, want all of it", r.redrawn)
	}
	full := canvas(r)
	for i := range full {
		if d := int(full[i]) - int(partial[i]); d > 1 || d < -1 {
			px := i / 4
			t.Fatalf("pixel (%d, %d) is %d redrawn in part and %d in full", px%w, h-1-px/w, partial[i], full[i])
		}
	}
}

func BenchmarkRenderGallery(b *testing.B) {
	r, done := hiddenGL(b)
	defer done()
	ops := galleryOps(40)
	r.draw(ops, paint.Everything, int(benchSize.W), int(benchSize.H), 1) // rasterize the glyphs
	r.gl.Finish()
	r.draws = 0
	b.ResetTimer()
	for range b.N {
		r.draw(ops, paint.Everything, int(benchSize.W), int(benchSize.H), 1)
		r.gl.Finish()
	}
	b.ReportMetric(float64(r.draws)/float64(b.N), "draws/frame")
	b.ReportMetric(float64(len(ops)), "ops/frame")
}

func TestGalleryDrawsInAFewCalls(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	ops := galleryOps(40)
	r.draw(ops, paint.Everything, int(benchSize.W), int(benchSize.H), 1)
	r.draws = 0
	r.draw(ops, paint.Everything, int(benchSize.W), int(benchSize.H), 1)
	// One batch before the layer, one inside it, the layer's
	// composite, and the canvas's copy to the window.
	if r.draws > 4 {
		t.Fatalf("%d ops drew in %d calls, want at most 4", len(ops), r.draws)
	}
}

// TestFrameForASmallerWindowSitsTopLeft draws a frame laid out for a
// window of 400 by 300 into the 800 by 600 buffer, as happens on
// Windows while a window grows faster than frames keep up. The frame
// sits at the top left, and the rest of the buffer takes its
// background colour.
func TestFrameForASmallerWindowSitsTopLeft(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	bg := color.NRGBA{R: 0x20, G: 0x40, B: 0x60, A: 0xff}
	mark := color.NRGBA{R: 0xff, A: 0xff}
	ops := []paint.Op{
		&paint.RRectOp{Rect: geom.Rc(0, 0, 400, 300), Fill: paint.Solid(bg), Transform: paint.Identity},
		&paint.RRectOp{Rect: geom.Rc(10, 10, 20, 20), Fill: paint.Solid(mark), Transform: paint.Identity},
	}
	w, h := int(benchSize.W), int(benchSize.H)
	r.draw(ops, paint.Everything, w, h, 1)
	at := func(x, y int) [3]byte {
		pix := make([]byte, 4)
		r.gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		// GL counts rows from the bottom.
		r.gl.ReadPixels(pix, int32(x), int32(h-1-y), 1, 1, gl.RGBA, gl.UNSIGNED_BYTE)
		return [3]byte{pix[0], pix[1], pix[2]}
	}
	if got := at(20, 20); got != [3]byte{0xff, 0, 0} {
		t.Errorf("the mark at the top left is %v, want red", got)
	}
	for _, p := range [][2]int{{700, 50}, {50, 500}, {700, 500}} {
		if got := at(p[0], p[1]); got != [3]byte{0x20, 0x40, 0x60} {
			t.Errorf("the buffer past the frame at %v is %v, want the background", p, got)
		}
	}
}
