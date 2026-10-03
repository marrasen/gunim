package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestAHandleDragsItsEndOfTheSelection(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("one two three four five")
	ty.secondary(ty.xOf(5), true) // a long press on two
	f := ty.field
	if f.handles[0] == nil || f.handles[1] == nil {
		t.Fatal("no handles showed after a long press selected a word")
	}
	ty.run(2)
	p := f.handles[1]
	win := p.Offscreen()
	if win == nil {
		t.Fatal("the end's handle has no window")
	}
	// The popup's window sits where the handle hangs; the field is at
	// the window's top left.
	win.SetOrigin(f.handleAt[1].Min)
	grip := geom.Pt(handleW/2, handleH-8)
	from, to := ty.xOf(7), ty.xOf(18) // the end of two, the end of four
	p.Input(input.PointerDown{Pos: grip, Button: input.ButtonPrimary, Clicks: 1, Touch: true})
	if f.menuItems != nil {
		t.Fatal("the menu stayed open while a handle dragged")
	}
	p.Input(input.PointerMove{Pos: grip.Add(geom.Pt(to-from, 0)), Touch: true})
	p.Input(input.PointerUp{Pos: grip.Add(geom.Pt(to-from, 0)), Button: input.ButtonPrimary, Touch: true})
	ty.run(2)
	if s, e := f.Selection(); s != 4 || e != 18 {
		t.Fatalf("dragging the end's handle to four's end selected %d–%d, want 4–18", s, e)
	}
	if f.menuItems == nil {
		t.Fatal("the menu did not come back as the handle let go")
	}
	ty.typeText("x")
	if f.handles[0] != nil || f.handles[1] != nil {
		t.Fatal("the handles stayed after typing")
	}
}

func TestAnEndOfTheSelectionStopsShortOfTheOther(t *testing.T) {
	e := &editor{text: []rune("hello world"), anchor: 6, caret: 11}
	e.moveEnd(1, 2) // the end's handle dragged back past the start
	if s, en := e.Selection(); s != 6 || en != 7 {
		t.Fatalf("the selection is %d–%d, want 6–7, a rune left", s, en)
	}
	e.moveEnd(0, 9) // the start's handle dragged on past the end
	if s, en := e.Selection(); s != 6 || en != 7 {
		t.Fatalf("the selection is %d–%d, want 6–7", s, en)
	}
}
