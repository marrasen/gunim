//go:build linux || windows || darwin

package render

import (
	"image/color"
	"math"
	"math/rand"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// readFrame returns the canvas's pixels, row by row from the top.
func readFrame(r *Renderer) []byte {
	w, h := r.fbW, r.fbH
	pix := make([]byte, w*h*4)
	r.GL.BindFramebuffer(0x8D40, r.fbo(0))
	r.GL.ReadPixels(pix, 0, 0, int32(w), int32(h), 0x1908, 0x1401)
	out := make([]byte, len(pix))
	for y := range h {
		copy(out[y*w*4:(y+1)*w*4], pix[(h-1-y)*w*4:(h-y)*w*4])
	}
	return out
}

// Cells drawn by their own program show each cell's background, and
// each pattern over it in the foreground colour, pixel for pixel, with
// hard edges and nothing past the grid, at any scale.
func TestCellsDrawPixelForPixel(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	for _, scale := range []float32{1, 1.5, 2} {
		const cols, rows = 23, 9
		cw, ch := 9, 18
		pats := &paint.Patterns{W: cw, H: ch}
		// An upper half, a shade, and a vertical bar.
		type piece struct {
			x0, y0, x1, y1 int
			alpha          uint8
		}
		shapes := []piece{{0, 0, cw, ch / 2, 255}, {0, 0, cw, ch, 128}, {3, 0, 6, ch, 255}}
		for _, s := range shapes {
			m := make([]byte, cw*ch)
			for y := s.y0; y < s.y1; y++ {
				for x := s.x0; x < s.x1; x++ {
					m[y*cw+x] = s.alpha
				}
			}
			pats.Masks = append(pats.Masks, m)
		}
		size := geom.Sz(float32(cw)/scale, float32(ch)/scale)
		rng := rand.New(rand.NewSource(int64(scale * 10)))
		colour := func() color.NRGBA {
			a := uint8(255)
			if rng.Intn(4) == 0 {
				a = uint8(rng.Intn(256))
			}
			return color.NRGBA{R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: a}
		}
		grid := make([][]paint.CellPaint, rows)
		for y := range grid {
			grid[y] = make([]paint.CellPaint, cols)
			for x := range grid[y] {
				grid[y][x] = paint.CellPaint{BG: colour(), FG: colour(), Pattern: uint16(rng.Intn(len(shapes) + 1))}
			}
		}
		// Whole device pixels from the corner, as a grid puts itself.
		at := geom.Pt(30/scale, 20/scale)
		bg := color.NRGBA{R: 0x20, G: 0x30, B: 0x40, A: 0xff}
		w, h := int(benchSize.W), int(benchSize.H)

		var fast paint.Painter
		fast.RRect(geom.Rect{Max: geom.Pt(float32(w)/scale, float32(h)/scale)}, 0, paint.Solid(bg))
		for y, row := range grid {
			fast.Cells(geom.Pt(at.X, at.Y+float32(y)*size.H), size, row, pats, 1, y, 0, 0)
		}
		// What each pixel should be: the background over the canvas, and
		// the pattern's coverage of the foreground over that.
		over := func(dst [3]float64, c color.NRGBA, cover float64) [3]float64 {
			a := float64(c.A) / 255 * cover
			return [3]float64{
				float64(c.R)/255*a + dst[0]*(1-a),
				float64(c.G)/255*a + dst[1]*(1-a),
				float64(c.B)/255*a + dst[2]*(1-a),
			}
		}
		base := [3]float64{float64(bg.R) / 255, float64(bg.G) / 255, float64(bg.B) / 255}
		want := make([]byte, w*h*4)
		for py := range h {
			for px := range w {
				v := base
				gx, gy := px-int(at.X*scale+0.5), py-int(at.Y*scale+0.5)
				if gx >= 0 && gy >= 0 && gx < cols*cw && gy < rows*ch {
					c := grid[gy/ch][gx/cw]
					v = over(v, c.BG, 1)
					if c.Pattern != 0 {
						v = over(v, c.FG, float64(pats.Masks[c.Pattern-1][(gy%ch)*cw+gx%cw])/255)
					}
				}
				i := (py*w + px) * 4
				want[i], want[i+1], want[i+2], want[i+3] = byte(v[0]*255+0.5), byte(v[1]*255+0.5), byte(v[2]*255+0.5), 255
			}
		}
		r.draws = 0
		r.Draw(fast.Ops(), paint.Everything, w, h, scale)
		got := readFrame(r)
		if r.draws > 3 {
			t.Errorf("at scale %v the grid took %d draws, want its rows together", scale, r.draws)
		}
		bad := 0
		for i := range got {
			if d := int(got[i]) - int(want[i]); d < -2 || d > 2 {
				if bad < 5 {
					p := i / 4
					t.Errorf("at scale %v pixel %d,%d is %v, want %v", scale, p%w, p/w, got[p*4:p*4+4], want[p*4:p*4+4])
				}
				bad++
			}
		}
		if bad > 0 {
			t.Fatalf("at scale %v %d channels differ", scale, bad)
		}
	}
}

// What is drawn between rows of cells stays over the rows before it and
// under the rows after.
func TestCellsKeepTheirPlaceAmongOtherDrawing(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pats := &paint.Patterns{W: 10, H: 10}
	red, blue, green := color.NRGBA{R: 0xff, A: 0xff}, color.NRGBA{B: 0xff, A: 0xff}, color.NRGBA{G: 0xff, A: 0xff}
	row := func(c color.NRGBA) []paint.CellPaint { return []paint.CellPaint{{BG: c}, {BG: c}, {BG: c}} }
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(color.NRGBA{A: 0xff}))
	p.Cells(geom.Pt(0, 0), geom.Sz(10, 10), row(red), pats, 1, 0, 0, 0)
	// Green over the first row and the second, before the second is drawn.
	p.RRect(geom.Rc(0, 0, 30, 20), 0, paint.Solid(green))
	p.Cells(geom.Pt(0, 10), geom.Sz(10, 10), row(blue), pats, 1, 1, 0, 0)
	w, h := int(benchSize.W), int(benchSize.H)
	r.Draw(p.Ops(), paint.Everything, w, h, 1)
	pix := readFrame(r)
	at := func(x, y int) [3]byte { i := (y*w + x) * 4; return [3]byte{pix[i], pix[i+1], pix[i+2]} }
	if got := at(5, 5); got != [3]byte{0, 0xff, 0} {
		t.Errorf("over the first row is %v, want the green drawn after it", got)
	}
	if got := at(5, 15); got != [3]byte{0, 0, 0xff} {
		t.Errorf("over the second row is %v, want the row, drawn after the green", got)
	}
}

// Glyphs drawn with the cells look as the same glyphs drawn as text
// over the cells' backgrounds do, greyscale and on subpixels, at any
// scale; and glyphs too big for their cells, drawn as text after the
// cells, look so too.
func TestGlyphsInCellsDrawAsText(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	face := text.GoMono(false, false)
	renderings := []text.Rendering{{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}}
	if r.dual {
		renderings = append(renderings, lcd)
	}
	const line = "Hello, gunim! {gjy}|@#~ AW"
	for _, tr := range renderings {
		for _, scale := range []float32{1, 1.5} {
			for _, tight := range []bool{false, true} {
				r.SetText(tr, false)
				const size = 14
				_, adv, _ := face.Glyph('M', size)
				ascent, descent, _ := face.Metrics(size)
				cw := int(math.Ceil(float64(adv * scale)))
				ch := int(math.Ceil(float64((ascent + descent) * scale)))
				if tight {
					// Shorter than the glyphs: they spill out, and are
					// drawn as text.
					ch = int(float64(ascent*scale) * 0.7)
				}
				cell := geom.Sz(float32(cw)/scale, float32(ch)/scale)
				base := float32(math.Round(float64(ascent*scale))) / scale
				at := geom.Pt(20/scale, 30/scale)
				rng := rand.New(rand.NewSource(7))
				colour := func(light bool) color.NRGBA {
					v := func() uint8 {
						if light {
							return uint8(160 + rng.Intn(96))
						}
						return uint8(rng.Intn(96))
					}
					return color.NRGBA{R: v(), G: v(), B: v(), A: 255}
				}
				rows := make([][]paint.CellPaint, 3)
				for y := range rows {
					for _, ch := range line {
						gly, _, ok := face.Glyph(ch, size)
						dark := rng.Intn(2) == 0
						c := paint.CellPaint{BG: colour(!dark), FG: colour(dark), Text: ok && ch != ' ', Glyph: gly}
						c.Glyph.At = geom.Point{}
						rows[y] = append(rows[y], c)
					}
				}
				w, h := int(benchSize.W), int(benchSize.H)
				bg := color.NRGBA{R: 0x20, G: 0x30, B: 0x40, A: 0xff}
				pats := &paint.Patterns{W: cw, H: ch}

				var fast paint.Painter
				fast.RRect(geom.Rect{Max: geom.Pt(float32(w)/scale, float32(h)/scale)}, 0, paint.Solid(bg))
				for y, row := range rows {
					fast.Cells(geom.Pt(at.X, at.Y+float32(y)*cell.H), cell, row, pats, 1, y, size, base)
				}
				var slow paint.Painter
				slow.RRect(geom.Rect{Max: geom.Pt(float32(w)/scale, float32(h)/scale)}, 0, paint.Solid(bg))
				// The backgrounds as cells without glyphs, whose edges are
				// as hard as the cells', and the glyphs as text after.
				for y, row := range rows {
					bare := make([]paint.CellPaint, len(row))
					for x, c := range row {
						bare[x] = paint.CellPaint{BG: c.BG}
					}
					slow.Cells(geom.Pt(at.X, at.Y+float32(y)*cell.H), cell, bare, pats, 2, y, size, base)
				}
				for y, row := range rows {
					for x, c := range row {
						if !c.Text {
							continue
						}
						g := c.Glyph
						g.At = geom.Pt(at.X+float32(x)*cell.W, at.Y+float32(y)*cell.H+base)
						slow.Text([]paint.Glyph{g}, size, c.FG, geom.Rect{Max: geom.Pt(float32(w), float32(h))})
					}
				}
				r.Draw(slow.Ops(), paint.Everything, w, h, scale)
				r.Draw(slow.Ops(), paint.Everything, w, h, scale)
				want := readFrame(r)
				r.cellsState.inCells, r.cellsState.spilled = 0, 0
				r.Draw(fast.Ops(), paint.Everything, w, h, scale)
				r.Draw(fast.Ops(), paint.Everything, w, h, scale)
				got := readFrame(r)
				if in, out := r.cellsState.inCells, r.cellsState.spilled; tight && out == 0 || !tight && in < out*4 {
					t.Errorf("%v at scale %v, tight %v: %d glyphs drawn in their cells and %d as text", tr.Smoothing, scale, tight, in, out)
				}
				// The row's own area: the rectangles' soft edges spill
				// past it, where the cells' do not.
				bad := 0
				gx0, gy0 := int(at.X*scale+0.5), int(at.Y*scale+0.5)
				gx1, gy1 := gx0+cw*len(rows[0]), gy0+ch*len(rows)
				for py := gy0; py < gy1; py++ {
					for px := gx0; px < gx1; px++ {
						i := (py*w + px) * 4
						for k := range 3 {
							if d := int(got[i+k]) - int(want[i+k]); d < -3 || d > 3 {
								if bad < 3 {
									t.Errorf("%v at scale %v, tight %v: pixel %d,%d is %v, want %v", tr.Smoothing, scale, tight, px, py, got[i:i+4], want[i:i+4])
								}
								bad++
								break
							}
						}
					}
				}
				if bad > 0 {
					t.Fatalf("%v at scale %v, tight %v: %d pixels differ", tr.Smoothing, scale, tight, bad)
				}
			}
		}
	}
}
