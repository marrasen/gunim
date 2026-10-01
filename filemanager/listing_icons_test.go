package filemanager

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// writePics writes small pictures called names into dir, each in the format its extension names.
func writePics(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := writeImage(filepath.Join(dir, n), picture(60, 40, rings)); err != nil {
			t.Fatal(err)
		}
	}
}

// newPicHarness is a harness on a folder with the pictures named and the files spec names.
func newPicHarness(t *testing.T, pics []string, spec ...string) *harness {
	t.Helper()
	h := newHarness(t, spec...)
	writePics(t, h.dir, pics...)
	h.do(Command{Name: CmdRefresh})
	h.until("the pictures are listed", func() bool { return len(h.shown()) == len(pics)+len(spec) })
	return h
}

// icons returns the icon view of the page showing.
func (h *harness) icons() *iconView { return h.b.listing.cur.icons }

// focused returns the node with the keyboard.
func (h *harness) focused() gunim.Node {
	h.t.Helper()
	type ask struct{}
	var n gunim.Node
	gunim.RegisterPatch(h.w, "browser", func(_ *browser, _ ask, u *gunim.UI) { n = u.Focused() })
	if err := h.w.Client().Patch(string(browserID), ask{}); err != nil {
		h.t.Fatal(err)
	}
	h.frames(1)
	return n
}

func key(k input.Key, mods input.Mods) input.KeyPress {
	return input.KeyPress{Key: k, Mods: mods, Time: time.Now()}
}

func TestTheViewSwitchFliesAndIsRemembered(t *testing.T) {
	h := newHarness(t, "a.txt", "b.txt", "sub/c.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 3 })
	h.w.Input(key(input.Key2, input.ModControl))
	h.until("the icons come", func() bool { return h.icons().on })
	h.frames(3)
	if v := h.icons().in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("three frames into the switch the icons are %v of the way in, want between", v)
	}
	h.until("the icons settle", func() bool { return h.icons().in.Value() == 1 })
	if h.focused() != gunim.Node(h.icons().grid) {
		t.Fatal("the keyboard did not follow the listing to the icons")
	}
	p, err := loadPrefs(filepath.Join(h.root, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Views[h.dir] || !p.Icons {
		t.Fatalf("the settings say %v for the folder and %v for the rest, want icons", p.Views, p.Icons)
	}
	// A folder not seen yet takes the view last chosen, and the folder keeps its own.
	h.do(Navigate{Path: filepath.Join(h.dir, "sub")})
	h.until("the folder opens as icons", func() bool {
		return slices.Equal(h.shown(), []string{"c.txt"}) && h.icons().on
	})
	h.w.Input(key(input.Key1, input.ModControl))
	h.until("the details come back", func() bool { return !h.icons().on })
	h.do(Command{Name: CmdBack})
	h.until("the first folder shows as icons again", func() bool { return len(h.shown()) == 3 && h.icons().on })
}

func TestIconKeysMoveInTwoDirections(t *testing.T) {
	spec := make([]string, 0, 60)
	for i := range 60 {
		spec = append(spec, fmt.Sprintf("file%03d.txt", i))
	}
	h := newHarness(t, spec...)
	h.until("the rows arrive", func() bool { return len(h.shown()) == 60 })
	h.do(Command{Name: CmdViewIcons})
	h.until("the icons settle", func() bool { return h.icons().on && h.icons().in.Value() == 1 })
	cols := h.icons().grid.Columns()
	if cols < 2 {
		t.Fatalf("the icons have %d columns", cols)
	}
	press := func(k input.Key, mods input.Mods, want int) {
		t.Helper()
		h.w.Input(key(k, mods))
		h.until("the cursor moves to "+spec[want], func() bool { return h.a.nav.cursor == spec[want] })
	}
	press(input.KeyRight, 0, 0)
	press(input.KeyRight, 0, 1)
	press(input.KeyDown, 0, 1+cols)
	press(input.KeyEnd, 0, 59)
	press(input.KeyHome, 0, 0)
	press(input.KeyRight, input.ModShift, 1)
	press(input.KeyDown, input.ModShift, 1+cols)
	h.until("the run from the anchor is selected", func() bool { return len(h.a.nav.sel) == 2+cols })
	press(input.KeyPageDown, 0, min(59, 1+cols+cols*pageRows(h)))
	if !h.icons().grid.IsSelected(h.icons().gridCursor()) {
		t.Fatal("the tile the keyboard is on is not selected")
	}
	h.w.Input(key(input.KeyA, input.ModControl))
	h.until("Ctrl+A selects them all", func() bool { return len(h.a.nav.sel) == 60 })
}

// pageRows is how many rows of tiles Page Down moves.
func pageRows(h *harness) int {
	g := h.icons().grid
	return max(1, int(h.icons().box.H/(g.Size.H+8)))
}

func (iv *iconView) gridCursor() int {
	_, c := iv.grid.Selected()
	return c
}

func TestCtrlWheelSizesTheTiles(t *testing.T) {
	h := newHarness(t, "a.txt", "b.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.do(Command{Name: CmdViewIcons})
	h.until("the icons settle", func() bool { return h.icons().on && h.icons().in.Value() == 1 })
	r := h.bounds(func(b *browser) gunim.Node { return b.listing.cur.icons.grid })
	was := h.b.status.views.tile
	h.w.Input(input.Scroll{Pos: r.Center(), Notches: geom.Pt(0, 2), Mods: input.ModControl, Time: time.Now()})
	h.frames(2)
	now := h.b.status.views.tile
	if now <= was || h.icons().grid.Size.W != now {
		t.Fatalf("Ctrl with the wheel took the tiles from %v to %v, and the grid has %v", was, now, h.icons().grid.Size)
	}
	if h.b.status.views.slider.Value() != now {
		t.Fatalf("the slider shows %v, want %v", h.b.status.views.slider.Value(), now)
	}
	h.until("the size is saved", func() bool {
		p, err := loadPrefs(filepath.Join(h.root, "prefs.json"))
		return err == nil && p.Tile == now
	})
}
