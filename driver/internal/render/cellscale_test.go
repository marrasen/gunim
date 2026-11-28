//go:build linux || windows || darwin

package render

import (
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

// A table in box drawing drawn under a zoom shows every one of its lines whole, at every scale, below one and
// above: none is lost between the pixels the cells land on. With patterns made at the size shown, the lines are
// sharp, on whole pixels, and every line is as thick as every other.
func TestBoxDrawingUnderAZoomDrawsEveryLine(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	const cw, ch = 9, 18
	w, h := int(benchSize.W), int(benchSize.H)
	for _, sized := range []bool{true, false} {
		for _, k := range []float32{0.5, 0.73, 0.86, 1, 1.25, 1.6} {
			pats := boxPatterns(cw, ch, sized)
			var p paint.Painter
			p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
			// Placed off the pixel grid, as a view zoomed about a point is.
			end := p.Push(paint.Translate(geom.Pt(37.3, 21.6)).Mul(paint.Scale(k, geom.Point{})))
			for y, row := range boxTable {
				cells := make([]paint.CellPaint, len(row))
				for x, n := range row {
					cells[x] = paint.CellPaint{BG: color.NRGBA{A: 255}, FG: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Pattern: n}
				}
				p.Cells(geom.Pt(0, float32(y*ch)), geom.Sz(cw, ch), cells, pats, 1, y, 0, 0)
			}
			end()
			r.Draw(p.Ops(), paint.Everything, w, h, 1)
			pix := readFrame(r)
			lit := func(x, y int) bool { return pix[(y*w+x)*4] > 200 }
			ox, oy := 37.3, 21.6
			kk := float64(k)
			// A point a share f of the way across cell x, or down row y, in device pixels.
			across := func(x int, f float64) int { return int(ox + (float64(x)+f)*cw*kk) }
			down := func(y int, f float64) int { return int(oy + (float64(y)+f)*ch*kk) }
			// Each line across the table, rows 0, 2 and 4: some row of pixels in its row of cells is lit all the way
			// from the first column to the last.
			thick := make([]int, 0, 6)
			for _, row := range []int{0, 2, 4} {
				y0, y1 := int(oy+float64(row)*ch*kk), int(math.Ceil(oy+float64(row+1)*ch*kk))
				whole, n := false, 0
				for y := y0; y < y1; y++ {
					all := true
					for x := across(0, 0.75); x <= across(6, 0.25); x++ {
						all = all && lit(x, y)
					}
					if all {
						whole = true
						n++
					}
				}
				if !whole {
					t.Errorf("sized %v, scale %v: the line across row %d is broken or missing", sized, k, row)
				}
				thick = append(thick, n)
			}
			// Each line down the table, columns 0, 3 and 6, likewise.
			for _, col := range []int{0, 3, 6} {
				x0, x1 := int(ox+float64(col)*cw*kk), int(math.Ceil(ox+float64(col+1)*cw*kk))
				whole, n := false, 0
				for x := x0; x < x1; x++ {
					all := true
					for y := down(0, 0.75); y <= down(4, 0.25); y++ {
						all = all && lit(x, y)
					}
					if all {
						whole = true
						n++
					}
				}
				if !whole {
					t.Errorf("sized %v, scale %v: the line down column %d is broken or missing", sized, k, col)
				}
				thick = append(thick, n)
			}
			if !sized {
				continue
			}
			for i, n := range thick[:3] {
				if n != thick[0] {
					t.Errorf("scale %v: the line across row %d is %d pixels thick, and the first %d", k, 2*i, n, thick[0])
				}
			}
			for i, n := range thick[3:] {
				if n != thick[3] {
					t.Errorf("scale %v: the line down column %d is %d pixels thick, and the first %d", k, 3*i, n, thick[3])
				}
			}
			// Sharp: every pixel of the table is lit or dark, none part way.
			soft := 0
			for y := int(oy); y < int(oy+5*ch*kk); y++ {
				for x := int(ox); x < int(ox+7*cw*kk); x++ {
					if v := pix[(y*w+x)*4]; v > 40 && v < 215 {
						soft++
					}
				}
			}
			if soft > 0 {
				t.Errorf("scale %v: %d pixels of the table are neither lit nor dark, want the lines on whole pixels", k, soft)
			}
		}
	}
}

// A grid whose zoom changes every frame does not fill the atlas with its glyphs at a new size each frame: it
// stretches the size it has while the zoom moves, made anew now and then, and draws sharp at the size it lands on.
func TestCellsZoomingDoNotRefillTheAtlasEachFrame(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	r.SetText(text.Rendering{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}, false)
	face := text.GoMono(false, false)
	const size = 14
	_, adv, _ := face.Glyph('M', size)
	ascent, descent, _ := face.Metrics(size)
	cw, ch := float32(math.Ceil(float64(adv))), float32(math.Ceil(float64(ascent+descent)))
	const line = "Zooming in and out, the atlas holds."
	var rows [6][]paint.CellPaint
	for y := range rows {
		for _, c := range line {
			gly, _, ok := face.Glyph(c, size)
			gly.At = geom.Point{}
			rows[y] = append(rows[y], paint.CellPaint{BG: color.NRGBA{A: 255}, FG: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
				Text: ok && c != ' ', Glyph: gly})
		}
	}
	faceID := rows[0][0].Glyph.Face
	pats := &paint.Patterns{W: int(cw), H: int(ch)}
	before := map[glyphKey]bool{}
	for key := range testShared.atlas.slots {
		before[key] = true
	}
	w, h := int(benchSize.W), int(benchSize.H)
	now := time.Unix(1000, 0)
	frame := func(k float32) {
		var p paint.Painter
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		end := p.Push(paint.Translate(geom.Pt(20, 20)).Mul(paint.Scale(k, geom.Point{})))
		for y, row := range rows {
			p.Cells(geom.Pt(0, float32(y)*ch), geom.Sz(cw, ch), row, pats, 1, y, size, float32(math.Round(float64(ascent))))
		}
		end()
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
		t.Errorf("once the zoom landed, %d glyphs drew in their cells and %d as text, want all in their cells, sharp",
			r.cellsState.inCells, r.cellsState.spilled)
	}
	sizes := map[int32]bool{}
	for key := range testShared.atlas.slots {
		if !before[key] && key.face == faceID {
			sizes[key.size] = true
		}
	}
	// The size it started at, one made part way, as the zoom took longer than a stretch may last, and the size it
	// landed on.
	if len(sizes) > 3 {
		t.Errorf("a zoom over %d frames rasterized its glyphs at %d sizes, want at most 3", frames, len(sizes))
	}
}

// Cells under a scale land on whole pixels, each as wide as the scale makes a cell, give or take its fraction, with
// no gap between them and none overlapping.
func TestScaledCellsLandOnWholePixels(t *testing.T) {
	for _, step := range []float32{6.57, 4.5, 7.74, 9, 11.25, 0.8} {
		g := cellGrid{step: [2]float32{step, step}}
		sizes := g.sizes()
		for c := range 400 {
			w := g.edge(c+1, 0) - g.edge(c, 0)
			if w != int(math.Floor(float64(step))) && w != int(math.Ceil(float64(step))) {
				t.Fatalf("cell %d of %v pixels is %d wide", c, step, w)
			}
			if step >= 1 && (w < sizes.lo[0] || w > sizes.hi[0]) {
				t.Fatalf("cell %d of %v pixels is %d wide, outside the sizes %v", c, step, w, sizes)
			}
		}
		if got, want := g.edge(400, 0), int(math.Floor(float64(400*step)+0.5)); got != want {
			t.Errorf("400 cells of %v pixels end at %d, want %d", step, got, want)
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
