package widget

import (
	"strconv"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
)

// spill is a text or mask op that shows outside where it should: what it is and how far across it shows.
type spill struct {
	what   string
	x0, x1 float32
}

// spillsOutside returns the text and mask ops among ops whose part left showing by the clip layers round them
// reaches across x outside from to to, by more than half a pixel. It takes the ops' transforms as moves only, and a
// run of text as reaching from its first glyph to a third of its size past its last.
func spillsOutside(ops []paint.Op, from, to, top, bottom float32) []spill {
	var out []spill
	clips := []geom.Rect{{Min: geom.Pt(-1e9, -1e9), Max: geom.Pt(1e9, 1e9)}}
	for _, op := range ops {
		var what string
		var r geom.Rect
		switch o := op.(type) {
		case *paint.LayerOp:
			c := clips[len(clips)-1]
			if o.Opts.Clip {
				b := o.Opts.Bounds.Add(geom.Pt(o.Transform.C, o.Transform.F))
				c = geom.Rect{Min: geom.Pt(max(c.Min.X, b.Min.X), max(c.Min.Y, b.Min.Y)),
					Max: geom.Pt(min(c.Max.X, b.Max.X), min(c.Max.Y, b.Max.Y))}
			}
			clips = append(clips, c)
			continue
		case *paint.LayerEndOp:
			clips = clips[:len(clips)-1]
			continue
		case *paint.TextOp:
			if len(o.Glyphs) == 0 {
				continue
			}
			x := o.Transform.C
			y := o.Transform.F
			what = "text"
			r = geom.Rect{Min: geom.Pt(x+o.Glyphs[0].At.X, y), Max: geom.Pt(x+o.Glyphs[len(o.Glyphs)-1].At.X+o.Size/3, y+o.Size)}
		case *paint.MaskOp:
			what = "mask"
			r = o.Rect.Add(geom.Pt(o.Transform.C, o.Transform.F))
		default:
			continue
		}
		c := clips[len(clips)-1]
		x0, x1 := max(r.Min.X, c.Min.X), min(r.Max.X, c.Max.X)
		y0, y1 := max(r.Min.Y, c.Min.Y), min(r.Max.Y, c.Max.Y)
		if x1 <= x0 || y1 <= y0 || y1 <= top || y0 >= bottom {
			continue
		}
		if x0 < from-0.5 || x1 > to+0.5 {
			out = append(out, spill{what + " at y=" + strconv.Itoa(int(r.Min.Y)), x0, x1})
		}
	}
	return out
}

// A grid scrolled sideways draws its titles inside the grid, and a sorted title's arrow inside its column, even
// at the right edge of a column that lines its text up at its right.
func TestADataGridsTitlesKeepToTheGridAndTheirColumns(t *testing.T) {
	cols := make([]GridColumn, 6)
	for i := range cols {
		cols[i] = GridColumn{Title: "Column number " + strconv.Itoa(i), Width: 150, End: true, Sort: 1}
	}
	g := NewDataGrid(cols...)
	g.Row = func(int) (GridRow, bool) { return GridRow{}, true }
	g.rows = 10
	_, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	for _, left := range []float32{0, 75, 150, 500} {
		g.left = left
		run(1)
		ops := painted(g, geom.Sz(400, 300))
		bodyW := g.bodyWidth(nil)
		if s := spillsOutside(ops, 0, bodyW, 0, g.header); len(s) > 0 {
			t.Fatalf("scrolled to %v, the header draws %v outside 0 to %v", left, s, bodyW)
		}
		arrows := 0
		for _, m := range masksIn(ops) {
			if s, ok := m.Shape.(icon.Stroke); !ok || s.Icon != icon.ChevronUp {
				continue
			}
			arrows++
			r := m.Rect.Add(geom.Pt(m.Transform.C, 0))
			c := g.columnAt(r.Center().X)
			if x := g.xs[c][0] - g.left; r.Min.X < x || r.Max.X > x+g.xs[c][1] {
				t.Fatalf("scrolled to %v, an arrow spans %v to %v, past column %d's %v to %v", left, r.Min.X, r.Max.X,
					c, x, x+g.xs[c][1])
			}
		}
		if arrows == 0 {
			t.Fatalf("scrolled to %v, the header draws no arrows", left)
		}
	}
}
