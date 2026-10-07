package filemanager

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/marrasen/gunim"
)

// openWithSystem stands in for the system's side of Open with: it offers
// Editor and Viewer for any file, and records what it was asked.
type openWithSystem struct {
	mu     sync.Mutex
	exts   []string
	opened []string // "id path", or "dialog path"
}

// asked returns the extensions the programs were asked for, and what was
// opened.
func (s *openWithSystem) asked() (exts, opened []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.exts), slices.Clone(s.opened)
}

// fakeOpenWith has the window's Open with use s for the test, and works
// says whether the system offers it at all.
func fakeOpenWith(t *testing.T, works bool) *openWithSystem {
	s := &openWithSystem{}
	works0, apps0, app0, dialog0 := openWithWorks, openWithApps, openWithApp, openWithDialog
	openWithWorks = func() bool { return works }
	openWithApps = func(ext string) ([]OpenWithApp, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.exts = append(s.exts, ext)
		return []OpenWithApp{{ID: "editor.exe", Name: "Editor"}, {ID: "viewer.exe", Name: "Viewer"}, {ID: "editor.exe", Name: "Editor"}}, nil
	}
	openWithApp = func(p, id string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.opened = append(s.opened, id+" "+p)
		return nil
	}
	openWithDialog = func(p string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.opened = append(s.opened, "dialog "+p)
		return nil
	}
	t.Cleanup(func() {
		openWithWorks, openWithApps, openWithApp, openWithDialog = works0, apps0, app0, dialog0
	})
	return s
}

// pickInMenu picks the item labelled in the listing's context menu.
func (h *harness) pickInMenu(label string) {
	h.t.Helper()
	h.ui(func(b *browser, u *gunim.UI) {
		m := b.listing.cur.menu
		i := slices.Index(m.Items, label)
		if i < 0 {
			h.t.Fatalf("the menu %v has no %q", m.Items, label)
		}
		if m.Disabled[i] {
			h.t.Fatalf("%q is dimmed", label)
		}
		m.Picked(i, u)
	})
}

// menuItems returns the items of the listing's context menu.
func (h *harness) menuItems() []string {
	var items []string
	h.ui(func(b *browser, _ *gunim.UI) { items = slices.Clone(b.listing.cur.menu.Items) })
	return items
}

func TestOpenWithOffersTheSystemsProgramsForAFile(t *testing.T) {
	for _, remote := range []bool{false, true} {
		var set func(o *Options)
		if remote {
			set = onBareFS
		}
		sys := fakeOpenWith(t, true)
		h, _, _ := newFetchHarness(t, set, "a.txt", "sub/")
		h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
		items, off := h.rowMenu("sub")
		if i := slices.Index(items, "Open with…"); i < 0 || !off[i] {
			t.Fatalf("remote %v: Open with on a folder is in %v, dimmed %v", remote, items, off)
		}
		items, off = h.rowMenu("a.txt")
		if i := slices.Index(items, "Open with…"); i < 0 || off[i] {
			t.Fatalf("remote %v: Open with on a file is in %v, dimmed %v", remote, items, off)
		}
		h.pickInMenu("Open with…")
		want := []string{"Editor", "Viewer", "Choose another app…"}
		h.until("the programs' menu opens", func() bool { return slices.Equal(h.menuItems(), want) })
		if exts, _ := sys.asked(); !slices.Equal(exts, []string{".txt"}) {
			t.Fatalf("remote %v: the programs were asked for %v", remote, exts)
		}
		h.pickInMenu("Viewer")
		h.until("the file opens with Viewer", func() bool { _, o := sys.asked(); return len(o) == 1 })
		h.idle()
		_, opened := sys.asked()
		id, p, _ := strings.Cut(opened[0], " ")
		if id != "viewer.exe" {
			t.Fatalf("remote %v: opened with %q", remote, id)
		}
		if remote {
			if filepath.Base(p) != "a.txt" || contents(t, p) != "a.txt" {
				t.Fatalf("remote: opened %s, not a copy of a.txt", p)
			}
		} else if p != filepath.Join(h.dir, "a.txt") {
			t.Fatalf("opened %s", p)
		}

		h.rowMenu("a.txt")
		h.pickInMenu("Open with…")
		h.until("the programs' menu opens again", func() bool { return slices.Equal(h.menuItems(), want) })
		h.pickInMenu("Choose another app…")
		h.until("the dialog opens", func() bool { _, o := sys.asked(); return len(o) == 2 })
		if _, o := sys.asked(); !strings.HasPrefix(o[1], "dialog ") || filepath.Base(o[1]) != "a.txt" {
			t.Fatalf("remote %v: asked %v", remote, o)
		}
	}
}

func TestOpenWithIsNotOfferedWhereTheSystemHasNoPrograms(t *testing.T) {
	fakeOpenWith(t, false)
	h, _, _ := newFetchHarness(t, nil, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if items, _ := h.rowMenu("a.txt"); slices.Contains(items, "Open with…") {
		t.Fatalf("the menu %v offers Open with", items)
	}
}

func TestExtOfTakesTheLastDot(t *testing.T) {
	for name, want := range map[string]string{"a.txt": ".txt", "a.tar.gz": ".gz", "README": "", ".gitignore": ".gitignore"} {
		if got := extOf(name); got != want {
			t.Errorf("extOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAnOpenWithAnswerForAnotherMenuIsDropped(t *testing.T) {
	fakeOpenWith(t, true)
	h, _, _ := newFetchHarness(t, nil, "a.txt", "b.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.rowMenu("a.txt")
	h.pickInMenu("Open with…")
	h.until("the programs' menu opens", func() bool { return slices.Contains(h.menuItems(), "Viewer") })
	// Asked for a.txt again, b.txt's menu opens before the answer comes.
	h.ui(func(b *browser, u *gunim.UI) {
		b.dnd.withAsked = filepath.Join(h.dir, "a.txt")
	})
	items, _ := h.rowMenu("b.txt")
	h.ui(func(b *browser, u *gunim.UI) {
		b.dnd.openWithMenu(OpenWithMenu{Path: filepath.Join(h.dir, "a.txt"), Apps: []OpenWithApp{{ID: "x", Name: "X"}}}, u)
	})
	if got := h.menuItems(); !slices.Equal(got, items) {
		t.Fatalf("b.txt's menu %v turned into %v", items, got)
	}
}
