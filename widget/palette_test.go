package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// paletteOpener is a focusable block that opens its palette on F1.
type paletteOpener struct {
	p      *Palette
	picked []int
}

func (o *paletteOpener) Focusable() bool { return true }

func (o *paletteOpener) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok && k.Key == input.KeyF1 {
		o.p.Open(o, geom.Rc(0, 40, 600, 0), u)
		return true
	}
	return false
}

func (o *paletteOpener) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (o *paletteOpener) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

func newPaletteStage(t *testing.T) (*gunim.Window, *paletteOpener, func(int)) {
	t.Helper()
	o := &paletteOpener{}
	o.p = &Palette{
		Items: []PaletteItem{
			{Title: "Close pane", Hint: "Ctrl+Shift+W"},
			{Title: "Split right", Hint: "Ctrl+Shift+D"},
			{Title: "Split down", Hint: "Ctrl+Shift+E"},
			{Title: "Paste"},
		},
		Pick: func(i int, _ *gunim.UI) { o.picked = append(o.picked, i) },
	}
	w, run := stage(t, o)
	run(1)
	return w, o, run
}

func focusOpener(w *gunim.Window, run func(int)) {
	w.Input(input.PointerDown{Pos: geom.Pt(300, 300), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(300, 300), Time: time.Now()})
	run(1)
}

func TestAPaletteFindsWhatIsTypedAndPicksIt(t *testing.T) {
	w, o, run := newPaletteStage(t)
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(20)
	if !o.p.IsOpen() || len(o.p.card.found) != 4 {
		t.Fatalf("open %v with %d items, want open with all 4", o.p.IsOpen(), len(o.p.card.found))
	}
	w.Input(input.TextInput{Text: "sd"})
	run(20)
	if f := o.p.card.found; len(f) != 1 || f[0].Index != 2 {
		t.Fatalf("typing sd found %v, want Split down alone", f)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(o.picked) != 1 || o.picked[0] != 2 {
		t.Fatalf("picked %v, want item 2", o.picked)
	}
	if o.p.IsOpen() {
		t.Fatal("the palette stayed open after a pick")
	}
	// The keyboard went back to the node that opened it.
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(1)
	if !o.p.IsOpen() {
		t.Fatal("F1 no longer reached the opener after the palette closed")
	}
}

func TestThePaletteKeysMoveTheHighlightAndEscapeCloses(t *testing.T) {
	w, o, run := newPaletteStage(t)
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(20)
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyUp})
	run(5)
	if o.p.card.hot != 1 {
		t.Fatalf("the highlight is on %d, want 1", o.p.card.hot)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(20)
	if o.p.IsOpen() || len(o.picked) != 0 {
		t.Fatalf("after Escape: open %v, picked %v; want closed with nothing picked", o.p.IsOpen(), o.picked)
	}
}

func TestThePaletteShrinksSmoothlyAsTheQueryNarrows(t *testing.T) {
	w, o, run := newPaletteStage(t)
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(40)
	c := o.p.card
	full := c.height.Value()
	w.Input(input.TextInput{Text: "sp"})
	last := full
	for i := range 40 {
		run(1)
		h := c.height.Value()
		if h > last+0.01 {
			t.Fatalf("frame %d: the list grew from %v to %v while it shrank", i, last, h)
		}
		last = h
	}
	step := PaletteRowHeight.Default() + ListSpacing.Default()
	if full != 4*step || last != 3*step {
		t.Fatalf("the list went from %v to %v, want %v to %v", full, last, 4*step, 3*step)
	}
}
