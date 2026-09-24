package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type pressed struct{ N int }

func TestAButtonFiresOnlyWhenReleasedOverIt(t *testing.T) {
	b := NewButton("Go")
	b.On = pressed{1}
	w, run := stage(t, &frame{child: b, size: geom.Sz(100, 36)})
	drain := func() int {
		n := 0
		for {
			select {
			case <-w.Client().Intents():
				n++
			default:
				return n
			}
		}
	}

	w.Input(input.PointerDown{Pos: geom.Pt(20, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 18)})
	w.Input(input.PointerUp{Pos: geom.Pt(300, 18)})
	run(1)
	if n := drain(); n != 0 {
		t.Fatalf("released outside, the button fired %d times", n)
	}

	w.Input(input.PointerDown{Pos: geom.Pt(20, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(30, 18)})
	w.Input(input.PointerUp{Pos: geom.Pt(30, 18)})
	run(1)
	if n := drain(); n != 1 {
		t.Fatalf("dragged out and back, then released over it, the button fired %d times, want 1", n)
	}
}

func tab(w *gunim.Window, run func(int), mods input.Mods) {
	w.Input(input.KeyPress{Key: input.KeyTab, Mods: mods})
	run(1)
}

func TestTabVisitsFocusableNodesInOrder(t *testing.T) {
	a, field, c := NewButton("A"), NewTextField(), NewButton("C")
	label := NewLabel("not focusable")
	col := Column(a, label, field, c)
	var focused gunim.Node
	w, run := stage(t, &frame{child: col, size: geom.Sz(300, 400)})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { focused = u.Focused() })
	check := func(want gunim.Node) {
		t.Helper()
		if err := w.Client().Patch("stage", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if focused != want {
			t.Fatalf("focus is on %T %p, want %T %p", focused, focused, want, want)
		}
	}

	tab(w, run, 0)
	check(a)
	tab(w, run, 0)
	check(field) // the label is skipped
	tab(w, run, 0)
	check(c) // Tab passes through the field
	tab(w, run, 0)
	check(a) // and wraps
	tab(w, run, input.ModShift)
	check(c)

	// A click on something that takes no focus drops it.
	w.Input(input.PointerDown{Pos: geom.Pt(250, 390)})
	check(nil)
}

type probeFocus struct{}

func TestTabBringsAFocusedNodeIntoView(t *testing.T) {
	buttons := make([]gunim.Node, 0, 20)
	for range 20 {
		buttons = append(buttons, NewButton("Row"))
	}
	sc := NewScroll(Column(buttons...))
	w, run := stage(t, &frame{child: sc, size: geom.Sz(200, 200)})
	for range 10 {
		tab(w, run, 0)
	}
	run(120)
	// The tenth button's top is 9 * (36 + 8) = 396 down the content;
	// its bottom must now show within 200.
	if off := sc.Offset(); off < 396+36-200 || off > 396 {
		t.Fatalf("offset %v; the tenth button, at 396..432, is out of view", off)
	}
}

func TestADialogKeepsTabAmongItsButtons(t *testing.T) {
	d := NewDialog("Sure?")
	w, run := stage(t, d)
	var focused gunim.Node
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { focused = u.Focused() })
	check := func(want gunim.Node) {
		t.Helper()
		if err := w.Client().Patch("stage", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if focused != want {
			t.Fatalf("focus is on %T %p, want %T %p", focused, focused, want, want)
		}
	}
	if err := w.Client().Focus("stage"); err != nil {
		t.Fatal(err)
	}
	run(30)
	tab(w, run, 0)
	check(d.cancel)
	tab(w, run, 0)
	check(d.ok)
	tab(w, run, 0)
	check(d.cancel)

	// A click on the panel's empty space keeps focus in the dialog, on
	// the dialog itself, where Escape reaches it.
	w.Input(input.PointerDown{Pos: geom.Pt(400, 300)})
	check(d)
}
