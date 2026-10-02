package filemanager

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// clickWith presses and lets go at p with mods held, as the clicks-th click
// of a run of them.
func (h *harness) clickWith(p geom.Point, mods input.Mods, clicks int) {
	h.w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Mods: mods, Clicks: clicks, Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Mods: mods, Time: time.Now()})
	h.frames(2)
}

// placeAt is where to click the row of the sidebar's places with key k.
func (h *harness) placeAt(k widget.Key) geom.Point {
	h.t.Helper()
	r := h.bounds(func(b *browser) gunim.Node {
		n, _ := b.side.places.Row(k)
		return n
	})
	return geom.Pt(r.Center().X, r.Max.Y-10)
}

func TestCtrlClickOnAPlaceOpensOneWindow(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	var opened []string
	h := newHarnessWith(t, func(o *Options) {
		dir := o.Dir
		o.Places = func() ([]Place, error) {
			return []Place{
				{Name: "Work", Path: filepath.Join(dir, "work"), Kind: "home"},
				{Name: "Server", Path: "/home/me", Kind: "home", Group: "Servers", FS: "sftp://server"},
			}, nil
		}
		o.Visit = func(_ *Window, fs, path string, newWindow bool) {
			mu.Lock()
			defer mu.Unlock()
			what := "here"
			if newWindow {
				what = "new"
			}
			visits = append(visits, what+" "+fs+" "+path)
		}
	}, "work/", "a.txt")
	h.a.hub.open = func(o Options) error {
		mu.Lock()
		defer mu.Unlock()
		opened = append(opened, o.Dir)
		return nil
	}
	h.until("the places arrive", func() bool { return h.b.side.places.Len() == 2 })
	h.frames(30)
	work, srv := placeKey(Place{Path: filepath.Join(h.dir, "work")}), placeKey(Place{FS: "sftp://server", Path: "/home/me"})
	seen := func(what string, got *[]string, want ...string) {
		t.Helper()
		h.until(what, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return slices.Equal(*got, want)
		})
		h.frames(10)
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(*got, want) {
			t.Fatalf("%s: got %q", what, *got)
		}
	}

	h.clickWith(h.placeAt(work), input.ModControl, 1)
	seen("a Ctrl+click opens the place in a window", &opened, filepath.Join(h.dir, "work"))
	if !SystemPaths.Same(h.a.nav.path, h.dir) {
		t.Fatalf("the window went to %s itself", h.a.nav.path)
	}

	at := h.placeAt(work)
	h.clickWith(at, input.ModControl, 1)
	h.clickWith(at, input.ModControl, 2)
	seen("a Ctrl+double-click opens one window more", &opened, filepath.Join(h.dir, "work"), filepath.Join(h.dir, "work"))

	at = h.placeAt(srv)
	h.clickWith(at, input.ModControl, 1)
	seen("a Ctrl+click elsewhere asks for a new window", &visits, "new sftp://server /home/me")
	h.clickWith(at, input.ModControl, 1)
	h.clickWith(at, input.ModControl, 2)
	seen("a Ctrl+double-click elsewhere asks once", &visits, "new sftp://server /home/me", "new sftp://server /home/me")
	h.clickWith(at, 0, 1)
	h.clickWith(at, 0, 2)
	seen("a double-click elsewhere asks once, for this window", &visits, "new sftp://server /home/me",
		"new sftp://server /home/me", "here sftp://server /home/me")

	h.clickWith(h.placeAt(work), 0, 1)
	h.until("a click goes to the place", func() bool { return SystemPaths.Same(h.a.nav.path, filepath.Join(h.dir, "work")) })
	seen("a click opens no window", &opened, filepath.Join(h.dir, "work"), filepath.Join(h.dir, "work"))
}

func TestTurningToAnotherFileSystemKeepsTheSidebarRows(t *testing.T) {
	other := t.TempDir()
	tree(t, other, "x.txt")
	store := newAnyStore()
	var mu sync.Mutex
	var visits []string
	h := newAnyFSHarness(t, store, &visits, &mu)
	// Once the window turns, the places are slow to come, as a server's
	// may be, and the window shows those it had meanwhile.
	gate := make(chan struct{})
	slow := false
	h.a.opts.Places = func() ([]Place, error) {
		mu.Lock()
		wait := slow
		mu.Unlock()
		if wait {
			<-gate
		}
		return []Place{
			{Name: "Here", Path: h.dir, Kind: "home", Group: "This computer"},
			{Name: "Slash", Path: "/", Kind: "drive", Group: "Servers", FS: "slash"},
			{Name: "Also here", Path: "/", Kind: "drive", Group: "This computer"},
		}, nil
	}
	h.a.loadPlaces()
	h.until("the places arrive", func() bool { return h.b.side.places.Len() == 3 && h.b.side.favs.Len() == 2 })
	h.frames(60)
	type row struct {
		key  widget.Key
		node gunim.Node
	}
	rows := func() []row {
		var out []row
		for _, l := range []*widget.List{h.b.side.places, h.b.side.favs} {
			for _, k := range l.Keys() {
				n, _ := l.Row(k)
				out = append(out, row{k, n})
			}
		}
		return out
	}
	before := rows()
	if before[1].key == before[2].key {
		t.Fatal("the same path on two file systems has one key")
	}

	mu.Lock()
	slow = true
	mu.Unlock()
	h.a.showFS(slashFS{root: other}, "/")
	steady := func() {
		t.Helper()
		for range 60 {
			h.pump()
			if got := rows(); !slices.Equal(got, before) {
				t.Fatalf("the sidebar's rows changed from %v to %v", before, got)
			}
		}
	}
	h.until("the window shows the other file system", func() bool { return slices.Equal(h.shown(), []string{"x.txt"}) })
	steady()
	close(gate)
	steady()
	here, slash := h.b.side.items[before[0].key], h.b.side.items[before[1].key]
	if !here.away || slash.away || !slash.current {
		t.Fatalf("after the turn the computer's place is %+v and the server's %+v", here, slash)
	}
	if fav := h.b.side.items[before[3].key]; !fav.away {
		t.Fatalf("the favourite of the computer is %+v, want it away", fav)
	}
}
