package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
)

func TestACheckedMenuItemShowsACheck(t *testing.T) {
	m := NewMenu("Hidden files", "Extensions", "Preview")
	m.Checked = []bool{true, false, true}
	w, _ := stage(t, Row(m))
	ms := maskOps(w.Offscreen())
	if len(ms) != 2 {
		t.Fatalf("the menu drew %d marks, want a check on each of two items", len(ms))
	}
	s := IconSize.Default()
	for k, i := range []int{0, 2} {
		if got := strokeOf(t, ms[k]).Icon; got != icon.Check {
			t.Fatalf("item %d is marked with %s, want a check", i, got.Name)
		}
		want := geom.Rc(m.card.Min.X+MenuRowPadding.Default()+menuTick/2-1-s/2, m.rowY(i)+(m.row-s)/2, s, s)
		if ms[k].Rect != want || ms[k].Color != Ink.Default() {
			t.Errorf("item %d's check is at %v in %v, want at %v in the ink", i, ms[k].Rect, ms[k].Color, want)
		}
	}
}
