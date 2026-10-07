package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// newLongPicker opens a drop-down of 60 items, with item sel chosen, and returns it with its menu.
func newLongPicker(t *testing.T, sel int) (*Dropdown, *Menu, func(int)) {
	t.Helper()
	items := make([]string, 60)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d", i)
	}
	d := NewDropdown(items...)
	d.Selected = sel
	w, run := stage(t, &frame{child: d, size: geom.Sz(200, 36)})
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18)})
	run(1)
	if !d.IsOpen() {
		t.Fatal("a click did not open the list")
	}
	return d, d.menu, run
}

// overshoot is how far past its target a scroll's spring may carry the rows on a scroll of d: the quick spring's
// damping of 0.85 carries it about 0.6% further.
func overshoot(d float32) float32 { return max(1, 0.01*d) }

// rowShown fails unless row i lies wholly inside the menu's card.
func rowShown(t *testing.T, m *Menu, i int, when string) {
	t.Helper()
	r := m.RowRect(i)
	if r.Min.Y < m.card.Min.Y-0.5 || r.Max.Y > m.card.Max.Y+0.5 {
		t.Fatalf("%s: row %d at %v lies outside the card %v", when, i, r, m.card)
	}
}

func TestALongMenuKeepsToItsRoomAndScrolls(t *testing.T) {
	_, m, run := newLongPicker(t, 0)
	run(1)
	if h := m.card.Size().H; h > 480 {
		t.Fatalf("the menu is %v tall, past the drop-down's 480", h)
	}
	if !m.scroll.scrollable() {
		t.Fatal("60 items in at most 480 do not scroll")
	}
}

func TestAMenuOpensWithTheChosenItemInView(t *testing.T) {
	_, m, run := newLongPicker(t, 45)
	for f := range 20 {
		rowShown(t, m, 45, fmt.Sprintf("frame %d of opening", f))
		run(1)
	}
}

func TestTheKeysKeepTheHighlightInView(t *testing.T) {
	_, m, run := newLongPicker(t, 0)
	m.Key(input.KeyPress{Key: input.KeyEnd}, nil)
	end := m.scroll.end()
	last := float32(0)
	for f := range 40 {
		run(1)
		at := m.scroll.Offset()
		// The spring may overshoot the end a little, as every scroll's does.
		if at < last-overshoot(end) || at > end+overshoot(end) {
			t.Fatalf("frame %d: offset %v after %v, want it rising to %v", f, at, last, end)
		}
		last = at
	}
	rowShown(t, m, 59, "after End")
	if m.Highlighted() != 59 {
		t.Fatalf("End highlighted %d", m.Highlighted())
	}
	// Down goes round to the first item, and the menu scrolls back up to it.
	m.Key(input.KeyPress{Key: input.KeyDown}, nil)
	run(40)
	rowShown(t, m, 0, "after Down from the last")
}

func TestTheWheelScrollsAMenuAndAClickPicksTheRowUnderIt(t *testing.T) {
	d, m, run := newLongPicker(t, 0)
	mid := m.card.Center()
	d.popup.Input(input.Scroll{Pos: mid, Delta: geom.Pt(0, -200), Time: time.Now()})
	last := float32(0)
	for f := range 40 {
		run(1)
		at := m.scroll.Offset()
		if at < last-overshoot(200) || at > m.scroll.end()+overshoot(200) {
			t.Fatalf("frame %d: offset %v after %v", f, at, last)
		}
		last = at
	}
	if last < 150 {
		t.Fatalf("the wheel scrolled the menu %v, want 200", last)
	}
	want := m.rowAt(mid)
	if want < 3 {
		t.Fatalf("after the wheel, row %d is under the pointer; want one further down", want)
	}
	left := geom.Pt(m.card.Min.X+20, mid.Y)
	d.popup.Input(input.PointerMove{Pos: left, Time: time.Now()})
	d.popup.Input(input.PointerDown{Pos: left, Clicks: 1, Time: time.Now()})
	d.popup.Input(input.PointerUp{Pos: left, Time: time.Now()})
	run(2)
	if d.Selected != want {
		t.Fatalf("a click on row %d picked %d", want, d.Selected)
	}
}

// fadeIn returns the fade of the layer in ops that fades, and false where none does.
func fadeIn(ops []paint.Op) (geom.Insets, bool) {
	for _, op := range ops {
		if l, ok := op.(*paint.LayerOp); ok && l.Opts.Fade != (geom.Insets{}) {
			return l.Opts.Fade, true
		}
	}
	return geom.Insets{}, false
}

func TestALongMenuFadesWhereMoreRowsLiePastItsEdge(t *testing.T) {
	_, m, run := newLongPicker(t, 0)
	run(1)
	full := ScrollFade.Default()
	fade, ok := fadeIn(painted(m, geom.Sz(m.card.Max.X, m.card.Max.Y)))
	if !ok || fade.Top != 0 || fade.Bottom != full {
		t.Fatalf("at the top the menu fades by %v (%v), want only at the bottom, by %v", fade, ok, full)
	}
	m.Key(input.KeyPress{Key: input.KeyEnd}, nil)
	for f := range 40 {
		run(1)
		fade, _ = fadeIn(painted(m, geom.Sz(m.card.Max.X, m.card.Max.Y)))
		at, end := m.scroll.Offset(), m.scroll.end()
		if fade.Top != fadeFor(at, full) || fade.Bottom != fadeFor(end-at, full) {
			t.Fatalf("frame %d: scrolled %v of %v, the menu fades by %v", f, at, end, fade)
		}
	}
	if fade.Top != full || fade.Bottom > 0.5 {
		t.Fatalf("at the end the menu fades by %v, want only at the top", fade)
	}
}

func TestAShortMenuDoesNotFade(t *testing.T) {
	m := NewMenu("One", "Two")
	m.Layout(gunim.Constraints{Max: geom.Sz(400, 400)}, gunim.Frame{Scale: 1}, gunim.Children{})
	if fade, ok := fadeIn(painted(m, geom.Sz(m.card.Max.X, m.card.Max.Y))); ok {
		t.Fatalf("a menu that fits fades by %v", fade)
	}
}
