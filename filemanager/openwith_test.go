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
// Editor and Viewer for any file, and records what it was asked. With a
// gate, it answers once the gate lets it.
type openWithSystem struct {
	mu     sync.Mutex
	exts   []string
	opened []string // "id path", or "dialog path"
	gate   chan struct{}
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
		s.exts = append(s.exts, ext)
		gate := s.gate
		s.mu.Unlock()
		if gate != nil {
			<-gate
		}
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

// hold has the system wait to answer for the programs until the test
// ends, or until the function returned lets it.
func (s *openWithSystem) hold(t *testing.T) (answer func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	s.gate = gate
	s.mu.Unlock()
	var once sync.Once
	answer = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(answer)
	return answer
}

// withItems returns the labels of the Open with submenu of the listing's
// context menu, which are dimmed, and whether the menu is open.
func (h *harness) withItems() (items []string, off []bool, open bool) {
	h.t.Helper()
	const label = "Open with"
	h.ui(func(b *browser, _ *gunim.UI) {
		m := b.listing.cur.menu
		i := slices.Index(labelsOf(m.Items()), label)
		if i < 0 || m.Items()[i].Sub == nil {
			h.t.Fatalf("the menu %v has no %q with a submenu", labelsOf(m.Items()), label)
		}
		sub := m.Items()[i].Sub.Items
		items, off, open = labelsOf(sub), disabledOf(sub), m.Focusable()
	})
	return items, off, open
}

// pickWith picks the item sub in the Open with submenu of the listing's
// context menu.
func (h *harness) pickWith(sub string) {
	h.t.Helper()
	const label = "Open with"
	h.ui(func(b *browser, u *gunim.UI) {
		m := b.listing.cur.menu
		i := slices.Index(labelsOf(m.Items()), label)
		if i < 0 || m.Items()[i].Sub == nil {
			h.t.Fatalf("the menu %v has no %q with a submenu", labelsOf(m.Items()), label)
		}
		items := m.Items()[i].Sub.Items
		j := slices.Index(labelsOf(items), sub)
		if j < 0 || items[j].Disabled {
			h.t.Fatalf("the submenu %v has no %q to pick", labelsOf(items), sub)
		}
		m.OnPickSub([]int{i, j}, u)
	})
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
		if i := slices.Index(items, "Open with"); i < 0 || !off[i] {
			t.Fatalf("remote %v: Open with on a folder is in %v, dimmed %v", remote, items, off)
		}
		if exts, _ := sys.asked(); len(exts) != 0 {
			t.Fatalf("remote %v: a folder's menu asked for the programs of %v", remote, exts)
		}
		items, off = h.rowMenu("a.txt")
		if i := slices.Index(items, "Open with"); i < 0 || off[i] {
			t.Fatalf("remote %v: Open with on a file is in %v, dimmed %v", remote, items, off)
		}
		want := []string{"Editor", "Viewer", "Choose another app…"}
		h.until("the programs fill the submenu", func() bool { got, _, _ := h.withItems(); return slices.Equal(got, want) })
		if exts, _ := sys.asked(); !slices.Equal(exts, []string{".txt"}) {
			t.Fatalf("remote %v: the programs were asked for %v", remote, exts)
		}
		h.pickWith("Viewer")
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

		// Opened again, the menu shows the programs found before at once, and asks again.
		h.rowMenu("a.txt")
		if got, _, _ := h.withItems(); !slices.Equal(got, want) {
			t.Fatalf("remote %v: opened again, the submenu has %v", remote, got)
		}
		h.until("the programs are asked for again", func() bool { exts, _ := sys.asked(); return len(exts) == 2 })
		h.pickWith("Choose another app…")
		h.until("the dialog opens", func() bool { _, o := sys.asked(); return len(o) == 2 })
		if _, o := sys.asked(); !strings.HasPrefix(o[1], "dialog ") || filepath.Base(o[1]) != "a.txt" {
			t.Fatalf("remote %v: asked %v", remote, o)
		}
	}
}

func TestOpenWithLooksForTheProgramsWhileItsMenuIsOpen(t *testing.T) {
	sys := fakeOpenWith(t, true)
	answer := sys.hold(t)
	h, _, _ := newFetchHarness(t, nil, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.rowMenu("a.txt")
	h.until("the programs are asked for", func() bool { exts, _ := sys.asked(); return len(exts) == 1 })
	items, off, open := h.withItems()
	if !slices.Equal(items, []string{"Looking for apps…", "Choose another app…"}) || !off[0] || off[1] || !open {
		t.Fatalf("before the programs come the submenu has %v, dimmed %v, the menu open %v", items, off, open)
	}
	answer()
	h.until("the programs fill the submenu in place", func() bool {
		items, _, open := h.withItems()
		return open && slices.Equal(items, []string{"Editor", "Viewer", "Choose another app…"})
	})
}

func TestOpenWithStillChoosesAnotherAppWhereNoProgramsCome(t *testing.T) {
	sys := fakeOpenWith(t, true)
	sys.hold(t)
	h, _, _ := newFetchHarness(t, nil, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.rowMenu("a.txt")
	h.until("the programs are asked for", func() bool { exts, _ := sys.asked(); return len(exts) == 1 })
	h.pickWith("Choose another app…")
	h.until("the dialog opens", func() bool { _, o := sys.asked(); return len(o) == 1 })
	if _, o := sys.asked(); o[0] != "dialog "+filepath.Join(h.dir, "a.txt") {
		t.Fatalf("asked %v", o)
	}
}

func TestOpenWithIsNotOfferedWhereTheSystemHasNoPrograms(t *testing.T) {
	fakeOpenWith(t, false)
	h, _, _ := newFetchHarness(t, nil, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if items, _ := h.rowMenu("a.txt"); slices.Contains(items, "Open with") {
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
	sys := fakeOpenWith(t, true)
	sys.hold(t)
	h, _, _ := newFetchHarness(t, nil, "a.txt", "b.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.rowMenu("a.txt")
	// b.txt's menu opens before the answer for a.txt comes.
	h.rowMenu("b.txt")
	h.until("the programs are asked for both", func() bool { exts, _ := sys.asked(); return len(exts) == 2 })
	h.ui(func(b *browser, u *gunim.UI) {
		b.dnd.openWithMenu(OpenWithMenu{Path: filepath.Join(h.dir, "a.txt"), Apps: []OpenWithApp{{ID: "x", Name: "X"}}}, u)
	})
	if got, _, _ := h.withItems(); !slices.Equal(got, []string{"Looking for apps…", "Choose another app…"}) {
		t.Fatalf("b.txt's submenu turned into %v", got)
	}
}
