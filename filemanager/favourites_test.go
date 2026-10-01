package filemanager

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

// anyStore is a favourites store whose favourites may be on any file
// system, which it names by their IDs.
type anyStore struct {
	*memStore
	names map[string]string
}

func (s anyStore) Where(fs string) string { return s.names[fs] }

// server is the file system of the favourites elsewhere in these tests.
const server = "sftp://server"

// newAnyFSHarness opens a window on the computer's own file system whose
// favourites are a folder of its own, old, and data on the server. The
// visits the window asks for are kept in visits.
func newAnyFSHarness(t *testing.T, store anyStore, visits *[]string, mu *sync.Mutex) *harness {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	tree(t, dir, "old/", "work/", "a.txt")
	store.favs = []Favourite{{Path: filepath.Join(dir, "old")}, {Path: "/srv/data", FS: server}}
	return openHarnessWith(t, nil, root, dir, func(o *Options) {
		o.Favourites = store
		o.Visit = func(_ *Window, fs, path string) {
			mu.Lock()
			defer mu.Unlock()
			*visits = append(*visits, fs+" "+path)
		}
	})
}

func newAnyStore() anyStore {
	return anyStore{memStore: &memStore{}, names: map[string]string{server: "Server"}}
}

func TestFavouritesOnAnyFileSystemAreListedAndVisited(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	store := newAnyStore()
	h := newAnyFSHarness(t, store, &visits, &mu)
	h.until("both favourites show", func() bool { return h.b.side.favs.Len() == 2 })
	keys := h.b.side.favs.Keys()
	own, away := h.b.side.items[keys[0]], h.b.side.items[keys[1]]
	if own.away || own.FS != "" || own.Note != "" || own.Name != "old" {
		t.Fatalf("the favourite of the window's own file system is %+v", own)
	}
	if !away.away || away.FS != server || away.Note != "Server" || away.Name != "data" {
		t.Fatalf("the favourite on the server is %+v, want it away and saying where", away)
	}
	h.frames(30)
	row := h.bounds(func(b *browser) gunim.Node {
		n, _ := b.side.favs.Row(keys[1])
		return n
	})
	h.click(geom.Pt(row.Center().X, row.Min.Y+10))
	h.until("the click asks to visit the server", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Equal(visits, []string{server + " /srv/data"})
	})
	if !SystemPaths.Same(h.a.nav.path, h.dir) {
		t.Fatalf("the window went to %s itself", h.a.nav.path)
	}

	hits := rankCommands("data", nil, h.a.favPlaces(), h.a.fs.ID())
	if len(hits) == 0 || hits[0].Key != "visit:"+server+"\x00/srv/data" || hits[0].Detail != "Server: /srv/data" {
		t.Fatalf("the palette offers %+v for the favourite on the server", hits)
	}
	h.do(PalettePicked{Key: hits[0].Key})
	h.until("the palette asks to visit the server", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(visits) == 2 && visits[1] == server+" /srv/data"
	})
	if !SystemPaths.Same(h.a.nav.path, h.dir) {
		t.Fatalf("the palette went to %s itself", h.a.nav.path)
	}
	// A file system's ID may hold a NUL, as a machine beyond another's
	// does; the path is what follows the last.
	h.do(PalettePicked{Key: "visit:far\x00k1\x00/var/log"})
	h.until("the palette asks to visit the far machine", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(visits) == 3 && visits[2] == "far\x00k1 /var/log"
	})
}

func TestPinningKeepsTheFavouritesElsewhere(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	store := newAnyStore()
	h := newAnyFSHarness(t, store, &visits, &mu)
	h.pick("work")
	h.do(Command{Name: CmdPin})
	want := []Favourite{{Path: filepath.Join(h.dir, "old")}, {Path: "/srv/data", FS: server}, {Path: filepath.Join(h.dir, "work")}}
	h.until("the store holds all three", func() bool { return slices.Equal(store.saved(), want) })
	// The same path on the server is a favourite of its own.
	store.mu.Lock()
	store.favs = append(store.favs, Favourite{Path: filepath.Join(h.dir, "work"), FS: server})
	store.mu.Unlock()
	h.a.hub.Refresh()
	h.until("the sidebar shows all four", func() bool { return h.b.side.favs.Len() == 4 })
	h.pick("work")
	h.do(Command{Name: CmdPin})
	if len(store.saved()) != 4 {
		t.Fatalf("pinning a favourite again saved %+v", store.saved())
	}
}

func TestAFavouriteElsewhereIsRenamedReorderedAndUnpinned(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	store := newAnyStore()
	h := newAnyFSHarness(t, store, &visits, &mu)
	h.until("both favourites show", func() bool { return h.b.side.favs.Len() == 2 })
	old := filepath.Join(h.dir, "old")

	var items []string
	h.ui(func(b *browser, u *gunim.UI) {
		m, _ := b.dnd.side.Children()[0].(*widget.ContextMenu)
		row, found := b.side.favs.Row(placeKey(Place{FS: server, Path: "/srv/data"}, true))
		if m == nil || !found {
			t.Fatal("the sidebar has no menu, or no row for the favourite on the server")
		}
		mr, _ := u.Bounds(m)
		rr, _ := u.Bounds(row)
		m.Open(rr.Center().Sub(mr.Min), u)
		items = slices.Clone(m.Items)
	})
	if !slices.Equal(items, []string{"Open", "Rename favourite", "Unpin"}) {
		t.Fatalf("the menu of the favourite on the server is %v", items)
	}

	h.do(RenameFavourite{FS: server, Path: "/srv/data"})
	h.answer(Prompted{Token: h.a.ops.tokens, Text: "Data", OK: true})
	h.until("the favourite takes its name", func() bool { return h.a.favName(server, "/srv/data") == "Data" })

	var reorder gunim.Intent
	h.ui(func(b *browser, _ *gunim.UI) {
		keys := b.side.favs.Keys()
		slices.Reverse(keys)
		reorder = b.side.favs.Reorder(keys)
	})
	h.do(reorder)
	want := []Favourite{{Path: "/srv/data", Name: "Data", FS: server}, {Path: old}}
	h.until("the store holds them reordered", func() bool { return slices.Equal(store.saved(), want) })

	// The path of the folder of the window's own is not the favourite on
	// the server.
	h.do(Unpin{FS: server, Path: old})
	h.do(Unpin{FS: server, Path: "/srv/data"})
	h.until("the store holds the window's own", func() bool { return slices.Equal(store.saved(), []Favourite{{Path: old}}) })
}

func TestFilesDraggedOverAFavouriteElsewhereAreRefused(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	store := newAnyStore()
	h := newAnyFSHarness(t, store, &visits, &mu)
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:data")
	var spot widget.DropSpot
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.side.Over() })
	if hint, _ := spot.Hint.(widget.DropHint); !spot.Refused || hint.Text != "Cannot drop on another file system" {
		t.Fatalf("over the favourite on the server the drop says %v, refused %v", spot.Hint, spot.Refused)
	}
	h.script("drop")
	h.frames(10)
	if len(h.a.ops.running) != 0 || !h.exists("a.txt") || len(store.saved()) != 2 {
		t.Fatal("a drop on the favourite on the server did something")
	}
}

func TestAStoreOfOneFileSystemKeepsItsFavouritesWithoutOne(t *testing.T) {
	store := &memStore{}
	h := newHarnessWith(t, func(o *Options) {
		onBareFS(o)
		o.Favourites = store
	}, "work/")
	h.pick("work")
	h.do(Command{Name: CmdPin})
	work := filepath.Join(h.dir, "work")
	h.until("the store holds the favourite", func() bool { return slices.Equal(store.saved(), []Favourite{{Path: work}}) })
	if len(h.a.favs) != 1 || h.a.favs[0].FS != "elsewhere" {
		t.Fatalf("the window holds %+v, want the favourite on its file system", h.a.favs)
	}
	h.until("the favourite shows", func() bool { return h.b.side.favs.Len() == 1 })
	if it := h.b.side.items[widget.Key(work)]; it.away || it.Note != "" {
		t.Fatalf("the favourite shows as %+v", it)
	}
	h.do(Unpin{FS: "elsewhere", Path: work})
	h.until("the store is empty", func() bool { return len(store.saved()) == 0 })
}

func TestWindowsOnTwoFileSystemsShareTheirFavourites(t *testing.T) {
	root := t.TempDir()
	local, other := filepath.Join(root, "local"), filepath.Join(root, "other")
	tree(t, local, "work/")
	tree(t, other, "far/")
	// A pointer, as a store that cannot be compared is not shared.
	store := newAnyStore()
	hub := &Hub{}
	l := openHarnessWith(t, hub, root, local, func(o *Options) { o.Favourites = &store })
	o := openHarnessWith(t, hub, root, other, func(o *Options) {
		onBareFS(o)
		o.Favourites = &store
	})
	l.pick("work")
	l.do(Command{Name: CmdPin})
	o.pick("far")
	o.do(Command{Name: CmdPin})
	want := []Favourite{{Path: filepath.Join(local, "work")}, {Path: filepath.Join(other, "far"), FS: "elsewhere"}}
	o.until("the store holds both", func() bool { return slices.Equal(store.saved(), want) })
	l.until("the computer's window shows both", func() bool { return l.b.side.favs.Len() == 2 })
	if it := l.b.side.items[placeKey(Place{FS: "elsewhere", Path: filepath.Join(other, "far")}, true)]; !it.away {
		t.Fatalf("the other file system's favourite shows as %+v", it)
	}
	if !slices.Equal(l.a.favs, want) || !slices.Equal(o.a.favs, want) {
		t.Fatalf("the windows hold %+v and %+v", l.a.favs, o.a.favs)
	}
}
