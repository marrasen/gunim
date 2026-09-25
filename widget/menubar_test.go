package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type barPick struct{ menu, item int }

// newBarStage lays out a menubar over a text field that has the
// keyboard.
func newBarStage(t *testing.T) (*gunim.Window, *Menubar, *TextField, *[]barPick, func(int)) {
	t.Helper()
	var picks []barPick
	b := NewMenubar(
		BarMenu{Title: "File", Items: []string{"New", "Open", "Quit"}, Breaks: []int{2}},
		BarMenu{Title: "Edit", Items: []string{"Copy", "Paste", "Select All"}, Disabled: []bool{true}, Hints: []string{"Ctrl+C", "Ctrl+V"}},
		BarMenu{Title: "View", Items: []string{"Sidebar"}, Checked: []bool{true}},
	)
	b.Pick = func(m, i int, _ *gunim.UI) { picks = append(picks, barPick{m, i}) }
	field := NewTextField()
	col := Column(b, field)
	col.Cross = CrossStretch
	w, run := stage(t, &frame{child: col, size: geom.Sz(600, 400)})
	w.Input(input.PointerDown{Pos: geom.Pt(100, 60), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(100, 60), Time: time.Now()})
	run(2)
	return w, b, field, &picks, run
}

func clickAt(w *gunim.Window, run func(int), at geom.Point) {
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: at, Time: time.Now()})
	run(1)
}

func TestAMenubarOpensMenusAndGivesTheKeyboardBack(t *testing.T) {
	w, b, field, _, run := newBarStage(t)
	file := b.span(0)
	clickAt(w, run, geom.Pt((file[0]+file[1])/2, 15))
	if b.open != 0 {
		t.Fatalf("clicking File opened menu %d", b.open)
	}
	// With a menu open, the pointer onto Edit opens Edit.
	edit := b.span(1)
	w.Input(input.PointerMove{Pos: geom.Pt((edit[0]+edit[1])/2, 15), Time: time.Now()})
	run(1)
	if b.open != 1 {
		t.Fatalf("moving onto Edit left menu %d open", b.open)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(20)
	if b.IsOpen() {
		t.Fatal("Escape left the menu open")
	}
	// The field has the keyboard again.
	w.Input(input.TextInput{Text: "hi"})
	run(1)
	if field.Text() != "hi" {
		t.Fatalf("after the menu closed, typing reached %q, want the field", field.Text())
	}
}

func TestMenubarKeysCrossMenusAndPassDisabledItems(t *testing.T) {
	w, b, _, picks, run := newBarStage(t)
	file := b.span(0)
	clickAt(w, run, geom.Pt((file[0]+file[1])/2, 15))
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	// Edit's first item, Copy, is disabled: Down goes to Paste.
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(*picks) != 1 || (*picks)[0] != (barPick{1, 1}) {
		t.Fatalf("picked %v, want Paste from Edit", *picks)
	}
	if b.IsOpen() {
		t.Fatal("the menu stayed open after a pick")
	}
}

func TestAMenuFindsItemsBelowALine(t *testing.T) {
	w, b, _, picks, run := newBarStage(t)
	file := b.span(0)
	clickAt(w, run, geom.Pt((file[0]+file[1])/2, 15))
	run(20)
	m := b.menu
	// Quit sits below the line, a line's room further down.
	if got, want := m.rowY(2)-m.rowY(1), m.row+menuBreak; got != want {
		t.Fatalf("Quit is %v below Open, want %v", got, want)
	}
	if i := m.rowAt(geom.Pt(m.card.Min.X+20, m.rowY(2)+2)); i != 2 {
		t.Fatalf("the pointer at Quit finds item %d", i)
	}
	for range 3 {
		w.Input(input.KeyPress{Key: input.KeyDown})
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(*picks) != 1 || (*picks)[0] != (barPick{0, 2}) {
		t.Fatalf("picked %v, want Quit", *picks)
	}
}
