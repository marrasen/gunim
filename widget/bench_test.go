package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
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
