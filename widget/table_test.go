package widget

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type tableEvents struct {
	activated []Key
	sorts     [][2]int
}

// newTableStage mounts a table of n rows, named row 0 on, with a size
// column, in a 600 by 400 window, with the keyboard.
func newTableStage(t *testing.T, n int) (*gunim.Window, *Table, *tableEvents, func(int)) {
	t.Helper()
	ev := &tableEvents{}
	tbl := NewTable(TableColumn{Title: "Name"}, TableColumn{Title: "Size", Width: 100, End: true})
	tbl.Row = func(k Key) TableRow { return TableRow{Cells: []string{"row " + string(k), string(k) + " KB"}} }
	tbl.OnActivate = func(k Key, _ *gunim.UI) { ev.activated = append(ev.activated, k) }
	tbl.OnSort = func(col int, desc bool, _ *gunim.UI) {
		d := 0
		if desc {
			d = 1
		}
		ev.sorts = append(ev.sorts, [2]int{col, d})
	}
	keys := make([]Key, n)
	for i := range keys {
		keys[i] = Key(strconv.Itoa(i))
	}
	w := gunim.NewOffscreen(geom.Sz(600, 400), nil)
	gunim.RegisterView(w, "t", func(struct{}) *Table { return tbl },
		func(tb *Table, _ struct{}, u *gunim.UI) { tb.SetKeys(keys, u) })
	if err := w.Client().Mount(gunim.Root, "t", "t", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Update("t", struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Focus("t"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(10)
	return w, tbl, ev, run
}

func TestATableBuildsOnlyTheRowsInView(t *testing.T) {
	_, tbl, _, _ := newTableStage(t, 1000)
	if n := tbl.list.Len(); n != 1000 {
		t.Fatalf("the table holds %d rows, want 1000", n)
	}
	if n := tbl.list.Built(); n == 0 || n > 40 {
		t.Fatalf("the table built %d rows for a view of about 14", n)
	}
}

func TestTheKeysMoveTheCursorAndTheViewFollows(t *testing.T) {
	w, tbl, ev, run := newTableStage(t, 1000)
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if k, _ := tbl.Cursor(); k != "2" {
		t.Fatalf("after Down twice the cursor is on %q, want 2", k)
	}
	w.Input(input.KeyPress{Key: input.KeyEnd})
	run(60)
	if k, _ := tbl.Cursor(); k != "999" {
		t.Fatalf("after End the cursor is on %q, want 999", k)
	}
	if tbl.list.Offset() <= 0 {
		t.Fatal("after End the view stayed at the top")
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if !slices.Equal(ev.activated, []Key{"999"}) {
		t.Fatalf("activated %v, want 999", ev.activated)
	}
}

func TestSpaceMarksAndMovesOn(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 10)
	w.Input(input.KeyPress{Key: input.KeySpace})
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(1)
	if got := tbl.Marked(); !slices.Equal(got, []Key{"0", "2"}) {
		t.Fatalf("marked %v, want 0 and 2", got)
	}
	if k, _ := tbl.Cursor(); k != "3" {
		t.Fatalf("the cursor is on %q, want 3", k)
	}
}

func TestAClickOnATitleSortsAndAgainTheOtherWay(t *testing.T) {
	w, _, ev, run := newTableStage(t, 10)
	for range 2 {
		w.Input(input.PointerDown{Pos: geom.Pt(40, 13), Clicks: 1, Time: time.Now()})
		w.Input(input.PointerUp{Pos: geom.Pt(40, 13), Time: time.Now()})
		run(1)
	}
	if !slices.Equal(ev.sorts, [][2]int{{0, 0}, {0, 1}}) {
		t.Fatalf("sorts %v, want the names up, then down", ev.sorts)
	}
}

func TestTheCursorStaysOnItsRowAsTheRowsReorder(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 10)
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	// Reverse the rows; row 1 moves to the second last place.
	rev := make([]Key, 10)
	for i := range rev {
		rev[i] = Key(strconv.Itoa(9 - i))
	}
	gunim.RegisterView(w, "flip", func(struct{}) *Label { return NewLabel("") },
		func(_ *Label, _ struct{}, u *gunim.UI) { tbl.SetKeys(rev, u) })
	if err := w.Client().Mount(gunim.Root, "flip", "flip", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Update("flip", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(2)
	if k, _ := tbl.Cursor(); k != "1" {
		t.Fatalf("after the rows reversed the cursor is on %q, want 1", k)
	}
}
