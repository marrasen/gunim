package main

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// press presses k and lets it go.
func (h *harness) press(k input.Key, mods input.Mods) {
	h.w.Input(input.KeyPress{Key: k, Mods: mods, Time: time.Now()})
	h.w.Input(input.KeyRelease{Key: k, Mods: mods, Time: time.Now()})
	h.frames(2)
}

func TestTabReachesEveryPartOfTheWindowAndTheSidebarOpensAPlace(t *testing.T) {
	h := newHarness(t, "work/report.txt")
	h.do(Navigate{Path: filepath.Join(h.dir, "work")})
	h.do(Command{Name: CmdPin})
	h.do(Command{Name: CmdUp})
	h.until("the favourite shows and the folder is back", func() bool {
		return h.b.side.favs.Len() == 1 && len(h.shown()) == 1 && h.shown()[0] == "work"
	})
	h.frames(30)

	b := h.b
	want := map[gunim.Node]string{
		b.path.back:                 "Back",
		b.path.filter:               "the filter",
		b.side.places:               "the places",
		b.side.favs:                 "the favourites",
		b.listing.cur.focusNode():   "the listing",
		b.status.views.modes:        "the view switch",
		b.path.crumbs.crumbs[0].btn: "the first folder of the path",
	}
	seen := map[gunim.Node]bool{}
	var order []gunim.Node
	for range 40 {
		h.press(input.KeyTab, 0)
		f := h.focused()
		if f == nil {
			t.Fatal("Tab left nothing with the keyboard")
		}
		if seen[f] {
			break
		}
		seen[f] = true
		order = append(order, f)
	}
	for n, name := range want {
		if !seen[n] {
			t.Errorf("Tab never reached %s", name)
		}
	}
	for _, nb := range []*widget.IconButton{b.path.back, b.path.fwd, b.path.up} {
		if seen[nb] == nb.Disabled {
			t.Errorf("Tab reached %s %v times, and it is disabled %v", nb.Tooltip, seen[nb], nb.Disabled)
		}
	}
	if !b.path.fwd.Disabled {
		t.Error("Forward is enabled with nowhere to go")
	}
	// The folders of the path are one stop
	crumbs := 0
	for _, c := range b.path.crumbs.crumbs {
		if seen[c.btn] {
			crumbs++
		}
	}
	if crumbs != 1 {
		t.Errorf("Tab stopped at %d folders of the path, want 1", crumbs)
	}
	// The path bar comes before the sidebar, and the sidebar before the listing.
	at := func(n gunim.Node) int { return slices.Index(order, n) }
	if at(b.path.back) > at(b.side.places) || at(b.side.favs) > at(b.listing.cur.focusNode()) {
		t.Errorf("Tab went round in the order %v", order)
	}

	// The favourites take the arrows and Enter.
	for h.focused() != b.side.favs {
		h.press(input.KeyTab, 0)
	}
	h.press(input.KeyHome, 0)
	h.press(input.KeyEnter, 0)
	h.until("Enter on the favourite opens it", func() bool { return slices.Equal(h.shown(), []string{"report.txt"}) })
}

func TestAltAndALetterOpenTheFilesMenus(t *testing.T) {
	h := newHarness(t, "a.txt")
	h.w.Input(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt, Time: time.Now()})
	h.press(input.KeyV, input.ModAlt)
	h.w.Input(input.KeyRelease{Key: input.KeyLeftAlt, Time: time.Now()})
	h.frames(2)
	if !h.b.title.bar.IsOpen() {
		t.Fatal("Alt+V left the menus shut")
	}
	h.press(input.KeyEscape, 0)
	h.frames(20)
	if h.b.title.bar.IsOpen() {
		t.Fatal("Escape left a menu open")
	}
}

func TestTheArrowsMoveAlongThePathAndTabComesBackThere(t *testing.T) {
	h := newHarness(t, "a.txt")
	cs := h.b.path.crumbs.crumbs
	if len(cs) < 3 {
		t.Fatalf("the path has %d folders, want at least 3", len(cs))
	}
	for h.focused() != cs[0].btn {
		h.press(input.KeyTab, 0)
	}
	h.press(input.KeyRight, 0)
	if h.focused() != cs[1].btn {
		t.Fatal("Right did not move to the second folder")
	}
	h.press(input.KeyEnd, 0)
	if h.focused() != cs[len(cs)-1].btn {
		t.Fatal("End did not move to the last folder")
	}
	h.press(input.KeyLeft, 0)
	at := h.focused()
	h.press(input.KeyTab, 0)
	h.press(input.KeyTab, input.ModShift)
	if h.focused() != at {
		t.Fatal("Shift+Tab back into the path did not come back to the folder it left")
	}
	h.press(input.KeyHome, 0)
	if h.focused() != cs[0].btn {
		t.Fatal("Home did not move to the first folder")
	}
}
