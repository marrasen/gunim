package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// newKeyedList mounts a button over a scrolled list of n 40-tall rows that can be clicked, and returns what reports
// the node with the keyboard.
func newKeyedList(t *testing.T, n int) (w *gunim.Window, l *List, sc *Scroll, run func(int), focused func() gunim.Node) {
	t.Helper()
	l = NewList()
	l.OnClick = func(k Key) gunim.Intent { return rowClicked{k} }
	sc = NewScroll(l)
	col := Column(NewButton("Before"), sc).Grow(sc, 1)
	col.Cross = CrossStretch
	w = gunimtest.New(t, geom.Sz(300, 400), nil)
	var has gunim.Node
	gunim.RegisterView(w, "l", func(shownItems) gunim.Node { return col },
		func(_ gunim.Node, s shownItems, u *gunim.UI) {
			Sync(l, u, s.Items, func(k Key) Key { return k },
				func(k Key) *block { return &block{key: k, h: 40} }, nil)
		})
	gunim.RegisterPatch(w, "l", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { has = u.Focused() })
	if err := w.Client().Mount(gunim.Root, "l", "l", shownItems{keys(n)}); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(60)
	focused = func() gunim.Node {
		t.Helper()
		if err := w.Client().Patch("l", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		return has
	}
	return w, l, sc, run, focused
}

func TestTabReachesAListAndTheArrowsMoveItsCursor(t *testing.T) {
	w, l, sc, run, focused := newKeyedList(t, 20)
	tab(w, run, 0)
	tab(w, run, 0)
	if f := focused(); f != l {
		t.Fatalf("two Tabs put the keyboard on %T, want the list", f)
	}
	if k, ok := l.Cursor(); !ok || k != "0" {
		t.Fatalf("the focused list's cursor is on %q, want the first row", k)
	}
	press := func(k input.Key) {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	press(input.KeyDown)
	press(input.KeyDown)
	press(input.KeyUp)
	if k, _ := l.Cursor(); k != "1" {
		t.Fatalf("Down twice then Up put the cursor on %q, want row 1", k)
	}
	press(input.KeyEnter)
	if got := sent(w); len(got) != 1 || got[0] != (rowClicked{"1"}) {
		t.Fatalf("Enter sent %v, want a click on row 1", got)
	}

	// End goes to the last row, and scrolls it into view.
	press(input.KeyEnd)
	run(120)
	if k, _ := l.Cursor(); k != "19" {
		t.Fatalf("End put the cursor on %q, want the last row", k)
	}
	if off, bottom := sc.Offset(), float32(19*46+40); off+sc.viewport < bottom-0.5 {
		t.Fatalf("scrolled to %v, which leaves the last row, ending at %v, out of view", off, bottom)
	}
	press(input.KeyHome)
	run(120)
	if k, _ := l.Cursor(); k != "0" || sc.Offset() > 0.5 {
		t.Fatalf("Home put the cursor on %q, scrolled to %v; want the first row, in view", k, sc.Offset())
	}
	if r := l.ring.Value(); r < 0.99 {
		t.Fatalf("the focused list's ring is %v grown, want all the way", r)
	}
}

func TestAClickPutsAListsCursorOnItsRow(t *testing.T) {
	w, l, _, run, _ := newKeyedList(t, 5)
	// The button over the list is 36 tall, with the column's gap under it.
	top := ButtonHeight.Default() + Gap.Default()
	w.Input(input.PointerDown{Pos: geom.Pt(100, top+46*2+20), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(100, top+46*2+20)})
	run(1)
	if k, _ := l.Cursor(); k != "2" {
		t.Fatalf("a click on row 2 left the cursor on %q", k)
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(1)
	if got := sent(w); len(got) != 2 || got[1] != (rowClicked{"3"}) {
		t.Fatalf("a click on row 2, Down and Space sent %v, want clicks on rows 2 and 3", got)
	}
}

func TestAListThatSkipsFocusIsClickedButLeavesTheKeys(t *testing.T) {
	w, l, _, run, focused := newKeyedList(t, 5)
	l.SkipFocus = true
	tab(w, run, 0)
	tab(w, run, 0)
	if f := focused(); f == l {
		t.Fatal("Tab put the keyboard on a list with SkipFocus")
	}
	top := ButtonHeight.Default() + Gap.Default()
	w.Input(input.PointerDown{Pos: geom.Pt(100, top+46*2+20), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(100, top+46*2+20)})
	run(1)
	if f := focused(); f == l {
		t.Fatal("a click put the keyboard on a list with SkipFocus")
	}
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (rowClicked{"2"}) {
		t.Fatalf("a click on row 2 and Space sent %v, want the click alone", got)
	}
}

func TestTabReachesARowsControlsInTheOrderShown(t *testing.T) {
	l := NewList()
	buttons := map[Key]*Button{}
	w := gunimtest.New(t, geom.Sz(300, 400), nil)
	var has gunim.Node
	gunim.RegisterView(w, "l", func(shownItems) gunim.Node { return l },
		func(_ gunim.Node, s shownItems, u *gunim.UI) {
			Sync(l, u, s.Items, func(k Key) Key { return k }, func(k Key) *Button {
				buttons[k] = NewButton(string(k))
				return buttons[k]
			}, nil)
		})
	gunim.RegisterPatch(w, "l", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { has = u.Focused() })
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	if err := w.Client().Mount(gunim.Root, "l", "l", shownItems{[]Key{"b"}}); err != nil {
		t.Fatal(err)
	}
	run(30)
	// a comes after b, and is shown before it.
	if err := w.Client().Update("l", shownItems{[]Key{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	run(60)
	var got []Key
	for range 2 {
		tab(w, run, 0)
		if err := w.Client().Patch("l", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		for k, b := range buttons {
			if has == b {
				got = append(got, k)
			}
		}
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Tab went to %v, want a then b, as shown", got)
	}
}
