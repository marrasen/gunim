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

func TestTypingFindsARowByTheStartOfItsName(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 300)
	now := time.Now()
	// "row 2" is row 2; "row 25" comes further on.
	w.Input(input.TextInput{Text: "ROW 2", Time: now})
	w.Input(input.TextInput{Text: "5", Time: now.Add(200 * time.Millisecond)})
	run(1)
	if k, _ := tbl.Cursor(); k != "25" {
		t.Fatalf("typing row 25 put the cursor on %q", k)
	}
	// After a pause, typing starts a new name.
	w.Input(input.TextInput{Text: "row 7", Time: now.Add(3 * time.Second)})
	run(1)
	if k, _ := tbl.Cursor(); k != "7" {
		t.Fatalf("typing row 7 after a pause put the cursor on %q", k)
	}
}

func TestSpaceTwiceMarksTwoRows(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 10)
	now := time.Now()
	for i := range 2 {
		at := now.Add(time.Duration(i) * 100 * time.Millisecond)
		w.Input(input.KeyPress{Key: input.KeySpace, Typed: true, Time: at})
		w.Input(input.TextInput{Text: " ", Time: at})
	}
	run(1)
	if got := tbl.Marked(); !slices.Equal(got, []Key{"0", "1"}) {
		t.Fatalf("marked %v, want 0 and 1", got)
	}
}

// Backspace takes back a letter of a name being found, Escape gives up
// finding, and after that both are left to whatever holds the table.
func TestBackspaceAndEscapeWorkOnANameBeingFound(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 300)
	now := time.Now()
	w.Input(input.TextInput{Text: "row 25", Time: now})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyBackspace, Time: now.Add(100 * time.Millisecond)})
	run(1)
	if k, _ := tbl.Cursor(); k != "2" {
		t.Fatalf("taking back the 5 left the cursor on %q, want row 2", k)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape, Time: now.Add(200 * time.Millisecond)})
	run(1)
	if tbl.finding(now.Add(300 * time.Millisecond)) {
		t.Fatal("Escape left the name being found")
	}
}

func TestInsertMarksAndMovesOn(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 10)
	w.Input(input.KeyPress{Key: input.KeyInsert})
	run(1)
	if !slices.Equal(tbl.Marked(), []Key{"0"}) {
		t.Fatalf("Insert marked %v", tbl.Marked())
	}
	if k, _ := tbl.Cursor(); k != "1" {
		t.Fatalf("Insert left the cursor on %q", k)
	}
}
