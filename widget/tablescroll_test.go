package widget

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// columnsKept fails the test where the window's last frame shows text past the table's edges or past the column it
// starts in.
func columnsKept(t *testing.T, w *gunim.Window, tbl *Table, when string) {
	t.Helper()
	for _, s := range shownOps(w.Offscreen().Ops()) {
		if s.r.Min.X < -0.5 || s.r.Max.X > tbl.width+0.5 {
			t.Fatalf("%s: a %s shows from x=%v to %v, outside the table, 0 to %v", when, s.what, s.r.Min.X, s.r.Max.X, tbl.width)
		}
		if s.what != "text" {
			continue
		}
		for c, x := range tbl.xs {
			if left := x[0] - tbl.left; s.r.Min.X >= left-0.5 && s.r.Min.X < left+x[1] && s.r.Max.X > left+x[1]+0.5 {
				t.Fatalf("%s: text in column %d shows to x=%v, past the column's end, %v", when, c, s.r.Max.X, left+x[1])
			}
		}
	}
}

// Columns wider than the table scroll sideways, the titles with the rows: by a sideways wheel on a row, the wheel
// with Shift held on the titles, and Left and Right. Every frame keeps the text inside the table and its columns.
func TestATableWiderThanItScrollsSideways(t *testing.T) {
	w, tbl, ev, run := newTableStage(t, 300)
	tbl.Columns = []TableColumn{
		{Title: "Name"},
		{Title: "A long title for a column", Width: 250},
		{Title: "Another long title", Width: 250, End: true},
		{Title: "Size", Width: 250, End: true},
	}
	tbl.Row = func(k Key) TableRow {
		return TableRow{Cells: []string{"row " + string(k), "a cell with more text than fits in it, by far", "1234567890 1234567890 1234567890", string(k) + " KB"}}
	}
	run(1)
	if tbl.wide <= tbl.width || tbl.left != 0 {
		t.Fatalf("columns %v wide in a table %v wide, scrolled %v", tbl.wide, tbl.width, tbl.left)
	}
	columnsKept(t, w, tbl, "at rest")
	steps := []struct {
		what string
		ev   any
		want func() float32
	}{
		{"a sideways wheel on a row", input.Scroll{Pos: geom.Pt(300, 100), Delta: geom.Pt(-120, 0), Time: time.Now()}, func() float32 { return 120 }},
		{"Shift and the wheel on the titles", input.Scroll{Pos: geom.Pt(300, 10), Delta: geom.Pt(0, -5000), Mods: input.ModShift, Time: time.Now()}, func() float32 { return tbl.wide - tbl.width }},
		{"Right at the end", input.KeyPress{Key: input.KeyRight}, func() float32 { return tbl.wide - tbl.width }},
		{"Left", input.KeyPress{Key: input.KeyLeft}, func() float32 { return tbl.wide - tbl.width - ScrollLine.Default() }},
	}
	top := tbl.Offset()
	for _, s := range steps {
		w.Input(s.ev)
		for i := range 3 {
			run(1)
			columnsKept(t, w, tbl, s.what+", frame "+strconv.Itoa(i))
		}
		if want := s.want(); tbl.left != want {
			t.Fatalf("after %s the columns are scrolled %v, want %v", s.what, tbl.left, want)
		}
	}
	if tbl.Offset() != top {
		t.Fatalf("scrolling sideways moved the rows from %v to %v", top, tbl.Offset())
	}
	// A click on the last title, where it is now, sorts by it.
	x := tbl.xs[3][0] - tbl.left + 20
	w.Input(input.PointerDown{Pos: geom.Pt(x, 10), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(x, 10), Button: input.ButtonPrimary})
	run(1)
	if len(ev.sorts) != 1 || ev.sorts[0][0] != 3 {
		t.Fatalf("a click on the last title asked for sorts %v, want by column 3", ev.sorts)
	}
}

// A column too narrow for even an ellipsis shows nothing of its cells, and a table no wider than its columns does
// not scroll.
func TestATableColumnTooNarrowForTextShowsNone(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 30)
	tbl.Columns = []TableColumn{{Title: "Name"}, {Title: "Tiny", Width: 5}, {Title: "Size", Width: 100, End: true}}
	tbl.Row = func(k Key) TableRow { return TableRow{Cells: []string{"row " + string(k), "xyz", "1 KB"}} }
	run(1)
	left := tbl.xs[1][0]
	for _, s := range shownOps(w.Offscreen().Ops()) {
		if s.what == "text" && s.r.Min.X >= left-0.5 && s.r.Min.X < left+5 {
			t.Fatalf("the 5 px column shows text from x=%v to %v", s.r.Min.X, s.r.Max.X)
		}
	}
	w.Input(input.Scroll{Pos: geom.Pt(300, 100), Delta: geom.Pt(-120, 0), Time: time.Now()})
	run(1)
	if tbl.left != 0 {
		t.Fatalf("a table as wide as its columns scrolled sideways to %v", tbl.left)
	}
}

// BenchmarkTableTypeAhead times a name typed to find the last of 200 000 rows, and the frame after it.
func BenchmarkTableTypeAhead(b *testing.B) {
	tbl := NewTable(TableColumn{Title: "Name"}, TableColumn{Title: "Size", Width: 100, End: true})
	tbl.Row = func(k Key) TableRow { return TableRow{Cells: []string{"Row " + string(k), string(k) + " KB"}} }
	keys := make([]Key, 200_000)
	for i := range keys {
		keys[i] = Key(strconv.Itoa(i))
	}
	w := gunimtest.New(b, geom.Sz(600, 400), nil)
	gunim.RegisterView(w, "t", func(struct{}) *Table { return tbl },
		func(tb *Table, _ struct{}, u *gunim.UI) { tb.SetKeys(keys, u) })
	if err := w.Client().Mount(gunim.Root, "t", "t", nil); err != nil {
		b.Fatal(err)
	}
	if err := w.Client().Update("t", struct{}{}); err != nil {
		b.Fatal(err)
	}
	if err := w.Client().Focus("t"); err != nil {
		b.Fatal(err)
	}
	w.Frame(time.Second / 60)
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		// Each name after a pause, so each starts afresh.
		w.Input(input.TextInput{Text: "row 199999", Time: now.Add(time.Duration(i) * 2 * time.Second)})
		w.Frame(time.Second / 60)
	}
	b.StopTimer()
	if k, _ := tbl.Cursor(); k != "199999" {
		b.Fatalf("typing found %q", k)
	}
}
