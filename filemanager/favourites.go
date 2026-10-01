package filemanager

import (
	"reflect"
	"slices"
	"strings"
	"sync"
)

// Favourite is a folder the user pinned to the sidebar.
type Favourite struct {
	Path string
	// Name is the name the user gave it, and empty for the folder's own.
	Name string
	// FS is the ID of the file system the folder is on. A store that is
	// not an AnyFSFavourites leaves it empty, and its favourites are on
	// the window's file system.
	FS string
}

// FavouriteStore keeps a window's favourites between runs. A window
// loads them as it opens and when its hub is told to refresh, and saves
// them whole each time the user pins, unpins, renames or reorders one.
// Its methods are called on the window's own goroutine, so they should
// be quick. Windows of a hub given the same store share its favourites
// as they change; a store that cannot be compared with ==, as a pointer
// can, is not shared, and a window's save may then undo another's. A
// store keeps the favourites of the window's file system, unless it is
// an [AnyFSFavourites].
type FavouriteStore interface {
	Load() ([]Favourite, error)
	Save(favs []Favourite) error
}

// AnyFSFavourites is a FavouriteStore whose favourites may be on any file
// system, each naming its own in FS, empty for the computer's own. A
// window lists them all; one on another file system than the window's
// is shown as elsewhere, and a click on it asks for a Visit.
type AnyFSFavourites interface {
	FavouriteStore
	// Where names the file system of ID fs, as a favourite on another
	// file system than the window's says where it is.
	Where(fs string) string
}

// prefsFavourites keeps the favourites of a window on the computer's own
// file system in its settings file, as the store it has when its options
// give none.
type prefsFavourites struct{ a *app }

// Load implements [FavouriteStore].
func (s prefsFavourites) Load() ([]Favourite, error) {
	p := &s.a.prefs
	out := make([]Favourite, len(p.Favourites))
	for i, path := range p.Favourites {
		out[i] = Favourite{Path: path, Name: p.FavNames[path]}
	}
	return out, nil
}

// Save implements [FavouriteStore]. The name given to a favourite stays
// in the settings when it is unpinned, so it comes back with it. The
// settings say why they could not be saved themselves.
func (s prefsFavourites) Save(favs []Favourite) error {
	s.a.savePrefs(func(p *prefs) { p.Favourites, p.FavNames = mergeFavourites(p.FavNames, favs) })
	return nil
}

// name returns the name the favourite at path was given before it was
// unpinned, if any.
func (s prefsFavourites) name(path string) string { return s.a.prefs.FavNames[path] }

// mergeFavourites splits favs into their paths, and the names given to
// them by path, as the settings keep them, keeping the names in was that
// favs do not set.
func mergeFavourites(was map[string]string, favs []Favourite) (paths []string, names map[string]string) {
	names = cloneNames(was)
	paths = make([]string, len(favs))
	for i, f := range favs {
		paths[i] = f.Path
		switch {
		case f.Name != "":
			if names == nil {
				names = map[string]string{}
			}
			names[f.Path] = f.Name
		case names != nil:
			delete(names, f.Path)
		}
	}
	return paths, names
}

// memFavourites keeps the favourites of the windows of a hub on a file
// system other than the computer's own, for as long as the hub runs, as
// the store they have when their options give none: the settings file
// is the computer's own.
type memFavourites struct {
	mu   sync.Mutex
	favs []Favourite
}

// Load implements [FavouriteStore].
func (m *memFavourites) Load() ([]Favourite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.favs), nil
}

// Save implements [FavouriteStore].
func (m *memFavourites) Save(favs []Favourite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.favs = slices.Clone(favs)
	return nil
}

// favStore is the store of a's favourites.
func (a *app) favStore() FavouriteStore {
	switch {
	case a.opts.Favourites != nil:
		return a.opts.Favourites
	case a.fs.ID() == "":
		return prefsFavourites{a}
	}
	return a.hub.memFavourites(a.fs.ID())
}

// sharesFavourites reports whether a and o keep their favourites in the
// same store: the one their options give, the settings file, or the
// hub's store of their file system. A store that cannot be compared is
// shared with no other window.
func (a *app) sharesFavourites(o *app) bool {
	x, y := a.opts.Favourites, o.opts.Favourites
	switch {
	case x == nil && y == nil:
		return a.fs.ID() == o.fs.ID()
	case x == nil || y == nil:
		return false
	case reflect.TypeOf(x) != reflect.TypeOf(y) || !reflect.TypeOf(x).Comparable():
		return false
	}
	return x == y
}

// loadFavourites reads the favourites from their store.
func (a *app) loadFavourites() {
	favs, err := a.favStore().Load()
	if err != nil {
		a.fail("Reading the favourites: " + err.Error())
		return
	}
	a.favs = a.fromStore(favs)
}

// fromStore returns favs as loaded from a's store, each with its file
// system: those of a store that is not an AnyFSFavourites are on the
// window's.
func (a *app) fromStore(favs []Favourite) []Favourite {
	favs = slices.Clone(favs)
	if _, anyFS := a.favStore().(AnyFSFavourites); !anyFS {
		for i := range favs {
			favs[i].FS = a.fs.ID()
		}
	}
	return favs
}

// toStore returns favs as a's store keeps them: all of them for an
// AnyFSFavourites, and otherwise those on the window's file system, with
// FS empty, as such a store keeps them.
func (a *app) toStore(favs []Favourite) []Favourite {
	if _, anyFS := a.favStore().(AnyFSFavourites); anyFS {
		return slices.Clone(favs)
	}
	out := make([]Favourite, 0, len(favs))
	for _, f := range favs {
		if f.FS == a.fs.ID() {
			f.FS = ""
			out = append(out, f)
		}
	}
	return out
}

// setFavourites makes favs the favourites: it saves them, shows them,
// and has the windows that keep theirs in the same store show them too,
// each as it would load them.
func (a *app) setFavourites(favs []Favourite) {
	a.favs = slices.Clone(favs)
	saved := a.toStore(favs)
	if err := a.favStore().Save(slices.Clone(saved)); err != nil {
		a.fail("Saving the favourites: " + err.Error())
	}
	a.publishPlaces()
	a.hub.others(a, func(o *app) {
		if !a.sharesFavourites(o) {
			return
		}
		o.favs = o.fromStore(saved)
		if o.opts.Favourites == nil && o.fs.ID() == "" {
			o.prefs.Favourites, o.prefs.FavNames = mergeFavourites(o.prefs.FavNames, saved)
		}
		o.publishPlaces()
	})
}

// isFav reports whether f is the favourite at path on the file system
// of ID fs: paths on the window's file system compare as it writes them,
// and others as they are.
func (a *app) isFav(f Favourite, fs, path string) bool {
	switch {
	case f.FS != fs:
		return false
	case fs == a.fs.ID():
		return a.ps.Same(f.Path, path)
	}
	return f.Path == path
}

// favourite returns the index of the favourite at path on the file
// system of ID fs, or -1.
func (a *app) favourite(fs, path string) int {
	return slices.IndexFunc(a.favs, func(f Favourite) bool { return a.isFav(f, fs, path) })
}

// addFavourites adds the folders at paths on the window's file system
// that are not favourites yet, and returns how many it added.
func (a *app) addFavourites(paths []string) int {
	favs := slices.Clone(a.favs)
	added := 0
	for _, p := range paths {
		if !slices.ContainsFunc(favs, func(f Favourite) bool { return a.isFav(f, a.fs.ID(), p) }) {
			f := Favourite{Path: p, FS: a.fs.ID()}
			if s, ok := a.favStore().(prefsFavourites); ok {
				f.Name = s.name(p)
			}
			favs = append(favs, f)
			added++
		}
	}
	if added > 0 {
		a.setFavourites(favs)
	}
	return added
}

// favName is what the sidebar calls the favourite at path on the file
// system of ID fs: the name the user gave it, or the folder's.
func (a *app) favName(fs, path string) string {
	if i := a.favourite(fs, path); i >= 0 {
		if n := strings.TrimSpace(a.favs[i].Name); n != "" {
			return n
		}
	}
	return a.folderName(fs, path)
}

// folderName is the name of the folder at path on the file system of ID
// fs. How another file system writes paths is not known, so its paths
// are taken to end in a name after the last slash or backslash.
func (a *app) folderName(fs, path string) string {
	if fs == a.fs.ID() {
		return a.ps.placeName(path)
	}
	if name := path[strings.LastIndexAny(path, `/\`)+1:]; name != "" {
		return name
	}
	return path
}

// favPlaces returns the favourites as places, as the sidebar and the
// palette list them: one on another file system than the window's says
// where it is, as its store names it.
func (a *app) favPlaces() []Place {
	where, _ := a.favStore().(AnyFSFavourites)
	out := make([]Place, len(a.favs))
	for i, f := range a.favs {
		out[i] = Place{Name: a.favName(f.FS, f.Path), Path: f.Path, Kind: "favourite", FS: f.FS}
		if f.FS != a.fs.ID() && where != nil {
			out[i].Note = where.Where(f.FS)
		}
	}
	return out
}
