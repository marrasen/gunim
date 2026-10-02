package widget

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
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
	w, run := stage(t, &frame{child: col, size: geom.Sz(600, 400), keysGoOn: true})
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
	// Up from the first item goes round to the last, as on Windows, and Down comes back round
	m.Key(input.KeyPress{Key: input.KeyUp}, nil)
	if m.Highlighted() != 2 {
		t.Fatalf("Up from the first item moved to %d, want round to Down, the last", m.Highlighted())
	}
	m.Key(input.KeyPress{Key: input.KeyDown}, nil)
	if m.Highlighted() != 1 {
		t.Fatalf("Down from the last item moved to %d, want round to Right, past the caption", m.Highlighted())
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

func TestF10OpensACompactMenubarsListWithTheKeysOnIt(t *testing.T) {
	var picks []barPick
	b := NewMenubar(
		BarMenu{Title: "File", Items: []string{"New"}},
		BarMenu{Title: "Edit", Items: []string{"Copy", "Paste"}, Disabled: []bool{true}},
	)
	b.Compact = true
	b.Pick = func(m, i int, _ *gunim.UI) { picks = append(picks, barPick{m, i}) }
	col := Column(b, &f10{open: func(u *gunim.UI) { b.Open(0, u) }})
	col.Cross = CrossStretch
	w, run := stage(t, &frame{child: col, size: geom.Sz(600, 400)})
	w.Input(input.PointerDown{Pos: geom.Pt(100, 60), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(100, 60), Time: time.Now()})
	run(2)
	w.Input(input.KeyPress{Key: input.KeyF10})
	run(1)
	if b.open != 0 || b.inMenu || b.list.Highlighted() != 0 {
		t.Fatalf("F10 left menu %d open, File's line lit %v, the keys in the menu %v", b.open, b.list.Highlighted() == 0, b.inMenu)
	}
	// Opened before the list had laid out, the menu goes beside its line once it has.
	run(3)
	if card, row := b.panel.cardInList(), b.list.RowRect(0); card.Min.X < row.Max.X || card.Min.Y > row.Min.Y {
		t.Fatalf("File's menu is on a card at %v, want beside its line, %v", card, row)
	}
	// Down goes along the list to Edit, and opens its menu
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if b.open != 1 || b.inMenu {
		t.Fatalf("Down left menu %d open, the keys in the menu %v; want Edit's, the keys on the list", b.open, b.inMenu)
	}
	// Right goes into Edit, where Copy is disabled: Enter picks Paste
	w.Input(input.KeyPress{Key: input.KeyRight})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(picks) != 1 || picks[0] != (barPick{1, 1}) {
		t.Fatalf("picked %v, want Edit's Paste", picks)
	}
}

// openCompactList opens a compact bar's list on a stage whose popups show what is behind them, so menus leave a
// margin for their shadow.
func openCompactList(t *testing.T) (b *Menubar, run func(int), move func(at geom.Point)) {
	t.Helper()
	w, b, _, _, run := newCompactStage(t)
	s := b.span(0)
	clickAt(w, run, geom.Pt((s[0]+s[1])/2, 15))
	run(2)
	if b.listPopup.Offscreen() == nil {
		t.Fatal("the list has no window")
	}
	list := b.listPopup
	move = func(at geom.Point) {
		list.Input(input.PointerMove{Pos: at, Time: time.Now()})
		run(1)
	}
	return b, run, move
}

// middle is the middle of r.
func middle(r geom.Rect) geom.Point { return r.Center() }

func TestACompactMenubarsMenuSitsLevelWithItsLineAgainstTheList(t *testing.T) {
	b, run, move := openCompactList(t)
	if b.list.margin <= 0 {
		t.Fatalf("the list leaves a margin of %v, want room for a shadow", b.list.margin)
	}
	for i := range b.Menus {
		move(middle(b.list.RowRect(i)))
		run(40)
		card := b.panel.cardInList()
		first := card.Min.Y + b.menu.RowRect(0).Min.Y
		if row := b.list.RowRect(i); first < row.Min.Y-1 || first > row.Min.Y+1 {
			t.Fatalf("menu %d's first line is at %v, want level with its line on the list, at %v", i, first, row.Min.Y)
		}
		if card.Min.X != b.list.card.Max.X {
			t.Fatalf("menu %d's card starts at %v, want against the list's, which ends at %v", i, card.Min.X, b.list.card.Max.X)
		}
	}
}

func TestMovingAlongACompactMenubarsListOpensNoWindow(t *testing.T) {
	b, run, move := openCompactList(t)
	list := b.listPopup
	for i := range b.Menus {
		move(middle(b.list.RowRect(i)))
		if b.open != i || b.popup != nil || b.listPopup != list {
			t.Fatalf("on line %d, menu %d is open, in a popup of its own %v, the list in the same one %v",
				i, b.open, b.popup != nil, b.listPopup == list)
		}
	}
	// The window is kept at its largest from the start, so it does not grow as the menus change
	size := list.Offscreen().Size()
	for i := range b.Menus {
		move(middle(b.list.RowRect(i)))
		run(20)
		if got := list.Offscreen().Size(); got != size {
			t.Fatalf("on line %d, the list's window is %v, where it was %v", i, got, size)
		}
	}
}

func TestHeadingForAnOpenMenuCrossesLinesWithoutOpeningTheirs(t *testing.T) {
	b, run, move := openCompactList(t)
	move(middle(b.list.RowRect(0)))
	run(20)
	if b.open != 0 {
		t.Fatalf("resting on File opened menu %d", b.open)
	}
	// From File's line, down and to the right toward File's menu, over Edit's line
	card := b.panel.cardInList()
	from := geom.Pt(b.list.RowRect(0).Center().X, b.list.RowRect(0).Max.Y-2)
	to := geom.Pt(card.Min.X-2, b.list.RowRect(1).Center().Y)
	move(from)
	move(to)
	if b.list.Highlighted() != 1 || b.open != 0 {
		t.Fatalf("heading for File's menu over Edit's line opened menu %d, with line %d lit", b.open, b.list.Highlighted())
	}
	// Resting there opens Edit's menu after all
	run(15)
	if b.open != 1 {
		t.Fatalf("resting on Edit's line left menu %d open", b.open)
	}
	// Moving straight down opens the next line's menu at once
	move(middle(b.list.RowRect(2)))
	if b.open != 2 {
		t.Fatalf("moving straight down to View left menu %d open", b.open)
	}
}

// newRoomStage opens a compact bar's list, with a menu of lines items, the screen ending below bottom and above top,
// in the window's space.
func newRoomStage(t *testing.T, lines int, top, bottom float32) (b *Menubar, run func(int), move func(at geom.Point)) {
	t.Helper()
	_, b, run, move = newAreaStage(t, lines, geom.Rect{Min: geom.Pt(-1000, top), Max: geom.Pt(5000, bottom)})
	return b, run, move
}

// newAreaStage opens a compact bar's list, with a menu of lines items, the screen's work area being area, in the
// window's space.
func newAreaStage(t *testing.T, lines int, area geom.Rect) (w *gunim.Window, b *Menubar, run func(int), move func(at geom.Point)) {
	t.Helper()
	tall := BarMenu{Title: "Font"}
	for i := range lines {
		tall.Items = append(tall.Items, fmt.Sprintf("Font %d", i))
	}
	b = NewMenubar(
		BarMenu{Title: "File", Items: []string{"New", "Open", "Quit"}},
		BarMenu{Title: "Edit", Items: []string{"Copy", "Paste"}},
		tall,
	)
	b.Compact = true
	col := Column(b, NewTextField())
	col.Cross = CrossStretch
	w, run = stage(t, &frame{child: col, size: geom.Sz(600, 400)})
	w.Offscreen().SetWorkArea(area)
	s := b.span(0)
	clickAt(w, run, geom.Pt((s[0]+s[1])/2, 15))
	run(2)
	list := b.listPopup
	move = func(at geom.Point) {
		list.Input(input.PointerMove{Pos: at.Add(b.panel.off), Time: time.Now()})
		run(1)
	}
	return w, b, run, move
}

func TestACompactMenubarsTallMenuMovesUpToStayOnTheScreen(t *testing.T) {
	const bottom = 440
	b, run, move := newRoomStage(t, 12, -1000, bottom)
	below := bottom - b.listPopup.Offscreen().Anchor().Max.Y
	move(middle(b.list.RowRect(2)))
	run(40)
	if b.open != 2 || b.panel.up {
		t.Fatalf("resting on Font left menu %d open, the panel opening above the button %v", b.open, b.panel.up)
	}
	card := b.panel.sideCard()
	if card.Max.Y > below-b.panel.margin {
		t.Fatalf("Font's menu reaches down to %v, past the room below the button, %v", card.Max.Y, below)
	}
	if first := b.panel.cardInList().Min.Y + b.menu.RowRect(0).Min.Y; first >= b.list.RowRect(2).Min.Y {
		t.Fatalf("Font's first line is at %v, not above its line on the list, at %v: the test has room enough", first, b.list.RowRect(2).Min.Y)
	}
	if h := b.listPopup.Offscreen().Size().H; h > below {
		t.Fatalf("the list's window is %v tall, taller than the room below the button, %v", h, below)
	}
	// A menu with room for it stays level with its line
	move(middle(b.list.RowRect(0)))
	run(40)
	first := b.panel.cardInList().Min.Y + b.menu.RowRect(0).Min.Y
	if row := b.list.RowRect(0); first < row.Min.Y-1 || first > row.Min.Y+1 {
		t.Fatalf("File's first line is at %v, want level with its line on the list, at %v", first, row.Min.Y)
	}
}

func TestACompactMenubarsListOpensAboveWhereItDoesNotFitBelow(t *testing.T) {
	b, run, move := newRoomStage(t, 12, -1000, 90)
	if !b.panel.up {
		t.Fatal("with no room below for the list, the panel opens below the button")
	}
	size := b.listPopup.Offscreen().Size()
	if bottom := b.panel.off.Y + b.list.card.Max.Y + b.list.margin; bottom < size.H-1 || bottom > size.H+1 {
		t.Fatalf("the list ends at %v, want at the bottom of the window, %v, against the button", bottom, size.H)
	}
	move(middle(b.list.RowRect(2)))
	run(40)
	if card := b.panel.sideCard(); b.open != 2 || card.Min.Y < b.panel.margin-1 || card.Max.Y > size.H-b.panel.margin+1 {
		t.Fatalf("Font's menu, open %v, is on a card at %v, want inside the window, %v tall", b.open == 2, card, size.H)
	}
}

func TestAMenuTallerThanTheRoomBelowRisesAboveTheButtonWithoutTheList(t *testing.T) {
	const bottom = 440
	b, run, move := newRoomStage(t, 30, -1000, bottom)
	move(middle(b.list.RowRect(2)))
	run(40)
	if b.open != 2 || b.panel.up {
		t.Fatalf("resting on Font left menu %d open, the list opening above the button %v", b.open, b.panel.up)
	}
	if b.panel.head <= 0 {
		t.Fatal("the window reaches no higher than the list, with Font's menu too tall for the room below")
	}
	// The window still ends inside the screen, the list at the head's depth
	size := b.listPopup.Offscreen().Size()
	if end := b.listPopup.Offscreen().Anchor().Max.Y + size.H; end > bottom+1 {
		t.Fatalf("the window reaches down to %v, past the bottom of the screen, %v", end, float32(bottom))
	}
	card := b.panel.sideCard()
	if card.Min.Y < b.panel.margin-1 || card.Max.Y > size.H-b.panel.margin+1 {
		t.Fatalf("Font's menu is on a card at %v, want inside the window, %v tall", card, size.H)
	}
}

func TestAMenuWithNoRoomOnTheRightOpensOnTheListsLeft(t *testing.T) {
	// The screen ends just right of the list, with room on the left
	const right = 260
	_, b, run, move := newAreaStage(t, 4, geom.Rect{Min: geom.Pt(-1000, -1000), Max: geom.Pt(right, 1000)})
	move(middle(b.list.RowRect(1)))
	run(40)
	if b.open != 1 || !b.panel.left {
		t.Fatalf("resting on Edit left menu %d open, on the list's left %v", b.open, b.panel.left)
	}
	card, list := b.panel.cardInList(), b.list.card
	if card.Max.X != list.Min.X {
		t.Fatalf("Edit's menu ends at %v, want against the list's left edge, at %v", card.Max.X, list.Min.X)
	}
	if b.panel.sideCard().Min.X < b.panel.margin {
		t.Fatalf("Edit's menu starts at %v, outside the window", b.panel.sideCard().Min.X)
	}
	// The list stays under the button, and the window on the screen
	pw := b.listPopup.Offscreen()
	if x := pw.Anchor().Min.X + b.panel.off.X + list.Min.X; x < b.span(0)[0]-1 || x > b.span(0)[0]+1 {
		t.Fatalf("the list starts at %v, want at the button's left edge, %v", x, b.span(0)[0])
	}
	if end := pw.Anchor().Min.X + pw.Size().W; end > right+1 {
		t.Fatalf("the window reaches %v, past the screen's right edge, %v", end, float32(right))
	}
	// Heading down and left for File's menu over Edit's line keeps it open, and resting there opens Edit's
	move(middle(b.list.RowRect(0)))
	run(40)
	if b.open != 0 || !b.panel.left {
		t.Fatalf("resting on File left menu %d open, on the list's left %v", b.open, b.panel.left)
	}
	move(geom.Pt(b.list.RowRect(0).Center().X, b.list.RowRect(0).Max.Y-2))
	move(geom.Pt(list.Min.X+2, b.list.RowRect(1).Center().Y))
	if b.open != 0 {
		t.Fatalf("heading for File's menu over Edit's line opened menu %d", b.open)
	}
	run(15)
	if b.open != 1 {
		t.Fatalf("resting on Edit's line left menu %d open", b.open)
	}
}

func TestMovingTheWindowByItsFrameClosesACompactMenubarsList(t *testing.T) {
	w, b, _, _, run := newCompactStage(t)
	s := b.span(0)
	clickAt(w, run, geom.Pt((s[0]+s[1])/2, 15))
	if !b.IsOpen() {
		t.Fatal("the click left the list closed")
	}
	// The press that starts a move goes to the system, which says so
	w.Input(driver.MoveStarted{})
	run(20)
	if b.IsOpen() {
		t.Fatal("starting to move the window left the list open")
	}
}

func TestACompactMenubarsListFitsItselfAgainWhereTheWindowMoves(t *testing.T) {
	// With room below, Font's menu stays level with its line
	w, b, run, move := newAreaStage(t, 12, geom.Rect{Min: geom.Pt(-1000, -1000), Max: geom.Pt(5000, 1000)})
	move(middle(b.list.RowRect(2)))
	run(40)
	level := b.list.RowRect(2).Min.Y
	if first := b.panel.cardInList().Min.Y + b.menu.RowRect(0).Min.Y; first < level-1 || first > level+1 {
		t.Fatalf("Font's first line is at %v, want level with its line, at %v", first, level)
	}
	// Moved down, so the screen ends close below the button, it moves up to stay on the screen
	w.Offscreen().SetOrigin(geom.Pt(0, 1000-440))
	w.Input(driver.Redraw{})
	run(40)
	if first := b.panel.cardInList().Min.Y + b.menu.RowRect(0).Min.Y; first >= level {
		t.Fatalf("after the window moved down, Font's first line is at %v, not above its line, at %v", first, level)
	}
}

// A line picked by its access key in a compact menubar's menu is picked,
// and the list that closed with it is not asked to light its line.
func TestACompactMenubarsLinePickedByItsKeyIsPicked(t *testing.T) {
	var picks []barPick
	b := NewMenubar(
		BarMenu{Title: "&File", Items: []string{"&New", "&Open"}},
		BarMenu{Title: "&Edit", Items: []string{"&Copy", "&Paste"}},
	)
	b.Compact = true
	b.Pick = func(m, i int, _ *gunim.UI) { picks = append(picks, barPick{m, i}) }
	col := Column(b)
	col.Cross = CrossStretch
	w, run := stage(t, &frame{child: col, size: geom.Sz(600, 400), keysGoOn: true})
	w.Input(input.KeyPress{Key: input.KeyE, Mods: input.ModAlt, Char: 'e'})
	run(3)
	w.Input(input.KeyPress{Key: input.KeyP, Char: 'p'})
	run(20)
	if len(picks) != 1 || picks[0] != (barPick{1, 1}) {
		t.Fatalf("picked %v, want Edit's Paste", picks)
	}
}

// A press on a title holds the pointer for the bar until the release. A move from the menu's own window meanwhile,
// such as one made up as the menu opens under the pointer, is in the menu's space: read in the bar's, it lands on
// another title, whose menu opens, and the move from that one's window lands back on the first.
func TestAMoveFromTheMenuWhileATitleIsHeldOpensNoOtherMenu(t *testing.T) {
	w, b, _, _, run := newBarStage(t)
	file, view := b.span(0), b.span(2)
	at := geom.Pt((view[0]+view[1])/2, 15)
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Clicks: 1, Time: time.Now()})
	run(2)
	if b.open != 2 {
		t.Fatalf("pressing View opened menu %d", b.open)
	}
	// The pointer, in View's menu's space, is over File in the bar's.
	b.popup.Input(input.PointerMove{Pos: geom.Pt((file[0]+file[1])/2, 6), Time: time.Now()})
	run(2)
	if b.open != 2 {
		t.Fatalf("a move from View's menu while View was held opened menu %d", b.open)
	}
	w.Input(input.PointerUp{Pos: at, Time: time.Now()})
	run(2)
	if b.open != 2 {
		t.Fatalf("letting View go left menu %d open", b.open)
	}
}
