package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
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

func TestTheKeysPassACaptionBy(t *testing.T) {
	m := NewMenu("Split", "Right", "Down")
	m.Captions = []int{0}
	m.Key(input.KeyPress{Key: input.KeyDown}, nil)
	if m.Highlighted() != 1 {
		t.Fatalf("Down from nothing highlighted %d, want Right, past the caption", m.Highlighted())
	}
	m.Key(input.KeyPress{Key: input.KeyUp}, nil)
	if m.Highlighted() != 1 {
		t.Fatalf("Up from the first item moved to %d, want it to stay", m.Highlighted())
	}
}

func TestAMenubarSaysWhatIsHighlighted(t *testing.T) {
	w, b, _, _, run := newBarStage(t)
	var lit []barPick
	b.OnHighlight = func(m, i int, _ *gunim.UI) { lit = append(lit, barPick{m, i}) }
	file := b.span(0)
	clickAt(w, run, geom.Pt((file[0]+file[1])/2, 15))
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	want := []barPick{{0, -1}, {0, 0}, {1, -1}, {1, 1}, {-1, -1}}
	if !slices.Equal(lit, want) {
		t.Fatalf("highlighted %v, want %v", lit, want)
	}
}

// The keypad's Enter picks, as the main one does.
func TestTheKeypadsEnterPicksFromAMenu(t *testing.T) {
	w, b, _, picks, run := newBarStage(t)
	file := b.span(0)
	clickAt(w, run, geom.Pt((file[0]+file[1])/2, 15))
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyKPEnter})
	run(20)
	if len(*picks) != 1 || (*picks)[0] != (barPick{0, 0}) {
		t.Fatalf("picked %v, want New from File", *picks)
	}
}

// newCompactStage is newBarStage with the bar compact.
func newCompactStage(t *testing.T) (*gunim.Window, *Menubar, *TextField, *[]barPick, func(int)) {
	t.Helper()
	w, b, field, picks, run := newBarStage(t)
	b.Compact = true
	run(2)
	return w, b, field, picks, run
}

func TestACompactMenubarOpensItsListAndAMenuBesideIt(t *testing.T) {
	w, b, field, picks, run := newCompactStage(t)
	if n := len(b.spans); n != 1 {
		t.Fatalf("a compact bar has %d buttons, want 1", n)
	}
	s := b.span(0)
	clickAt(w, run, geom.Pt((s[0]+s[1])/2, 15))
	if b.list == nil || b.open >= 0 {
		t.Fatalf("the click opened list %v and menu %d, want the list alone", b.list != nil, b.open)
	}
	// Down comes to File, which opens beside the list; Down again to
	// Edit, which takes its place.
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if b.open != 0 {
		t.Fatalf("Down opened menu %d, want File", b.open)
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if b.open != 1 {
		t.Fatalf("Down again opened menu %d, want Edit", b.open)
	}
	// Right goes into Edit, where Copy is disabled: the first item is
	// Paste.
	w.Input(input.KeyPress{Key: input.KeyRight})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(*picks) != 1 || (*picks)[0] != (barPick{1, 1}) {
		t.Fatalf("picked %v, want Edit's Paste", *picks)
	}
	if b.IsOpen() {
		t.Fatal("the list stayed open after the pick")
	}
	w.Input(input.TextInput{Text: "hi"})
	run(1)
	if field.Text() != "hi" {
		t.Fatalf("after the pick, typing reached %q, want the field", field.Text())
	}
}

func TestLeftTakesTheKeysBackToACompactMenubarsList(t *testing.T) {
	w, b, _, _, run := newCompactStage(t)
	s := b.span(0)
	clickAt(w, run, geom.Pt((s[0]+s[1])/2, 15))
	for _, k := range []input.Key{input.KeyDown, input.KeyRight, input.KeyDown} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	// In File, Down moved within it.
	if b.open != 0 || b.menu.Highlighted() != 1 {
		t.Fatalf("in File, Down left menu %d open at item %d", b.open, b.menu.Highlighted())
	}
	// Left goes back to the list, where Down goes on to Edit.
	for _, k := range []input.Key{input.KeyLeft, input.KeyDown} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	if b.open != 1 {
		t.Fatalf("Left then Down opened menu %d, want Edit", b.open)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(20)
	if b.IsOpen() {
		t.Fatal("Escape left the list open")
	}
}

func TestACompactMenubarLeavesTheRestOfTheBarToMoveTheWindow(t *testing.T) {
	_, b, _, _, _ := newCompactStage(t)
	s := b.span(0)
	r := b.CaptionRects(geom.Sz(600, 30))
	if len(r) != 1 || r[0].Min.X != s[1] || r[0].Max.X != 600 {
		t.Fatalf("the caption is %v, want all of the bar after the button, which ends at %v", r, s[1])
	}
}

// f10 has the keyboard, and opens a menubar's menu when F10 is
// pressed, as a program does.
type f10 struct{ open func(u *gunim.UI) }

func (k *f10) Focusable() bool { return true }

func (k *f10) Handle(e input.Event, u *gunim.UI) bool {
	if p, ok := e.(input.KeyPress); ok && p.Key == input.KeyF10 {
		k.open(u)
		return true
	}
	return false
}

func (k *f10) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Max.W, 100))
}

func (k *f10) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

func TestOpeningACompactMenubarsMenuPutsTheKeysInIt(t *testing.T) {
	var picks []barPick
	b := NewMenubar(
		BarMenu{Title: "File", Items: []string{"New"}},
		BarMenu{Title: "Edit", Items: []string{"Copy", "Paste"}, Disabled: []bool{true}},
	)
	b.Compact = true
	b.Pick = func(m, i int, _ *gunim.UI) { picks = append(picks, barPick{m, i}) }
	col := Column(b, &f10{open: func(u *gunim.UI) { b.Open(1, u) }})
	col.Cross = CrossStretch
	w, run := stage(t, &frame{child: col, size: geom.Sz(600, 400)})
	w.Input(input.PointerDown{Pos: geom.Pt(100, 60), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(100, 60), Time: time.Now()})
	run(2)
	w.Input(input.KeyPress{Key: input.KeyF10})
	run(1)
	if b.open != 1 || !b.inMenu {
		t.Fatalf("F10 left menu %d open, the keys in it %v", b.open, b.inMenu)
	}
	// Opened before the list had laid out, the menu moves beside it.
	run(3)
	if row := b.list.RowRect(1); b.besideAt.X < row.Max.X || b.besideAt.Y > row.Min.Y {
		t.Fatalf("Edit's menu is at %v, want beside its line, %v", b.besideAt, row)
	}
	// Edit's Copy is disabled: Down comes to Paste.
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(picks) != 1 || picks[0] != (barPick{1, 1}) {
		t.Fatalf("picked %v, want Edit's Paste", picks)
	}
}
