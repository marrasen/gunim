package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type reordered struct{ Keys []Key }

// shownItems is the state of a view holding a list.
type shownItems struct{ Items []Key }

// newReorderList mounts a list of five 40-tall rows, which the pointer
// can reorder. Rows sit 46 apart.
func newReorderList(t *testing.T) (*gunim.Window, *List, func(int)) {
	t.Helper()
	l := NewList()
	l.Reorder = func(keys []Key) gunim.Intent { return reordered{keys} }
	w := gunim.NewOffscreen(geom.Sz(300, 400), nil)
	gunim.RegisterView(w, "l", func(shownItems) gunim.Node { return l },
		func(_ gunim.Node, s shownItems, u *gunim.UI) {
			Sync(l, u, s.Items, func(k Key) Key { return k },
				func(k Key) *block { return &block{key: k, h: 40} }, nil)
		})
	if err := w.Client().Mount(gunim.Root, "l", "l", shownItems{keys(5)}); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(60)
	return w, l, run
}

func TestDraggingARowReordersTheList(t *testing.T) {
	w, l, run := newReorderList(t)
	// Pick up row 0 by its middle and carry it down past rows 1 and 2,
	// its middle short of row 3's.
	w.Input(input.PointerDown{Pos: geom.Pt(100, 20), Clicks: 1})
	for y := float32(20); y <= 110; y += 10 {
		w.Input(input.PointerMove{Pos: geom.Pt(100, y)})
		run(1)
	}
	// Row 1 has moved up into the gap row 0 left.
	if y := l.rows["1"].y.Value(); y >= 46 {
		t.Fatalf("row 1 still at %v while row 0 is carried past it", y)
	}
	if y := l.rows["0"].y.Value(); y != 90 {
		t.Fatalf("the carried row is at %v, want 90, under the pointer", y)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(100, 110)})
	run(120)
	want := []Key{"1", "2", "0", "3", "4"}
	if !slices.Equal(l.Keys(), want) {
		t.Fatalf("order %v, want %v", l.Keys(), want)
	}
	if y := l.rows["0"].y.Value(); y != 92 {
		t.Fatalf("the dropped row settled at %v, want 92", y)
	}
	got := sent(w)
	if len(got) != 1 {
		t.Fatalf("intents %v, want the new order once", got)
	}
	if r, ok := got[0].(reordered); !ok || !slices.Equal(r.Keys, want) {
		t.Fatalf("intents %v, want the new order once", got)
	}
}

func TestAClickOnARowLeavesTheOrder(t *testing.T) {
	w, l, run := newReorderList(t)
	w.Input(input.PointerDown{Pos: geom.Pt(100, 20), Clicks: 1})
	w.Input(input.PointerMove{Pos: geom.Pt(100, 22)})
	w.Input(input.PointerUp{Pos: geom.Pt(100, 22)})
	run(10)
	if !slices.Equal(l.Keys(), keys(5)) || len(sent(w)) != 0 {
		t.Fatal("a click moved a row")
	}
}

type rowClicked struct{ Key Key }

func TestARowThatIsNotDraggedIsClicked(t *testing.T) {
	w, l, run := newReorderList(t)
	l.OnClick = func(k Key) gunim.Intent { return rowClicked{k} }
	w.Input(input.PointerDown{Pos: geom.Pt(100, 46+20), Clicks: 1})
	w.Input(input.PointerMove{Pos: geom.Pt(102, 46+21)})
	w.Input(input.PointerUp{Pos: geom.Pt(102, 46+21)})
	run(1)
	w.Input(input.PointerDown{Pos: geom.Pt(100, 20), Clicks: 1})
	for y := float32(20); y <= 110; y += 10 {
		w.Input(input.PointerMove{Pos: geom.Pt(100, y)})
		run(1)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(100, 110)})
	run(60)
	got := sent(w)
	if len(got) != 2 || got[0] != (rowClicked{"1"}) {
		t.Fatalf("a click on row 1 and a drag of row 0 sent %v, want the click and then the new order", got)
	}
	if _, ok := got[1].(reordered); !ok {
		t.Fatalf("the drag sent %v, want the new order and no click", got[1])
	}
}
