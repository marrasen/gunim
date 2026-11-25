//go:build js && wasm

package web

import (
	"image/color"
	"math"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// fromBrowser returns the face of the one glyph a run has, which must be the browser's.
func fromBrowser(t *testing.T, s string, glyphs []paint.Glyph) *text.Face {
	t.Helper()
	if len(glyphs) != 1 {
		t.Fatalf("%q is %d glyphs, want one", s, len(glyphs))
	}
	f, ok := text.Lookup(glyphs[0].Face)
	if !ok || !f.FromBrowser() || glyphs[0].ID == 0 {
		t.Fatalf("%q is not the browser's: %+v", s, glyphs[0])
	}
	return f
}

// hasColorEmoji reports whether the browser here draws emoji in colour.
func hasColorEmoji() bool {
	return js.Global().Call("eval", `(() => {
		const x = document.createElement("canvas").getContext("2d");
		x.font = "32px sans-serif";
		x.fillStyle = "#fff";
		x.fillText("\u{1F600}\uFE0F", 0, 40);
		const p = x.getImageData(0, 0, 60, 60).data;
		for (let i = 0; i < p.length; i += 4) if (p[i + 3] > 128 && Math.min(p[i], p[i + 1], p[i + 2]) < 240) return true;
		return false;
	})()`).Bool()
}

// What no face built into the program has, the browser draws with its own fonts, one glyph for each character as its
// reader sees it. What a face has stays the face's.
func TestTheBrowserDrawsWhatNoFaceHas(t *testing.T) {
	face := text.Default()
	for _, s := range []string{"中", "한", "⇒", "א", "ไ", "x\u0302", "\U0001f468\u200d\U0001f469\u200d\U0001f467", "\U0001f1f8\U0001f1ea", "\U0001f44d\U0001f3fd"} {
		run := face.Shape(s, 20)
		f := fromBrowser(t, s, run.Glyphs)
		var m text.Mask
		if id := run.Glyphs[0].ID; f.IsColor(id) {
			m = f.RasterizeColor(id, 20)
		} else {
			m = f.Rasterize(id, 20, 0, text.Raster{})
		}
		if m.W == 0 || m.H == 0 || len(m.Pix) != m.W*m.H*map[bool]int{false: 1, true: 4}[m.Color] {
			t.Errorf("%q drew %dx%d pixels in %d bytes", s, m.W, m.H, len(m.Pix))
		}
		if run.Advance <= 0 || run.Advance > 60 || float32(m.W) > run.Advance+6 {
			t.Errorf("%q is %v wide, and its picture %d", s, run.Advance, m.W)
		}
	}
	for _, s := range []string{"a", "é", "e\u0301", "→", "▶", "⚙", "Ж", "\u00a0", "\u200b"} {
		for _, g := range face.Shape(s, 20).Glyphs {
			if f, _ := text.Lookup(g.Face); f.FromBrowser() {
				t.Errorf("%q is the browser's", s)
			}
		}
	}
}

// Emoji that stand side by side are each one glyph, a family joined by joiners and a flag of two letters too.
func TestEmojiInARowAreEachOneGlyph(t *testing.T) {
	s := "\U0001f600\U0001f44d\U0001f3fd\U0001f468\u200d\U0001f469\u200d\U0001f467\U0001f1f8\U0001f1ea\U0001f1e9\U0001f1f0"
	run := text.Default().Shape(s, 20)
	if len(run.Glyphs) != 5 {
		t.Fatalf("five emoji are %d glyphs", len(run.Glyphs))
	}
	if !hasColorEmoji() {
		t.Fatal("the browser has no emoji in colour: install one, such as fonts-noto-color-emoji")
	}
	for i, g := range run.Glyphs {
		f, _ := text.Lookup(g.Face)
		if !f.IsColor(g.ID) {
			t.Errorf("emoji %d is not in colour", i)
			continue
		}
		m := f.RasterizeColor(g.ID, 20)
		colours := map[[3]byte]bool{}
		for i := 0; i+3 < len(m.Pix); i += 4 {
			if max(m.Pix[i], m.Pix[i+1], m.Pix[i+2]) > m.Pix[i+3] {
				t.Fatalf("pixel %v is not premultiplied", m.Pix[i:i+4])
			}
			colours[[3]byte(m.Pix[i:i+3])] = true
		}
		if len(colours) < 10 {
			t.Errorf("emoji %d has %d colours", i, len(colours))
		}
	}
}

// colored reports whether a glyph is in colours of its own.
func colored(g paint.Glyph) bool {
	f, ok := text.Lookup(g.Face)
	return ok && f.IsColor(g.ID)
}

// A character in a cell is as wide as the cell at most, and stays between its top and its foot, at any scale.
func TestTheBrowsersCharactersFitTheirCells(t *testing.T) {
	const size, w, ascent, descent = 14, 8, 13, 4
	for _, c := range []struct {
		s    string
		wide float32
	}{{"中", 2}, {"⇒", 1}, {"⟶", 1}, {"中", 1}, {"\U0001f600", 2}, {"\U0001f600", 1}, {"\U0001f1f8\U0001f1ea", 2}, {"ไ", 1}, {"x\u0302", 1}} {
		glyphs := text.GoMono(false, false).Cell(c.s, size, w*c.wide, ascent, descent)
		f := fromBrowser(t, c.s, glyphs)
		var tall float32
		for _, scale := range []float32{1, 1.5, 2, 3} {
			var m text.Mask
			if colored(glyphs[0]) {
				m = f.RasterizeColor(glyphs[0].ID, size*scale)
			} else {
				m = f.Rasterize(glyphs[0].ID, size*scale, 0, text.Raster{})
			}
			// A pixel over is the edge it smooths.
			if m.W == 0 || m.Offset.X < -1 || float32(m.Offset.X+m.W) > w*c.wide*scale+1 ||
				float32(-m.Offset.Y) > ascent*scale+1 || float32(m.Offset.Y+m.H) > descent*scale+1 {
				t.Errorf("%q in %v cells at scale %v is %dx%d at %v, in a cell %v wide, %v above and %v below", c.s, c.wide, scale,
					m.W, m.H, m.Offset, w*c.wide*scale, ascent*scale, descent*scale)
			}
			// As large as its scale, and not made larger after, give or take the pixels of its edges.
			if scale == 1 {
				tall = float32(m.H)
			} else if float32(m.H) < (tall-2)*scale {
				t.Errorf("%q in %v cells is %v tall, and at scale %v %d", c.s, c.wide, tall, scale, m.H)
			}
		}
	}
	if got := text.GoMono(false, false).Cell("\ue123", size, w, ascent, descent); len(got) != 0 {
		t.Errorf("a character for private use that no font here has is %+v", got)
	}
	if got := text.GoMono(false, false).Cell("e\u0301", size, w, ascent, descent); len(got) != 1 || got[0] != text.GoMono(false, false).Cell("é", size, w, ascent, descent)[0] {
		t.Errorf("e with an acute accent is %+v", got)
	}
}

// Text the browser draws wraps and takes a caret as other text does: Chinese breaks between characters, an Arabic word
// is whole and reads from the right, and the carets run in order.
func TestTheBrowsersTextLaysOut(t *testing.T) {
	face := text.Default()
	p := face.Layout(strings.Repeat("日本語", 10), text.Style{Size: 20}, 100)
	// As wide as the browser's font has them, or its box for what it has no font for.
	wide := face.Shape("日", 20).Advance
	if per := int(100 / wide); len(p.Lines) != (30+per-1)/per || p.Size.W > 100 || p.Size.W < 100-wide {
		t.Errorf("30 characters of %v px wrap into %d lines, %v wide, in 100", wide, len(p.Lines), p.Size.W)
	}
	run := face.Shape("ab中c\U0001f600d", 20)
	last := float32(-1)
	for i := run.Start; i <= run.End; i++ {
		if x := run.CaretX(i); x <= last {
			t.Errorf("the caret before rune %d is at %v, after %v", i, x, last)
		} else {
			last = x
		}
	}
	if math.Abs(float64(last-run.Advance)) > 0.1 {
		t.Errorf("the last caret is at %v of %v", last, run.Advance)
	}
	word := face.Shape("مرحبا", 20)
	fromBrowser(t, "an Arabic word", word.Glyphs)
	if word.CaretX(word.Start) < word.CaretX(word.End) {
		t.Errorf("an Arabic word's carets run from %v to %v", word.CaretX(word.Start), word.CaretX(word.End))
	}
	hebrew := face.Shape("שלום", 20)
	if len(hebrew.Glyphs) != 4 || hebrew.CaretX(0) < hebrew.CaretX(4) {
		t.Errorf("a Hebrew word is %d glyphs, with carets from %v to %v", len(hebrew.Glyphs), hebrew.CaretX(0), hebrew.CaretX(4))
	}
}

// white is what the tests draw text in, on black.
var white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// onBlack returns a frame of what draw draws, over black.
func onBlack(w *Window, draw func(p *paint.Painter)) []paint.Op {
	var p paint.Painter
	p.RRect(geom.Rect{Max: geom.Pt(w.Size().W, w.Size().H)}, 0, paint.Solid(color.NRGBA{A: 255}))
	draw(&p)
	return p.Ops()
}

// page returns perRow by rows Chinese characters, the first from.
func page(from rune, perRow, rows int) string {
	var b strings.Builder
	for i := range rune(perRow * rows) {
		b.WriteRune(from + i)
		if int(i)%perRow == perRow-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// showing reads the canvas in the task that drew it, as the sum of its red, and how many pixels are lit.
func showing(d *Driver) (sum, lit int) {
	pix := make([]byte, d.surfW*d.surfH*4)
	d.g.BindFramebuffer(gl.FRAMEBUFFER, 0)
	d.g.ReadPixels(pix, 0, 0, int32(d.surfW), int32(d.surfH), gl.RGBA, gl.UNSIGNED_BYTE)
	for i := 0; i < len(pix); i += 4 {
		sum += int(pix[i])
		if pix[i] > 100 {
			lit++
		}
	}
	return sum, lit
}

// presentOps sends w a frame and draws it.
func presentOps(t *testing.T, d *Driver, w *Window, ops []paint.Op) {
	t.Helper()
	if err := w.Present(ops, []geom.Rect{paint.Everything}); err != nil {
		t.Fatal(err)
	}
	d.frame()
}

// finish draws the frames that w asks for until it asks for none, calling each after every one, and returns how many.
func finish(t *testing.T, d *Driver, w *Window, each func()) (frames int) {
	t.Helper()
	for give := time.Now().Add(20 * time.Second); !w.pane.R.Stretched.Empty(); frames++ {
		if time.Now().After(give) {
			t.Fatalf("the window is not whole after %d frames", frames)
		}
		time.Sleep(2 * time.Millisecond)
		w.sharpen = true
		d.frame()
		if each != nil {
			each()
		}
	}
	return frames
}

// A screenful of new characters is drawn over several frames, each a few milliseconds' worth, with no frame from the
// program, until it is whole. Those that come late fade in.
func TestNewCharactersOfTheBrowsersTakeSeveralFrames(t *testing.T) {
	d, w := testWindow(t)
	ops := onBlack(w, func(p *paint.Painter) {
		text.Default().Layout(page(0x5000, 22, 20), text.Style{Size: 11}, 0).Paint(p, geom.Point{}, white)
	})
	presentOps(t, d, w, ops)
	first, _ := showing(d)
	if w.pane.R.Stretched.Empty() || w.sharpTimer == nil {
		t.Fatal("the first frame draws it all, or asks for no other")
	}
	last := first
	frames := finish(t, d, w, func() {
		sum, _ := showing(d)
		if sum < last {
			t.Errorf("the text goes from %d to %d", last, sum)
		}
		last = sum
	})
	if frames < 5 || last <= first {
		t.Errorf("from %d it goes to %d in %d frames", first, last, frames)
	}
	presentOps(t, d, w, ops)
	if got, _ := showing(d); got != last || !w.pane.R.Stretched.Empty() {
		t.Errorf("drawn again at once it is %d, and over %d frames %d", got, frames, last)
	}
	noGLError(t, d)
}

// A character that comes late fades in, over several frames, in text and in a cell.
func TestALateCharacterFadesIn(t *testing.T) {
	for i, cell := range []bool{false, true} {
		d, w := testWindow(t)
		from := rune(0x8000 + 0x200*i)
		scale := d.scale
		// By a clock of the test's, 60 frames to its second.
		present(t, w, color.NRGBA{A: 255})
		d.frame()
		w.pane.R.Held = time.Now()
		presentOps(t, d, w, onBlack(w, func(p *paint.Painter) {
			text.Default().Layout(page(from, 40, 8), text.Style{Size: 5}, 0).Paint(p, geom.Pt(0, 180), white)
			if !cell {
				text.Default().Shape(string(from+400), 60).Paint(p, geom.Pt(10, 10), white)
				return
			}
			cw, ch := int(70*scale), int(80*scale)
			size := geom.Sz(float32(cw)/scale, float32(ch)/scale)
			g := text.GoMono(false, false).Cell(string(from+400), 60, size.W, 60, 20)
			p.Cells(geom.Pt(0, 0), size, []paint.CellPaint{{FG: white, Text: true, Glyph: g[0]}}, &paint.Patterns{W: cw, H: ch}, 1, 0, 60, 60)
		}))
		brightest := func() (most byte) {
			n := int(100 * scale)
			pix := make([]byte, n*n*4)
			d.g.BindFramebuffer(gl.FRAMEBUFFER, 0)
			d.g.ReadPixels(pix, 0, int32(d.surfH-n), int32(n), int32(n), gl.RGBA, gl.UNSIGNED_BYTE)
			for i := 0; i < len(pix); i += 4 {
				most = max(most, pix[i])
			}
			return most
		}
		if b := brightest(); b != 0 {
			t.Fatalf("cell: %v: it is there at %d in the first frame", cell, b)
		}
		var seen []byte
		finish(t, d, w, func() {
			w.pane.R.Held = w.pane.R.Held.Add(time.Second / 60)
			b := brightest()
			if n := len(seen); n > 0 && b < seen[n-1] {
				t.Errorf("cell: %v: it goes from %d to %d", cell, seen[n-1], b)
			}
			if b > 0 && (len(seen) == 0 || b != seen[len(seen)-1]) {
				seen = append(seen, b)
			}
		})
		if len(seen) < 6 || seen[0] > 60 || seen[len(seen)-1] < 240 {
			t.Errorf("cell: %v: it comes in as %v", cell, seen)
		}
	}
}

// A character that was to come late and went out of sight before it was drawn is there at once where it comes back.
func TestALateCharacterNeverDrawnFadesInNoMore(t *testing.T) {
	d, w := testWindow(t)
	const from = 0x8800
	present(t, w, color.NRGBA{A: 255})
	d.frame()
	w.pane.R.Held = time.Now()
	one := func(p *paint.Painter) {
		text.Default().Shape(string(rune(from+400)), 60).Paint(p, geom.Pt(10, 10), white)
	}
	presentOps(t, d, w, onBlack(w, func(p *paint.Painter) {
		text.Default().Layout(page(from, 40, 8), text.Style{Size: 5}, 0).Paint(p, geom.Pt(0, 180), white)
		one(p)
	}))
	if w.pane.R.Stretched.Empty() {
		t.Fatal("nothing waits")
	}
	for range 3 {
		w.pane.R.Held = w.pane.R.Held.Add(time.Second)
		presentOps(t, d, w, onBlack(w, func(*paint.Painter) {}))
	}
	presentOps(t, d, w, onBlack(w, one))
	if _, lit := showing(d); lit < 100 || !w.pane.R.Stretched.Empty() {
		t.Errorf("it shows %d lit pixels as it comes back, and fades: %v", lit, !w.pane.R.Stretched.Empty())
	}
}

// TurnTable shapes words no text had before until the fonts have dropped glyphs three times, with frame called often.
// The tests outside this package use it too.
func TurnTable(t *testing.T, frame func()) {
	t.Helper()
	turned := text.Turns()
	for i := 0; text.Turns() < turned+3; i++ {
		if i > 2000 {
			t.Fatal("the table does not turn")
		}
		var b strings.Builder
		for range 100 {
			n := wordsMade
			wordsMade++
			for range 4 {
				b.WriteRune([]rune("بتثجحخسشصضطظعغفقكلمنهي")[n%22])
				n /= 22
			}
			b.WriteByte(' ')
		}
		text.Default().Shape(b.String(), 12)
		if i%10 == 0 {
			frame()
		}
	}
}

// wordsMade counts the words TurnTable has made.
var wordsMade int

// What a window shows is drawn as it was, however many glyphs the fonts drop while other windows alone draw.
func TestWhatAWindowShowsOutlivesTheTable(t *testing.T) {
	d, w := testWindow(t)
	popup := testPopup(t, d, w)
	defer func() { _ = popup.Close() }()
	run := text.Default().Shape("مرحبا", 40)
	ops := onBlack(w, func(p *paint.Painter) { run.Paint(p, geom.Pt(100, 10), white) })
	presentOps(t, d, w, ops)
	finish(t, d, w, nil)
	want, _ := showing(d)
	TurnTable(t, func() {
		present(t, popup, testRed)
		d.frame()
	})
	present(t, popup, color.NRGBA{A: 255})
	presentOps(t, d, w, ops)
	face, _ := text.Lookup(run.Glyphs[0].Face)
	if got, _ := showing(d); got != want || face.Reach(run.Glyphs[0].ID) == 0 {
		t.Errorf("after three turns of the table it is %d, and was %d", got, want)
	}
}

// What waits for a later frame is drawn whole there however large the view is zoomed.
func TestLateCharactersAreWholeUnderAZoom(t *testing.T) {
	for _, zoom := range []paint.Transform{{A: 4, E: 4}, {A: 3.5, B: 0.6, D: -0.6, E: 3.5, C: 60}} {
		d, w := testWindow(t)
		from := rune(0x6000)
		if zoom.B != 0 {
			from = 0x6400
		}
		ops := onBlack(w, func(p *paint.Painter) {
			// Enough before it to spend a frame's time, and then the one looked at.
			text.Default().Layout(page(from, 40, 8), text.Style{Size: 5}, 0).Paint(p, geom.Pt(0, 180), white)
			defer p.Push(zoom)()
			text.Default().Shape(string([]rune{from + 400, from + 401}), 14).Paint(p, geom.Pt(10, 5), white)
		})
		presentOps(t, d, w, ops)
		if w.pane.R.Stretched.Empty() {
			t.Fatal("nothing waits")
		}
		finish(t, d, w, nil)
		// Over the same frame, as only the damage was drawn.
		got, _ := showing(d)
		presentOps(t, d, w, ops)
		if want, _ := showing(d); got != want || want == 0 {
			t.Errorf("under %v it is %d, and drawn whole %d", zoom, got, want)
		}
	}
}

// An atlas that starts again, as it does when it fills, draws what the browser made before without the browser, and in
// the same frame.
func TestAFullAtlasAsksTheBrowserForNothingAgain(t *testing.T) {
	d, w := testWindow(t)
	ops := onBlack(w, func(p *paint.Painter) {
		text.Default().Layout(page(0x7000, 20, 6), text.Style{Size: 12}, 0).Paint(p, geom.Point{}, white)
	})
	presentOps(t, d, w, ops)
	finish(t, d, w, nil)
	want, _ := showing(d)

	proto := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	was := proto.Get("fillText")
	counter := js.Global().Call("eval", `(() => { const f = () => { f.calls++; }; f.calls = 0; return f; })()`)
	defer proto.Set("fillText", was)
	proto.Set("fillText", counter)
	// Letters of so many sizes that they fill the atlas more than once.
	presentOps(t, d, w, onBlack(w, func(p *paint.Painter) {
		for i := range 60 {
			text.Default().Shape("ABCDEFGHIJKLM", float32(120+i)).Paint(p, geom.Pt(0, 0), white)
		}
	}))
	presentOps(t, d, w, ops)
	if !w.pane.R.Stretched.Empty() {
		t.Error("what the browser made before waits for a frame to come")
	}
	if got, _ := showing(d); got != want {
		t.Errorf("after the atlas filled the text is %d, and was %d", got, want)
	}
	if calls := counter.Get("calls").Int(); calls != 0 {
		t.Errorf("the browser drew %d glyphs again", calls)
	}
}

// Text larger than any picture made of it, a glyph of it or a word, is stretched to its size, and drawn.
func TestHugeTextOfTheBrowsersIsDrawn(t *testing.T) {
	d, w := testWindow(t)
	size := w.Size()
	for _, c := range []struct {
		name, s string
		size    float32
	}{
		{"a character", "中", 3 * size.H}, {"a character wider than a texture", "中", 2500},
		{"a word", "مرحبا", 2500}, {"a long word", strings.Repeat("بيت", 70), 20}, {"a long large word", strings.Repeat("بيت", 70), 300},
		{"an emoji", "\U0001f600", 2500},
	} {
		run := text.Default().Shape(c.s, c.size)
		// Across its middle, a window's width at a time, as the box for what there is no font for is hollow.
		lit := 0
		for x := float32(0); x < min(run.Advance, 4000); x += size.W {
			presentOps(t, d, w, onBlack(w, func(p *paint.Painter) {
				run.Paint(p, geom.Pt(-x, size.H/2-run.Height()/2), white)
			}))
			finish(t, d, w, nil)
			_, n := showing(d)
			lit += n
		}
		if lit < 200 {
			t.Errorf("%s at %v px, %v wide, shows %d pixels", c.name, c.size, run.Advance, lit)
		}
	}
	// Scrolled so a word's start is far out of sight, the rest of it shows, drawn again in part too.
	run := text.Default().Shape(strings.Repeat("بيت", 7), 40)
	if run.Advance < 5*40 {
		t.Fatalf("a long word is %v wide", run.Advance)
	}
	ops := onBlack(w, func(p *paint.Painter) { run.Paint(p, geom.Pt(-run.Advance+60, 20), white) })
	presentOps(t, d, w, ops)
	finish(t, d, w, nil)
	before, _ := showing(d)
	part := onBlack(w, func(p *paint.Painter) { run.Paint(p, geom.Pt(-run.Advance+60, 20), white) })
	if err := w.Present(part, []geom.Rect{{Max: geom.Pt(80, 100)}}); err != nil {
		t.Fatal(err)
	}
	d.frame()
	if got, _ := showing(d); got != before || before == 0 {
		t.Errorf("drawn again in part it is %d, and was %d", got, before)
	}
	noGLError(t, d)
}
