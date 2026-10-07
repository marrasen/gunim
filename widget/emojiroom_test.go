package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/text"
)

// openPickerOn opens an emoji picker from a button at the window's top left, the screen's work area being area.
func openPickerOn(t *testing.T, area geom.Rect) (picker *EmojiPicker, run func(int), ui **gunim.UI) {
	t.Helper()
	if !text.EmojiShows("\U0001F44D") {
		t.Skip("no colour emoji font here")
	}
	picker = &EmojiPicker{}
	button := NewButton("Emoji")
	ui = new(*gunim.UI)
	button.OnClick = func(u *gunim.UI) gunim.Intent { *ui = u; picker.Open(button, geom.Rc(0, 0, 80, 36), u); return nil }
	var w *gunim.Window
	w, run = stage(t, &frame{child: button, size: geom.Sz(120, 36)})
	w.Offscreen().SetWorkArea(area)
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18), Button: input.ButtonPrimary})
	return picker, run, ui
}

// On a screen 300 wide and 360 tall, the picker keeps to the room below its button and to the screen's width, every
// frame of its opening, and its list still scrolls to the last group.
func TestTheEmojiPickerKeepsToASmallScreen(t *testing.T) {
	picker, run, u := openPickerOn(t, geom.Rc(0, 0, 300, 360))
	for i := range 30 {
		run(1)
		if !picker.IsOpen() {
			continue
		}
		pw := picker.popup.Offscreen()
		if s := pw.Size(); pw.Anchor().Max.Y+s.H > 360 || s.W > 300 {
			t.Fatalf("frame %d: the window is %v from y=%v, on a screen 300 by 360", i, s, pw.Anchor().Max.Y)
		}
		if c := picker.card; c.card.Size().W > 8*c.cell+2*c.pad+0.01 {
			t.Fatalf("frame %d: the card is %v wide for cells of %v", i, c.card.Size().W, c.cell)
		}
	}
	c := picker.card
	c.list.ScrollToKey(c.tabs.groups[len(c.tabs.groups)-1].key, *u)
	run(60)
	if c.tabs.shown != len(c.tabs.groups)-1 {
		t.Fatalf("scrolled to the last group, the tabs show group %d of %d", c.tabs.shown, len(c.tabs.groups))
	}
}
