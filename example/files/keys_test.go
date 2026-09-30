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

// places returns the folders the path bar shows.
func (h *harness) places() []widget.AddressPlace {
	h.t.Helper()
	type ask struct{}
	var out []widget.AddressPlace
	gunim.RegisterPatch(h.w, "browser", func(b *browser, _ ask, u *gunim.UI) { out = b.path.addr.Places(u) })
	if err := h.w.Client().Patch(string(browserID), ask{}); err != nil {
		h.t.Fatal(err)
	}
	h.frames(1)
	return out
}

// onPlace is the index of the folder of the path with the keyboard, or -1.
func (h *harness) onPlace() int {
	h.t.Helper()
	return slices.IndexFunc(h.places(), func(p widget.AddressPlace) bool { return p.Focused })
}

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
		b.path.back:               "Back",
		b.path.filter:             "the filter",
		b.side.places:             "the sidebar",
		b.listing.cur.focusNode(): "the listing",
		b.status.views.modes:      "the view switch",
	}
	seen := map[gunim.Node]bool{}
	var order []gunim.Node
	placeStops := 0
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
		if h.onPlace() >= 0 {
			placeStops++
		}
	}
	if seen[b.side.favs] {
		t.Error("Tab stopped at the favourites, want the sidebar one stop")
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
	if placeStops != 1 {
		t.Errorf("Tab stopped at %d folders of the path, want 1", placeStops)
	}
	// The path bar comes before the sidebar, and the sidebar before the listing.
	at := func(n gunim.Node) int { return slices.Index(order, n) }
	if at(b.path.back) > at(b.side.places) || at(b.side.places) > at(b.listing.cur.focusNode()) {
		t.Errorf("Tab went round in the order %v", order)
	}

	// Down walks from the places on into the favourites, which take Enter.
	for h.focused() != b.side.places {
		h.press(input.KeyTab, 0)
	}
	h.press(input.KeyEnd, 0)
	h.press(input.KeyDown, 0)
	if h.focused() != b.side.favs {
		t.Fatal("Down from the last place did not go on into the favourites")
	}
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
	n := len(h.places())
	if n < 3 {
		t.Fatalf("the path has %d folders, want at least 3", n)
	}
	for range 40 {
		if h.onPlace() == 0 {
			break
		}
		h.press(input.KeyTab, 0)
	}
	if h.onPlace() != 0 {
		t.Fatal("Tab never reached the first folder of the path")
	}
	h.press(input.KeyRight, 0)
	if h.onPlace() != 1 {
		t.Fatal("Right did not move to the second folder")
	}
	h.press(input.KeyEnd, 0)
	if h.onPlace() != n-1 {
		t.Fatal("End did not move to the last folder")
	}
	h.press(input.KeyLeft, 0)
	at := h.onPlace()
	h.press(input.KeyTab, 0)
	h.press(input.KeyTab, input.ModShift)
	if h.onPlace() != at {
		t.Fatal("Shift+Tab back into the path did not come back to the folder it left")
	}
	h.press(input.KeyHome, 0)
	if h.onPlace() != 0 {
		t.Fatal("Home did not move to the first folder")
	}
}

func TestTheMousesSideButtonsGoBackAndForward(t *testing.T) {
	h := newHarness(t, "sub/x.txt")
	h.do(Navigate{Path: filepath.Join(h.dir, "sub")})
	h.until("the folder opens", func() bool { return slices.Equal(h.shown(), []string{"x.txt"}) })
	side := func(b input.Button, over gunim.Node) {
		r := h.bounds(func(*browser) gunim.Node { return over })
		at := r.Center()
		h.w.Input(input.PointerDown{Pos: at, Button: b, Time: time.Now()})
		h.w.Input(input.PointerUp{Pos: at, Button: b, Time: time.Now()})
		h.frames(2)
	}
	// Back over the listing, forward over the sidebar, and back again over the preview
	side(input.ButtonBack, h.b.listing)
	h.until("Back goes to the folder before", func() bool { return slices.Equal(h.shown(), []string{"sub"}) })
	side(input.ButtonForward, h.b.side)
	h.until("Forward goes into the folder again", func() bool { return slices.Equal(h.shown(), []string{"x.txt"}) })
	side(input.ButtonBack, h.b.preview)
	h.until("Back over the preview goes back", func() bool { return slices.Equal(h.shown(), []string{"sub"}) })
}

func TestTypingAtTheListingGoesToTheNameTyped(t *testing.T) {
	h := newHarness(t, "apple.txt", "banana.txt", "berry.txt", "cherry.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 4 })
	h.do(FocusListing{})
	h.frames(2)
	typeText := func(s string) {
		h.w.Input(input.TextInput{Text: s, Time: time.Now()})
		h.frames(2)
	}
	typeText("b")
	h.until("b goes to banana", func() bool { return h.a.nav.cursor == "banana.txt" })
	typeText("e")
	h.until("be goes on to berry", func() bool { return h.a.nav.cursor == "berry.txt" })
	if len(h.a.nav.sel) != 1 || !h.a.nav.sel["berry.txt"] {
		t.Fatalf("the selection is %v, want berry alone", h.a.nav.sel)
	}
	// A pause starts the text again
	h.frames(70)
	typeText("a")
	h.until("after a pause, a goes to apple", func() bool { return h.a.nav.cursor == "apple.txt" })
}
