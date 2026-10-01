package filemanager

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/widget"
)

// memStore is a favourites store that counts what it is given.
type memStore struct {
	mu    sync.Mutex
	favs  []Favourite
	saves int
}

func (s *memStore) Load() ([]Favourite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.favs), nil
}

func (s *memStore) Save(favs []Favourite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.favs, s.saves = slices.Clone(favs), s.saves+1
	return nil
}

func (s *memStore) saved() []Favourite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.favs)
}

func TestTheCallerGivesThePlacesWithTheirGroups(t *testing.T) {
	var mu sync.Mutex
	var visits []string
	h := newHarnessWith(t, func(o *Options) {
		dir := o.Dir
		o.Places = func() ([]Place, error) {
			return []Place{
				{Name: "Here", Path: dir, Kind: "home", Group: "This computer"},
				{Name: "Server home", Path: "/home/me", Kind: "home", Group: "server", Note: "Connected", FS: "sftp://server"},
			}, nil
		}
		o.Visit = func(fs, path string) {
			mu.Lock()
			defer mu.Unlock()
			visits = append(visits, fs+" "+path)
		}
	}, "a.txt")
	h.until("the places arrive", func() bool { return h.b.side.places.Len() == 3 })
	if got := h.b.side.first.Text; got != "THIS COMPUTER" {
		t.Fatalf("the first heading says %q", got)
	}
	keys := h.b.side.places.Keys()
	if heading := h.b.side.items[keys[1]]; !heading.heading || heading.Name != "SERVER" {
		t.Fatalf("the second row is %+v, want the heading of the server", heading)
	}
	if away := h.b.side.items[keys[2]]; !away.away || away.Note != "Connected" {
		t.Fatalf("the server's place is %+v", away)
	}
	h.frames(30)
	row := h.bounds(func(b *browser) gunim.Node {
		n, _ := b.side.places.Row(keys[2])
		return n
	})
	h.click(row.Center())
	h.until("the click asks to visit the server", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Equal(visits, []string{"sftp://server /home/me"})
	})
	if !SystemPaths.Same(h.a.nav.path, h.dir) {
		t.Fatalf("the window went to %s itself", h.a.nav.path)
	}
}

func TestTheCallersStoreKeepsTheFavourites(t *testing.T) {
	store := &memStore{}
	h := newHarnessWith(t, func(o *Options) {
		store.favs = []Favourite{{Path: filepath.Join(o.Dir, "old"), Name: "Kept"}}
		o.Favourites = store
	}, "old/", "work/")
	h.until("the stored favourite shows", func() bool { return h.b.side.favs.Len() == 1 })
	h.pick("work")
	h.do(Command{Name: CmdPin})
	want := []Favourite{{Path: filepath.Join(h.dir, "old"), Name: "Kept"}, {Path: filepath.Join(h.dir, "work")}}
	h.until("the store holds both", func() bool { return slices.Equal(store.saved(), want) })
	h.until("the sidebar shows both", func() bool { return h.b.side.favs.Len() == 2 })
	n, _ := h.b.side.favs.Row(widget.Key(filepath.Join(h.dir, "old")))
	if r, ok := n.(*placeRow); !ok || r.item.Name != "Kept" {
		t.Fatal("the favourite does not show the name it was given")
	}
	if len(h.a.prefs.Favourites) != 0 {
		t.Fatal("the favourites went to the settings file too")
	}
}

func TestRefreshFindsThePlacesAgain(t *testing.T) {
	var mu sync.Mutex
	note := "Connecting"
	places := func() ([]Place, error) {
		mu.Lock()
		defer mu.Unlock()
		return []Place{{Name: "Server", Path: "/", Kind: "drive", FS: "sftp://server", Note: note}}, nil
	}
	w := newHarnessWith(t, func(o *Options) { o.Places = places }, "a.txt")
	w.until("the place shows", func() bool { return w.b.side.places.Len() == 1 })
	mu.Lock()
	note = "Connected"
	mu.Unlock()
	w.a.hub.Refresh()
	w.until("the place says it connected", func() bool {
		for _, it := range w.b.side.items {
			if it.Note == "Connected" {
				return true
			}
		}
		return false
	})
}
