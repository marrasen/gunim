package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// manyPaletteItems returns n items with titles of a few words each, as a long list of files might have.
func manyPaletteItems(n int) []PaletteItem {
	verbs := []string{"Open", "Split", "Close", "Move", "Paste", "Show", "Find"}
	nouns := []string{"pane", "tab", "window", "file", "folder", "sidebar", "terminal", "editor", "theme", "font", "row"}
	items := make([]PaletteItem, n)
	for i := range items {
		items[i] = PaletteItem{Title: fmt.Sprintf("%s %s %d", verbs[i%len(verbs)], nouns[i%len(nouns)], i)}
	}
	return items
}

// BenchmarkPaletteNarrowingKey times a letter that extends the query, in a palette of 100 000 items, and the
// frame after it.
func BenchmarkPaletteNarrowingKey(b *testing.B) {
	o := &paletteOpener{p: &Palette{Items: manyPaletteItems(100_000)}}
	w := benchStage(b, o, geom.Pt(300, 300))
	w.Input(input.KeyPress{Key: input.KeyF1})
	w.Frame(time.Second / 60)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		o.p.SetQuery("p", o.u)
		b.StartTimer()
		o.p.SetQuery("pa", o.u)
		w.Frame(time.Second / 60)
	}
}

// BenchmarkPaletteNarrowingQuery times a letter that extends a query that has found a few hundred of 100 000 items,
// and the frame after it.
func BenchmarkPaletteNarrowingQuery(b *testing.B) {
	o := &paletteOpener{p: &Palette{Items: manyPaletteItems(100_000)}}
	w := benchStage(b, o, geom.Pt(300, 300))
	w.Input(input.KeyPress{Key: input.KeyF1})
	w.Frame(time.Second / 60)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		o.p.SetQuery("sidebar 12", o.u)
		b.StartTimer()
		o.p.SetQuery("sidebar 123", o.u)
		w.Frame(time.Second / 60)
	}
}

// BenchmarkPaletteFirstKey times the first letter typed in a palette of 100 000 items, and the frame after it.
func BenchmarkPaletteFirstKey(b *testing.B) {
	o := &paletteOpener{p: &Palette{Items: manyPaletteItems(100_000)}}
	w := benchStage(b, o, geom.Pt(300, 300))
	w.Input(input.KeyPress{Key: input.KeyF1})
	w.Frame(time.Second / 60)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		o.p.SetQuery("", o.u)
		b.StartTimer()
		o.p.SetQuery("p", o.u)
		w.Frame(time.Second / 60)
	}
}

// BenchmarkPalettePointerMove times the pointer moving over a row of a palette showing 100 000 items.
func BenchmarkPalettePointerMove(b *testing.B) {
	o := &paletteOpener{p: &Palette{Items: manyPaletteItems(100_000)}}
	w := benchStage(b, o, geom.Pt(300, 300))
	w.Input(input.KeyPress{Key: input.KeyF1})
	for range 30 {
		w.Frame(time.Second / 60)
	}
	var rows []*paletteRow
	for _, r := range o.p.card.list.live {
		if pr, ok := r.child.(*paletteRow); ok {
			rows = append(rows, pr)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		rows[i%len(rows)].Handle(input.PointerMove{}, o.u)
	}
}

// newRoomPalette opens a palette of n items from the top of a window whose screen's work area is area.
func newRoomPalette(t *testing.T, n int, area geom.Rect) (o *paletteOpener, run func(int)) {
	t.Helper()
	o = &paletteOpener{p: &Palette{Items: manyPaletteItems(n)}}
	var w *gunim.Window
	w, run = stage(t, o)
	w.Offscreen().SetWorkArea(area)
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	return o, run
}

// On a screen 360 tall, a palette opened at y=40 keeps its card on the screen, every frame of its opening, and
// scrolls its list to the highlight.
func TestAPaletteKeepsToAShortScreen(t *testing.T) {
	o, run := newRoomPalette(t, 40, geom.Rc(0, 0, 800, 360))
	for i := range 40 {
		run(1)
		pw := o.p.popup.Offscreen()
		if bottom := pw.Anchor().Max.Y + o.p.card.margin + o.p.card.card.Max.Y; bottom > 360 {
			t.Fatalf("frame %d: the card reaches y=%v, past the screen's bottom, 360", i, bottom)
		}
		if h := pw.Size().H; h > 360-40 {
			t.Fatalf("frame %d: the window is %v tall, more than the 320 below its anchor", i, h)
		}
	}
	for range 30 {
		o.p.card.move(1, o.u)
		run(1)
	}
	run(30)
	if r, ok := o.p.card.list.live[o.p.keyOf(o.p.card.hotIndex())]; !ok || r == nil {
		t.Fatalf("moved down to %d, its row is not built", o.p.card.hot)
	}
}

// On a phone 360 wide, a palette keeps to the screen's width, every frame, and cuts long titles short.
func TestAPaletteKeepsToANarrowScreen(t *testing.T) {
	o, run := newRoomPalette(t, 40, geom.Rc(0, 0, 360, 740))
	for i := range 40 {
		run(1)
		pw := o.p.popup.Offscreen()
		if w := pw.Size().W; w > 360 {
			t.Fatalf("frame %d: the window is %v wide, on a screen 360 wide", i, w)
		}
		if x := pw.Anchor().Min.X; x < 0 || x+pw.Size().W > 360 {
			t.Fatalf("frame %d: the window spans x=%v to %v, off the screen", i, x, x+pw.Size().W)
		}
	}
}
