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
	c = NewContextMenu(&block{h: 300})
	c.Prepare = func(at geom.Point, _ *gunim.UI) bool {
		if at.Y >= 150 {
			return false
		}
		c.Items = []string{"Open", "Paste", "Delete"}
		c.Hints = []string{"Enter", "Ctrl+V", "Delete"}
		c.Disabled = []bool{false, true, false}
		c.Breaks = []int{2}
		return true
	}
	c.OnPick = func(i int) gunim.Intent { return menuPicked{c.Items[i]} }
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
	if !slices.Equal(c.menu.Hints, []string{"Enter", "Ctrl+V", "Delete"}) || !flag(c.menu.Disabled, 1) {
		t.Fatalf("the menu has hints %v and disabled %v", c.menu.Hints, c.menu.Disabled)
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
