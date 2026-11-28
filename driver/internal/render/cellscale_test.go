//go:build linux || windows || darwin

package render

import (
	"fmt"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// boxArms are the arms of the box-drawing characters the tests draw: left, right, up and down. Pattern n is
// boxArms[n-1].
var boxArms = [...][4]bool{
	{true, true, false, false}, // ─
	{false, false, true, true}, // │
	{false, true, false, true}, // ┌
	{true, true, false, true},  // ┬
	{true, false, false, true}, // ┐
	{false, true, true, true},  // ├
	{true, true, true, true},   // ┼
	{true, false, true, true},  // ┤
	{false, true, true, false}, // └
	{true, true, true, false},  // ┴
	{true, false, true, false}, // ┘
}

// boxMask draws pattern i+1 of boxArms in a w by h cell, as a grid draws box drawing: lines a pixel or more thick
// through the cell's middle, on whole pixels.
func boxMask(i, w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}
	m := make([]byte, w*h)
	t := max(1, int(math.Round(float64(w)/9)))
	vx, hy := (w-t)/2, (h-t)/2
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				m[y*w+x] = 255
			}
		}
	}
	a := boxArms[i]
	if a[0] {
		fill(0, hy, vx+t, hy+t)
	}
	if a[1] {
		fill(vx, hy, w, hy+t)
	}
	if a[2] {
		fill(vx, 0, vx+t, hy+t)
	}
	if a[3] {
		fill(vx, hy, vx+t, h)
	}
	return m
}

// boxTable is a table of two columns and two rows drawn in box drawing, as pattern numbers.
var boxTable = [5][7]uint16{
	{3, 1, 1, 4, 1, 1, 5},
	{2, 0, 0, 2, 0, 0, 2},
	{6, 1, 1, 7, 1, 1, 8},
	{2, 0, 0, 2, 0, 0, 2},
	{9, 1, 1, 10, 1, 1, 11},
}

// boxPatterns returns the patterns of boxArms for cells of w by h device pixels, made again at any size when
// sized, or stretched from these by the driver when not.
func boxPatterns(w, h int, sized bool) *paint.Patterns {
	pats := &paint.Patterns{W: w, H: h}
	for i := range boxArms {
		pats.Masks = append(pats.Masks, boxMask(i, w, h))
	}
	if sized {
		pats.MaskAt = boxMask
	}
	return pats
}

// zoomCase is a grid drawn under a zoom: its scale, and its corner, off the pixel grid as a view zoomed about a
// point puts it.
type zoomCase struct {
	k      float32
	corner geom.Point
}

var zoomCases = []zoomCase{
	{0.73, geom.Pt(37.3, 21.6)},
	{0.5, geom.Pt(10.5, 10.5)},
	{0.86, geom.Pt(3.25, 7.75)},
	{2.0 / 3, geom.Pt(11.1, 5.9)},
	{1, geom.Pt(20.5, 30.5)},
	{1.25, geom.Pt(0.5, 0.5)},
	{1.6, geom.Pt(8.4, 2.2)},
}

// zoomed returns the transform of a grid zoomed as c says.
func (c zoomCase) zoomed() paint.Transform {
	return paint.Translate(c.corner).Mul(paint.Scale(c.k, geom.Point{}))
}

// splitAt are rows of the texture of cells to start a frame's rows at: its first, and so near its end that a grid's
// rows are drawn in two runs, as every so many frames they are.
var splitAt = []int{0, cellsRows - 4}

// drawCells draws rows of cells of cw by ch device pixels under c's zoom, at scale 1, starting the texture of cells
// at the row start, and returns the frame and where the renderer put the cells.
func drawCells(r *Renderer, c zoomCase, rows [][]paint.CellPaint, pats *paint.Patterns, start int) ([]byte, cellGrid) {
	cw, ch := float32(pats.W), float32(pats.H)
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	end := p.Push(c.zoomed())
	for y, row := range rows {
		p.Cells(geom.Pt(0, float32(y)*ch), geom.Sz(cw, ch), row, pats, 1, y, 0, 0)
	}
	end()
	if r.cellsReady() {
		r.cellsState.row = start
	}
	r.Draw(p.Ops(), paint.Everything, int(benchSize.W), int(benchSize.H), 1)
	grid, _ := r.cellGridOf(&paint.CellsOp{Size: geom.Sz(cw, ch), Cells: rows[0], Transform: c.zoomed()}, pats.W, pats.H)
	return readFrame(r), grid
}

// Every pixel of a zoomed grid shows the cell the renderer's edges give it, whatever the scale and the corner, and
// whichever run of rows the cell was drawn in: the cells program and the renderer agree to the pixel, rows keep their
// height where a grid's rows are drawn in two runs, and nothing past the last row or column is drawn.
func TestZoomedCellsLandOnTheRenderersEdges(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	const cols, nrows = 20, 12
	pats := &paint.Patterns{W: 9, H: 18}
	rows := make([][]paint.CellPaint, nrows)
	for y := range rows {
		for x := range cols {
			rows[y] = append(rows[y], paint.CellPaint{BG: color.NRGBA{R: uint8(10 + 12*x), G: uint8(10 + 12*y), B: 200, A: 255}})
		}
	}
	w := int(benchSize.W)
	for _, c := range zoomCases {
		for _, start := range splitAt {
			pix, g := drawCells(r, c, rows, pats, start)
			cellAt := func(p, axis, n int) int {
				for i := range n {
					if g.edge(i, axis) <= p && p < g.edge(i+1, axis) {
						return i
					}
				}
				return -1
			}
			bad := 0
			for py := g.edge(0, 1) - 2; py < g.edge(nrows, 1)+2; py++ {
				for px := g.edge(0, 0) - 2; px < g.edge(cols, 0)+2; px++ {
					if px < 0 || py < 0 {
						continue
					}
					want := [3]byte{}
					if x, y := cellAt(px, 0, cols), cellAt(py, 1, nrows); x >= 0 && y >= 0 {
						want = [3]byte{uint8(10 + 12*x), uint8(10 + 12*y), 200}
					}
					i := (py*w + px) * 4
					if got := [3]byte{pix[i], pix[i+1], pix[i+2]}; got != want {
						if bad < 3 {
							t.Errorf("scale %v, corner %v, rows from %d: pixel %d,%d is %v, want %v", c.k, c.corner, start, px, py, got, want)
						}
						bad++
					}
				}
			}
			if bad > 0 {
				t.Errorf("scale %v, corner %v, rows from %d: %d pixels show the wrong cell", c.k, c.corner, start, bad)
			}
			for y := range nrows {
				if h, want := g.edge(y+1, 1)-g.edge(y, 1), float64(pats.H)*float64(c.k); math.Abs(float64(h)-want) >= 1 {
					t.Errorf("scale %v, corner %v: row %d is %d pixels tall, want %.2f give or take a fraction", c.k, c.corner, y, h, want)
				}
			}
			for _, e := range [][2]float64{{float64(g.edge(1, 0)), float64(c.corner.X) + 9*float64(c.k)}, {float64(g.edge(5, 1)), float64(c.corner.Y) + 5*18*float64(c.k)}} {
				if math.Abs(e[0]-e[1]) > 0.5 {
					t.Errorf("scale %v, corner %v: an edge lands at %v, %.2f from where it belongs, want half a pixel at most", c.k, c.corner, e[0], e[0]-e[1])
				}
			}
		}
	}
}

// A table in box drawing drawn under a zoom shows every one of its lines whole, at every scale, below one and
// above, and where its rows are drawn in two runs: none is lost between the pixels the cells land on. With patterns
// made at the size shown, the lines are sharp, on whole pixels, and every line is as thick as every other.
func TestBoxDrawingUnderAZoomDrawsEveryLine(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	const cw, ch = 9, 18
	w := int(benchSize.W)
	for _, sized := range []bool{true, false} {
		for _, c := range zoomCases {
			for _, start := range splitAt {
				pats := boxPatterns(cw, ch, sized)
				rows := make([][]paint.CellPaint, len(boxTable))
				for y, row := range boxTable {
					for _, n := range row {
						rows[y] = append(rows[y], paint.CellPaint{BG: color.NRGBA{A: 255}, FG: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Pattern: n})
					}
				}
				pix, g := drawCells(r, c, rows, pats, start)
				name := func() string {
					return fmt.Sprintf("sized %v, scale %v, corner %v, rows from %d", sized, c.k, c.corner, start)
				}
				lit := func(x, y int) bool { return pix[(y*w+x)*4] > 200 }
				// A point a share f of the way across cell x, or down row y, in device pixels.
				across := func(x int, f float64) int { return g.edge(x, 0) + int(f*float64(g.edge(x+1, 0)-g.edge(x, 0))) }
				down := func(y int, f float64) int { return g.edge(y, 1) + int(f*float64(g.edge(y+1, 1)-g.edge(y, 1))) }
				// Each line across the table, rows 0, 2 and 4: some row of pixels in its row of cells is lit all the
				// way from the first column to the last. Each line down, columns 0, 3 and 6, likewise.
				thick := make([]int, 0, 6)
				for _, row := range []int{0, 2, 4} {
					n := 0
					for y := g.edge(row, 1); y < g.edge(row+1, 1); y++ {
						all := true
						for x := across(0, 0.75); x <= across(6, 0.25); x++ {
							all = all && lit(x, y)
						}
						if all {
							n++
						}
					}
					if n == 0 {
						t.Errorf("%s: the line across row %d is broken or missing", name(), row)
					}
					thick = append(thick, n)
				}
				for _, col := range []int{0, 3, 6} {
					n := 0
					for x := g.edge(col, 0); x < g.edge(col+1, 0); x++ {
						all := true
						for y := down(0, 0.75); y <= down(4, 0.25); y++ {
							all = all && lit(x, y)
						}
						if all {
							n++
						}
					}
					if n == 0 {
						t.Errorf("%s: the line down column %d is broken or missing", name(), col)
					}
					thick = append(thick, n)
				}
				if !sized {
					continue
				}
				if thick[1] != thick[0] || thick[2] != thick[0] || thick[4] != thick[3] || thick[5] != thick[3] {
					t.Errorf("%s: the lines across and down are %v pixels thick, want each way the same", name(), thick)
				}
				// Sharp: every pixel of the table is lit or dark, none part way.
				soft := 0
				for y := g.edge(0, 1); y < g.edge(5, 1); y++ {
					for x := g.edge(0, 0); x < g.edge(7, 0); x++ {
						if v := pix[(y*w+x)*4]; v > 40 && v < 215 {
							soft++
						}
					}
				}
				if soft > 0 {
					t.Errorf("%s: %d pixels of the table are neither lit nor dark, want the lines on whole pixels", name(), soft)
				}
			}
		}
	}
}

// A block cursor and an underline drawn over zoomed cells, as a grid draws them, at the cells' places, cover their
// cells within half a pixel: the cells land within half a pixel of where they belong.
func TestACursorCoversItsZoomedCell(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	const cw, ch = 9, 18
	w := int(benchSize.W)
	red, green := color.NRGBA{R: 255, A: 255}, color.NRGBA{G: 255, A: 255}
	pats := &paint.Patterns{W: cw, H: ch}
	for _, c := range append(zoomCases, zoomCase{0.73, geom.Pt(10.5, 10.5)}) {
		var p paint.Painter
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		end := p.Push(c.zoomed())
		row := make([]paint.CellPaint, 12)
		for i := range row {
			row[i] = paint.CellPaint{BG: color.NRGBA{B: 255, A: 255}}
		}
		for y := range 6 {
			p.Cells(geom.Pt(0, float32(y*ch)), geom.Sz(cw, ch), row, pats, 1, y, 0, 0)
		}
		// The cursor on cell 1 of row 3, and an underline under cells 4 to 8 of row 4, its last two pixels.
		p.RRect(geom.Rc(cw, 3*ch, cw, ch), 0, paint.Solid(red))
		p.RRect(geom.Rc(4*cw, 5*ch-2, 5*cw, 2), 0, paint.Solid(green))
		end()
		r.Draw(p.Ops(), paint.Everything, w, h(), 1)
		pix := readFrame(r)
		g, _ := r.cellGridOf(&paint.CellsOp{Size: geom.Sz(cw, ch), Cells: row, Transform: c.zoomed()}, cw, ch)
		at := func(x, y, ch int) byte { return pix[(y*w+x)*4+ch] }
		// The cursor: every pixel of its cell red, at least half; and the pixels round it less than half.
		x0, x1, y0, y1 := g.edge(1, 0), g.edge(2, 0), g.edge(3, 1), g.edge(4, 1)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if at(x, y, 0) < 120 {
					t.Fatalf("scale %v, corner %v: the cursor leaves pixel %d,%d of its cell, %d to %d by %d to %d, uncovered",
						c.k, c.corner, x, y, x0, x1, y0, y1)
				}
			}
		}
		mid := (y0 + y1) / 2
		if at(x0-1, mid, 0) > 135 || at(x1, mid, 0) > 135 {
			t.Errorf("scale %v, corner %v: the cursor reaches over the cells beside its own, %d and %d red", c.k, c.corner,
				at(x0-1, mid, 0), at(x1, mid, 0))
		}
		// The underline runs from the left edge of cell 4 to the right of cell 8, within half a pixel.
		uy := g.edge(5, 1) - 1
		if at(g.edge(4, 0), uy, 1) < 120 || at(g.edge(9, 0)-1, uy, 1) < 120 ||
			at(g.edge(4, 0)-1, uy, 1) > 135 || at(g.edge(9, 0), uy, 1) > 135 {
			t.Errorf("scale %v, corner %v: the underline's ends are %d %d | %d %d green, want it to end at its cells' edges",
				c.k, c.corner, at(g.edge(4, 0)-1, uy, 1), at(g.edge(4, 0), uy, 1), at(g.edge(9, 0)-1, uy, 1), at(g.edge(9, 0), uy, 1))
		}
	}
}

// h is the test canvas's height.
func h() int { return int(benchSize.H) }

// zoomGrid is rows of glyphs in cells, and their size, for the zoom tests.
type zoomGrid struct {
	rows           [6][]paint.CellPaint
	cw, ch, ascent float32
	pats           *paint.Patterns
}

func newZoomGrid() *zoomGrid {
	face := text.GoMono(false, false)
	_, adv, _ := face.Glyph('M', 14)
	ascent, descent, _ := face.Metrics(14)
	z := &zoomGrid{cw: float32(math.Ceil(float64(adv))), ch: float32(math.Ceil(float64(ascent + descent))), ascent: ascent}
	for y := range z.rows {
		for _, c := range "Zooming in and out, the atlas holds." {
			gly, _, ok := face.Glyph(c, 14)
			gly.At = geom.Point{}
			z.rows[y] = append(z.rows[y], paint.CellPaint{BG: color.NRGBA{A: 255}, FG: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
				Text: ok && c != ' ', Glyph: gly})
		}
	}
	z.pats = boxPatterns(int(z.cw), int(z.ch), true)
	z.rows[0][0].Text, z.rows[0][0].Pattern = false, 7
	return z
}

// paint draws the grid under t.
func (z *zoomGrid) paint(p *paint.Painter, t paint.Transform) {
	end := p.Push(t)
	for y, row := range z.rows {
		p.Cells(geom.Pt(0, float32(y)*z.ch), geom.Sz(z.cw, z.ch), row, z.pats, 1, y, 14, float32(math.Round(float64(z.ascent))))
	}
	end()
}

// zoomRenderer returns a renderer with an atlas of its own, which holds only what the test makes.
func zoomRenderer(t *testing.T) (r *Renderer, done func()) {
	r, done = hiddenGL(t)
	r.shared = &Shared{}
	r.SetText(text.Rendering{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}, false)
	return r, done
}

// sizesMade returns the sizes of glyph the atlas holds.
func sizesMade(r *Renderer) int {
	sizes := map[int32]bool{}
	for key := range r.shared.atlas.slots {
		sizes[key.size] = true
	}
	return len(sizes)
}

// A grid whose zoom changes every frame does not fill the atlas with its glyphs at a new size each frame: it
// stretches the size it has while the zoom moves, made anew now and then, and draws sharp at the size it lands on.
// The same grid shown twice, at rest and in a small copy zooming, as a window and its tile in an overview of all
// windows are, does the same, and keeps its patterns at both sizes rather than making them again each frame.
func TestCellsZoomingDoNotRefillTheAtlasEachFrame(t *testing.T) {
	for _, twice := range []bool{false, true} {
		r, done := zoomRenderer(t)
		z := newZoomGrid()
		w, h := int(benchSize.W), int(benchSize.H)
		now := time.Unix(1000, 0)
		sets := map[cellSizes]bool{}
		frame := func(k float32) {
			var p paint.Painter
			p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
			if twice {
				z.paint(&p, paint.Translate(geom.Pt(20, 300)))
				g, _ := r.cellGridOf(&paint.CellsOp{Size: geom.Sz(z.cw, z.ch), Cells: z.rows[0], Transform: paint.Translate(geom.Pt(20, 300))}, int(z.cw), int(z.ch))
				sets[g.sizes()] = true
			}
			zt := paint.Translate(geom.Pt(20.3, 20.6)).Mul(paint.Scale(k, geom.Point{}))
			z.paint(&p, zt)
			g, _ := r.cellGridOf(&paint.CellsOp{Size: geom.Sz(z.cw, z.ch), Cells: z.rows[0], Transform: zt}, int(z.cw), int(z.ch))
			sets[g.sizes()] = true
			r.clock = func() time.Time { return now }
			r.Draw(p.Ops(), paint.Everything, w, h, 1)
		}
		// A zoom from 1 to a half over twenty frames at 60 a second, and the frame the renderer asks for once it lands.
		const frames = 20
		for i := range frames + 1 {
			frame(1 - 0.5*float32(i)/frames)
			now = now.Add(time.Second / 60)
		}
		if r.SharpAt.IsZero() {
			t.Error("the zoom ended with nothing to draw sharp")
			now = now.Add(bigMaskSettle)
		} else {
			now = r.SharpAt
		}
		r.cellsState.inCells, r.cellsState.spilled = 0, 0
		frame(0.5)
		if r.cellsState.inCells == 0 || r.cellsState.spilled > 0 {
			t.Errorf("drawn twice %v: once the zoom landed, %d glyphs drew in their cells and %d as text, want all in their cells",
				twice, r.cellsState.inCells, r.cellsState.spilled)
		}
		// The size it started at, one made part way, as the zoom took longer than a stretch may last, and the size it
		// landed on; and the size at rest.
		if n := sizesMade(r); n > 3 {
			t.Errorf("drawn twice %v: a zoom over %d frames rasterized its glyphs at %d sizes, want at most 3", twice, frames, n)
		}
		if made := r.cellsState.patternsMade; made > len(sets) {
			t.Errorf("drawn twice %v: the patterns were laid out %d times for %d sets of sizes, want once a set", twice, made, len(sets))
		}
		done()
	}
}

// Cells under a scale land on whole pixels, each as wide as the scale makes a cell, give or take its fraction, with
// no gap between them and none overlapping. Each starts within half a pixel of where it belongs, give or take
// 1/8192 of a pixel a cell, as a cell's size is rounded to that.
func TestScaledCellsLandOnWholePixels(t *testing.T) {
	for _, step := range []float64{6.57, 4.5, 7.74, 9, 11.25, 0.8} {
		for _, corner := range []float64{0, 10.5, 37.3, -3.7} {
			g := cellGrid{o: [2]int32{int32(math.Round(corner * 4096))}, s: [2]int32{int32(math.Round(step * 4096))}}
			sizes := g.sizes()
			for c := range 400 {
				w := g.edge(c+1, 0) - g.edge(c, 0)
				if w != int(math.Floor(step)) && w != int(math.Ceil(step)) {
					t.Fatalf("cell %d of %v pixels from %v is %d wide", c, step, corner, w)
				}
				if step >= 1 && (w < sizes.lo[0] || w > sizes.hi[0]) {
					t.Fatalf("cell %d of %v pixels is %d wide, outside the sizes %v", c, step, w, sizes)
				}
				if e := float64(g.edge(c, 0)); math.Abs(e-(corner+float64(c)*step)) > 0.5+float64(c+1)/8192 {
					t.Fatalf("cell %d of %v pixels from %v starts at %v", c, step, corner, e)
				}
			}
		}
	}
}

// A grid zoomed in far has its patterns made at a size the texture holds, stretched to its cells.
func TestPatternsOfBigCellsFitTheirTexture(t *testing.T) {
	r := &Renderer{}
	r.cellsState.maxTex = 1024
	for _, cell := range [][2]int{{9, 18}, {90, 180}, {400, 800}, {2000, 4000}} {
		pt := &patternTexture{at: cellSizes{lo: cell, hi: [2]int{cell[0] + 1, cell[1] + 1}}}
		r.layOutPatterns(pt, 300)
		rows := (pt.room*pt.made.kinds() + pt.per - 1) / pt.per
		if pt.per*pt.made.hi[0] > 1024 || rows*pt.made.hi[1] > 1024 || pt.room < 300 {
			t.Errorf("cells of %v: patterns %v, %d to a row, room for %d, make a texture %dx%d, past 1024",
				cell, pt.made, pt.per, pt.room, pt.per*pt.made.hi[0], rows*pt.made.hi[1])
		}
		if pt.fit != (cell[0] > 9) {
			t.Errorf("cells of %v: stretched %v", cell, pt.fit)
		}
	}
}

// A mask shrunk or stretched to another size keeps every line of it, sharp.
func TestAResizedMaskKeepsItsLines(t *testing.T) {
	for _, size := range [][2]int{{4, 9}, {6, 13}, {7, 14}, {12, 24}, {20, 41}} {
		for i := range boxArms {
			m := resizeMask(boxMask(i, 9, 18), 9, 18, size[0], size[1])
			w, h := size[0], size[1]
			a := boxArms[i]
			// The middle row and column, at the ends of the cell an arm reaches.
			row, col := -1, -1
			for y := range h {
				if m[y*w+w/2] == 255 && (a[0] || a[1]) && (m[y*w] == 255 || m[y*w+w-1] == 255) {
					row = y
				}
			}
			for x := range w {
				if m[x] == 255 || m[(h-1)*w+x] == 255 {
					col = x
				}
			}
			if (a[0] || a[1]) && row < 0 {
				t.Errorf("%dx%d: pattern %d lost its line across", w, h, i+1)
			}
			if (a[2] || a[3]) && col < 0 {
				t.Errorf("%dx%d: pattern %d lost its line down", w, h, i+1)
			}
			for _, v := range m {
				if v != 0 && v != 255 {
					t.Fatalf("%dx%d: pattern %d has a pixel %d, want lit or dark", w, h, i+1, v)
				}
			}
		}
	}
}

// A grid shown twice, as a pane at rest and a small copy of it in an overview, keeps the sizing of each drawing
// apart. Frames whose damage holds only the copy leave the pane's alone, and the pane, drawn again, is sharp at once
// rather than stretched from the copy's size.
func TestAGridShownTwiceKeepsEachDrawingsSize(t *testing.T) {
	r, done := zoomRenderer(t)
	defer done()
	z := newZoomGrid()
	pane, tile := paint.Translate(geom.Pt(10, 10)), paint.Translate(geom.Pt(20, 400)).Mul(paint.Scale(0.3, geom.Point{}))
	var p paint.Painter
	p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
	z.paint(&p, pane)
	z.paint(&p, tile)
	ops := p.Ops()
	now := time.Unix(1000, 0)
	draw := func(damage geom.Rect) {
		r.clock = func() time.Time { return now }
		r.Draw(ops, damage, int(benchSize.W), int(benchSize.H), 1)
		now = now.Add(time.Second / 60)
	}
	draw(paint.Everything)
	for range 4 {
		// Only the copy changes.
		draw(geom.Rc(0, 380, 400, 100))
	}
	r.cellsState.inCells, r.cellsState.spilled = 0, 0
	draw(geom.Rc(0, 0, 800, 300))
	if r.cellsState.inCells == 0 || r.cellsState.spilled > 0 {
		t.Errorf("the pane drawn again after frames of only its copy drew %d glyphs in their cells and %d stretched as text, want all sharp in their cells",
			r.cellsState.inCells, r.cellsState.spilled)
	}
	if !r.SharpAt.IsZero() {
		t.Error("the pane drawn again asked for a frame to sharpen it, as if it had been drawn stretched")
	}
}

// Textures of patterns the window stopped drawing are let go of, after the time images are.
func TestIdlePatternTexturesAreLetGo(t *testing.T) {
	r, done := zoomRenderer(t)
	defer done()
	z := newZoomGrid()
	now := time.Unix(1000, 0)
	draw := func(k float32) {
		var p paint.Painter
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		z.paint(&p, paint.Scale(k, geom.Point{}))
		r.clock = func() time.Time { return now }
		r.Draw(p.Ops(), paint.Everything, int(benchSize.W), int(benchSize.H), 1)
	}
	draw(0.5)
	draw(1)
	if n := len(r.cellsState.pats); n != 2 {
		t.Fatalf("two sizes of a grid keep %d textures of patterns, want 2", n)
	}
	now = now.Add(imageIdle + time.Second)
	draw(1)
	if n := len(r.cellsState.pats); n != 1 {
		t.Errorf("with one size drawn for %v, %d textures of patterns are kept, want 1", imageIdle, n)
	}
}
