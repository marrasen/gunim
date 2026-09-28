package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

func TestADropdownShowsTheChosenItemsIcon(t *testing.T) {
	plain := NewDropdown("Details", "Icons")
	d := NewDropdown("Details", "Icons")
	d.Icons = []*icon.Icon{icon.List, icon.LayoutGrid}
	d.Selected = 1
	w, run := stage(t, Column(plain, d))
	run(2)
	if gap := d.size.W - plain.size.W; gap != IconSize.Default()+IconGap.Default() {
		t.Fatalf("the drop-down with icons is %v wider, want room for an icon and its gap", gap)
	}
	want := []*icon.Icon{icon.ChevronDown, icon.LayoutGrid, icon.ChevronDown}
	if got := iconsDrawn(t, w); !slices.Equal(got, want) {
		t.Fatalf("the drop-downs drew %v, want their chevrons and the chosen item's icon", got)
	}
	click(w, d.size.W/2, plain.size.H+d.size.H/2+1)
	run(10)
	if d.menu == nil || !slices.Equal(d.menu.Icons, d.Icons) {
		t.Fatal("the drop-down's list does not show the items' icons")
	}
}

func TestAMenuButtonShowsItsIconAndItsItemsIcons(t *testing.T) {
	b := NewMenuButton("Columns", "Time", "Level")
	b.Icon = icon.Columns3
	b.Icons = []*icon.Icon{icon.Clock, icon.Signal}
	w, run := stage(t, Row(b))
	run(2)
	if got := iconsDrawn(t, w); !slices.Equal(got, []*icon.Icon{icon.Columns3, icon.ChevronDown}) {
		t.Fatalf("the menu button drew %v, want its own icon and its chevron", got)
	}
	click(w, b.size.W/2, b.size.H/2)
	run(10)
	if b.menu == nil || !slices.Equal(b.menu.Icons, b.Icons) {
		t.Fatal("the menu button's menu does not show the items' icons")
	}
}

func TestAContextMenuShowsItsItemsIcons(t *testing.T) {
	w, c, run := contextStage(t)
	c.Prepare = func(geom.Point, *gunim.UI) bool {
		c.Items, c.Icons = []string{"Copy"}, []*icon.Icon{icon.Copy}
		return true
	}
	w.Input(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(50, 50), Button: input.ButtonSecondary, Time: time.Now()})
	run(10)
	if c.menu == nil || !slices.Equal(c.menu.Icons, []*icon.Icon{icon.Copy}) {
		t.Fatal("the context menu's menu does not show the items' icons")
	}
}
