package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// labelPair places two labels 300 wide, one at the top and one 100 below.
type labelPair struct{ a, b *Label }

func (p *labelPair) Children() []gunim.Node { return []gunim.Node{p.a, p.b} }

func (p *labelPair) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for i := range 2 {
		kid := kids.At(i)
		kid.Layout(gunim.Constraints{Max: geom.Sz(300, 100)})
		kid.Place(geom.Pt(0, float32(i)*100))
	}
	return c.Max
}

func (p *labelPair) Paint(pt *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(pt)
	kids.At(1).Paint(pt)
}

// runeAt returns where, in the label's space, the caret before rune i
// sits, halfway down its line.
func runeAt(l *Label, i int) geom.Point {
	_, at := l.laid.p.Caret(i)
	return at.Add(geom.Pt(l.sel.x, l.laid.p.LineHeight/2))
}

func dragSelect(w *gunim.Window, run func(int), from, to geom.Point) {
	w.Input(input.PointerDown{Pos: from, Clicks: 1})
	w.Input(input.PointerMove{Pos: geom.Pt((from.X+to.X)/2, (from.Y+to.Y)/2)})
	w.Input(input.PointerMove{Pos: to})
	w.Input(input.PointerUp{Pos: to})
	run(1)
}

func TestASelectableLabelSelectsAcrossLinesAndCopies(t *testing.T) {
	l := NewLabel("alpha beta gamma delta epsilon zeta")
	l.Selectable = true
	w, run := stage(t, &frame{child: l, size: geom.Sz(120, 200)})
	if len(l.laid.p.Lines) < 2 {
		t.Fatalf("the text took %d lines, want it wrapped", len(l.laid.p.Lines))
	}
	w.Input(input.PointerMove{Pos: runeAt(l, 3)})
	run(1)
	if c := w.Offscreen().Cursor(); c != input.CursorText {
		t.Fatalf("cursor %v over a selectable label, want the I-beam", c)
	}
	end := l.laid.p.Lines[1].Run.Start + 2
	dragSelect(w, run, runeAt(l, 2), runeAt(l, end))
	want := string([]rune(l.Text)[2:end])
	if got := l.SelectedText(); got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	w.Input(input.KeyPress{Key: input.KeyC, Mods: input.ModControl})
	run(1)
	if c, _ := w.Offscreen().Clipboard(); c != want {
		t.Fatalf("clipboard %q, want %q", c, want)
	}
}

func TestADoubleClickSelectsAWordAndATripleClickAll(t *testing.T) {
	l := NewLabel("error in frame parser")
	l.Selectable = true
	w, run := stage(t, &frame{child: l, size: geom.Sz(300, 100)})
	at := runeAt(l, 10)
	w.Input(input.PointerDown{Pos: at, Clicks: 1})
	w.Input(input.PointerUp{Pos: at})
	w.Input(input.PointerDown{Pos: at, Clicks: 2})
	w.Input(input.PointerUp{Pos: at})
	run(1)
	if got := l.SelectedText(); got != "frame" {
		t.Fatalf("a double click selected %q, want frame", got)
	}
	w.Input(input.PointerDown{Pos: at, Clicks: 3})
	w.Input(input.PointerUp{Pos: at})
	run(1)
	if got := l.SelectedText(); got != l.Text {
		t.Fatalf("a triple click selected %q, want all of it", got)
	}
}

func TestAClickElsewhereClearsTheSelection(t *testing.T) {
	a, b := NewLabel("first label"), NewLabel("second label")
	a.Selectable, b.Selectable = true, true
	w, run := stage(t, &frame{child: &labelPair{a, b}, size: geom.Sz(300, 400)})
	dragSelect(w, run, runeAt(a, 0), runeAt(a, 5))
	if got := a.SelectedText(); got != "first" {
		t.Fatalf("selected %q, want first", got)
	}
	w.Input(input.PointerDown{Pos: runeAt(b, 2).Add(geom.Pt(0, 100)), Clicks: 1})
	w.Input(input.PointerUp{Pos: runeAt(b, 2).Add(geom.Pt(0, 100))})
	run(1)
	if s, e := a.Selection(); s != e {
		t.Fatalf("a press in another label left %d..%d selected", s, e)
	}
	dragSelect(w, run, runeAt(b, 0).Add(geom.Pt(0, 100)), runeAt(b, 6).Add(geom.Pt(0, 100)))
	if got := b.SelectedText(); got != "second" {
		t.Fatalf("selected %q, want second", got)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(150, 300), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(150, 300)})
	run(1)
	if s, e := b.Selection(); s != e {
		t.Fatalf("a press outside the labels left %d..%d selected", s, e)
	}
}

func TestALabelThatIsNotSelectableIgnoresDrags(t *testing.T) {
	l := NewLabel("plain text")
	w, run := stage(t, &frame{child: l, size: geom.Sz(300, 100)})
	if err := w.Offscreen().SetClipboard("before"); err != nil {
		t.Fatal(err)
	}
	w.Input(input.PointerMove{Pos: runeAt(l, 1)})
	dragSelect(w, run, runeAt(l, 0), runeAt(l, 5))
	w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl})
	w.Input(input.KeyPress{Key: input.KeyC, Mods: input.ModControl})
	run(1)
	if s, e := l.Selection(); s != e {
		t.Fatalf("a drag selected %d..%d", s, e)
	}
	if c, _ := w.Offscreen().Clipboard(); c != "before" {
		t.Fatalf("clipboard %q, want it untouched", c)
	}
	if c := w.Offscreen().Cursor(); c != input.CursorArrow {
		t.Fatalf("cursor %v, want the arrow", c)
	}
}

// A NoWrap label keeps a long line whole, and scrolls sideways to show
// the rest of it.
func TestANoWrapLabelKeepsItsLinesAndScrollsAcross(t *testing.T) {
	long := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGx0bG9uZ2tleWxvbmdrZXlsb25na2V5bG9uZ2tleQ me@desk"
	wrapped, whole := NewLabel(long), NewLabel(long)
	whole.NoWrap = true
	f := gunim.Frame{Scale: 1}
	a := wrapped.Layout(gunim.Constraints{Max: geom.Sz(150, 400)}, f, gunim.Children{})
	b := whole.Layout(gunim.Constraints{Max: geom.Sz(150, 400)}, f, gunim.Children{})
	if b.H >= a.H {
		t.Fatalf("unwrapped, the label is %v high, and %v wrapped", b.H, a.H)
	}
	w, run := stage(t, &frame{child: whole, size: geom.Sz(150, 100)})
	run(1)
	if whole.over <= 0 {
		t.Fatal("the long line left nothing to scroll to")
	}
	w.Input(input.Scroll{Pos: geom.Pt(20, 5), Delta: geom.Pt(0, -40), Mods: input.ModShift})
	run(1)
	if whole.across <= 0 {
		t.Fatal("Shift and the wheel did not scroll the label sideways")
	}
}
