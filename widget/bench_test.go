package widget

import (
	"fmt"
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// BenchmarkFrame times one frame of a gallery like the widgets
// example's, laid out and painted in full: forty cards of wrapped text
// and a button, in a scroll view.
func BenchmarkFrame(b *testing.B) {
	list := Column()
	for i := range 40 {
		label := NewLabel(fmt.Sprintf("Item %d. A line of text in a card, long enough to wrap when the window is narrow.", i+1))
		row := Row(label, NewButton("Open")).Grow(label, 1)
		row.Cross = CrossCenter
		list.kids = append(list.kids, NewCard(row))
	}
	list.Cross = CrossStretch
	w := gunim.NewOffscreen(geom.Sz(720, 560), nil)
	gunim.RegisterView(w, "g", func(struct{}) gunim.Node { return NewPad(NewScroll(list)) }, nil)
	if err := w.Client().Mount(gunim.Root, "g", "g", nil); err != nil {
		b.Fatal(err)
	}
	for range 120 {
		w.Frame(time.Second / 60)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		// A pointer move asks for a frame, as hovering does.
		w.Input(input.PointerMove{Pos: geom.Pt(float32(100+i%2), 100)})
		w.Frame(time.Second / 60)
	}
	b.ReportMetric(float64(len(w.Offscreen().Ops())), "ops/frame")
}

// BenchmarkHalfBlockFrame times one frame of an animation drawn in half
// blocks, as termflix draws, at 250 by 75 cells: every cell in new
// colours, set and painted.
func BenchmarkHalfBlockFrame(b *testing.B) {
	const cols, rows = 250, 75
	g := NewCellGrid()
	g.Size = 14
	g.Resize(cols, rows)
	g.measure(14, 1.5)
	row := make([]Cell, cols)
	var p paint.Painter
	b.ReportAllocs()
	for i := range b.N {
		for y := range rows {
			for x := range row {
				v := uint8(x + y + i)
				row[x] = Cell{Rune: '▀', FG: color.NRGBA{R: v, G: 90, B: 160, A: 255}, BG: color.NRGBA{R: 40, G: v, B: 160, A: 255}}
			}
			g.SetRow(y, row)
		}
		p.Reset()
		g.Paint(&p, gunim.Frame{Scale: 1.5}, geom.Sz(cols*10, rows*20), gunim.Children{})
	}
	b.ReportMetric(float64(len(p.Ops())), "ops/frame")
}

// BenchmarkStillGridFrame times a frame in which a 250 by 75 grid of
// half blocks has not changed, as while something beside it animates.
func BenchmarkStillGridFrame(b *testing.B) {
	const cols, rows = 250, 75
	g := NewCellGrid()
	g.Size = 14
	g.Resize(cols, rows)
	g.measure(14, 1.5)
	row := make([]Cell, cols)
	for y := range rows {
		for x := range row {
			row[x] = Cell{Rune: '▀', FG: color.NRGBA{R: uint8(x), G: 90, B: 160, A: 255}, BG: color.NRGBA{R: 40, G: uint8(y), B: 160, A: 255}}
		}
		g.SetRow(y, row)
	}
	var p paint.Painter
	f, box := gunim.Frame{Scale: 1.5}, geom.Sz(cols*10, rows*20)
	b.ReportAllocs()
	for range b.N {
		p.Reset()
		g.Paint(&p, f, box, gunim.Children{})
		_ = p.Damage()
	}
	b.ReportMetric(float64(len(p.Ops())), "ops/frame")
}
