package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
)

func TestAChipShowsItsIconBeforeItsLabel(t *testing.T) {
	plain := NewChip("", "Images")
	c := NewChip("", "Images")
	c.Icon = icon.Image
	c.OnRemove = func() gunim.Intent { return removed{"Images"} }
	stage(t, Row(plain))
	w, run := stage(t, &frame{child: Row(c), size: geom.Sz(400, 40)})
	if d := c.size.W - plain.size.W; d != IconSize.Default()+IconGap.Default() {
		t.Fatalf("the chip with an icon is %v wider, want room for the icon and its gap", d)
	}
	var found bool
	for _, m := range maskOps(w.Offscreen()) {
		if strokeOf(t, m).Icon != icon.Image {
			continue
		}
		found = true
		h := ChipHeight.Default()
		if m.Rect != geom.Rc(h/2, (h-IconSize.Default())/2, IconSize.Default(), IconSize.Default()) {
			t.Errorf("the icon is at %v, want at the chip's start", m.Rect)
		}
		if m.Color != Ink.Default() {
			t.Errorf("the icon is %v, want the label's colour", m.Color)
		}
	}
	if !found {
		t.Fatal("the chip drew no icon")
	}
	click(w, c.crossX, 12)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (removed{"Images"}) {
		t.Fatalf("a click on the cross sent %v, want the chip removed", got)
	}
}
