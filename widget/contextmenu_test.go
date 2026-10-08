package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type menuPicked struct{ item string }

// contextStage holds a context menu whose items depend on where it is
// pressed: the top half has Open, Paste and Delete, with Paste disabled,
// and the bottom half none.
func contextStage(t *testing.T) (w *gunim.Window, c *ContextMenu, run func(int)) {
	t.Helper()
	c = NewContextMenu(&block{h: 300}, nil)
	c.Prepare = func(at geom.Point, _ *gunim.UI) bool {
		if at.Y >= 150 {
			return false
		}
		c.SetItems([]MenuItem{
			{Label: "Open", Hint: "Enter"},
			{Label: "Paste", Hint: "Ctrl+V", Disabled: true},
			{Label: "Delete", Hint: "Delete", Break: true},
		})
		return true
	}
	c.OnPick = func(i int, u *gunim.UI) gunim.Intent { return menuPicked{c.Items()[i].Label} }
	w, run = stage(t, &frame{child: c, size: geom.Sz(400, 300)})
	return w, c, run
}

func TestAContextMenuTakesItsItemsFromWhereItOpens(t *testing.T) {
	w, c, run := contextStage(t)
	w.Input(input.PointerDown{Pos: geom.Pt(50, 200), Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	run(2)
	if c.Focusable() {
		t.Fatal("a press where Prepare says no opened a menu")
	}
	w.Input(input.PointerUp{Pos: geom.Pt(50, 200), Button: input.ButtonSecondary, Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Time: time.Now()})
	run(10)
	if !c.Focusable() || c.menu == nil {
		t.Fatal("the menu did not open")
	}
	if items := c.menu.Items(); !slices.Equal(hintsOf(items), []string{"Enter", "Ctrl+V", "Delete"}) || !items[1].Disabled {
		t.Fatalf("the menu has hints %v and disabled %v", hintsOf(items), disabledOf(items))
	}
	// Down from Open passes the disabled Paste by.
	w.Input(input.KeyPress{Key: input.KeyDown, Time: time.Now()})
	w.Input(input.KeyPress{Key: input.KeyDown, Time: time.Now()})
	w.Input(input.KeyPress{Key: input.KeyEnter, Time: time.Now()})
	run(2)
	if got := sent(w); len(got) != 1 || got[0] != (menuPicked{"Delete"}) {
		t.Fatalf("the keys picked %v, want Delete", got)
	}
}

func TestAContextMenuCanActInsideTheWindow(t *testing.T) {
	w, c, run := contextStage(t)
	var local []int
	c.OnPick = func(i int, _ *gunim.UI) gunim.Intent {
		local = append(local, i)
		return nil
	}
	w.Input(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Time: time.Now()})
	run(5)
	w.Input(input.KeyPress{Key: input.KeyDown, Time: time.Now()})
	w.Input(input.KeyPress{Key: input.KeyEnter, Time: time.Now()})
	run(2)
	if !slices.Equal(local, []int{0}) {
		t.Fatalf("Picked heard %v, want the first item", local)
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("an OnPick of nil sent %v", got)
	}
}

// fileMenu is a file manager's context menu on a file: seventeen items in four groups.
func fileMenu() []MenuItem {
	labels := [][]string{
		{"Open", "Open with system", "Open with…", "Open in new window", "Show in system file manager", "Compare", "Send to"},
		{"Cut", "Copy", "Paste"},
		{"Rename", "Duplicate", "Move to trash", "Delete permanently"},
		{"Pin to favourites", "Copy path", "Properties"},
	}
	var items []MenuItem
	for _, group := range labels {
		for i, l := range group {
			items = append(items, MenuItem{Label: l, Break: i == 0 && len(items) > 0})
		}
	}
	return items
}

// openContextMenuIn opens a context menu of items, over all of a window 800 by 600 on a screen of the same size, by
// a press at at, and returns it with the size of the menu's window.
func openContextMenuIn(t *testing.T, items []MenuItem, at geom.Point) (*ContextMenu, geom.Size) {
	t.Helper()
	c := NewContextMenu(&block{h: 600}, items)
	w, run := stage(t, &frame{child: c, size: geom.Sz(800, 600)})
	w.Offscreen().SetWorkArea(geom.Rc(0, 0, 800, 600))
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonSecondary, Time: time.Now()})
	run(20)
	if c.menu == nil || c.popup == nil {
		t.Fatal("the menu did not open")
	}
	return c, c.popup.Offscreen().Size()
}

func TestAFileMenuOpenedLowInTheWindowShowsAllItsItems(t *testing.T) {
	// About 520 tall: more than the room above or below the press, but not the screen's.
	c, size := openContextMenuIn(t, fileMenu(), geom.Pt(300, 380))
	if c.menu.scroll.scrollable() {
		t.Fatalf("a menu %v tall scrolls, on a screen 600 tall", c.menu.content()+2*c.menu.pad)
	}
	if size.H > 600 {
		t.Fatalf("the menu's window is %v, on a screen 600 tall", size)
	}
}

func TestAContextMenuTallerThanTheScreenScrolls(t *testing.T) {
	c, size := openContextMenuIn(t, Labels(slices.Repeat([]string{"Item"}, 40)...), geom.Pt(300, 380))
	if !c.menu.scroll.scrollable() {
		t.Fatal("40 items on a screen 600 tall do not scroll")
	}
	if size.H > 600 {
		t.Fatalf("the menu's window is %v, on a screen 600 tall", size)
	}
}
