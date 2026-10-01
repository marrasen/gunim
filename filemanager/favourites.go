package filemanager

import (
	"slices"
	"strings"
)

// Favourite is a folder the user pinned to the sidebar.
type Favourite struct {
	Path string
	// Name is the name the user gave it, and empty for the folder's own.
	Name string
}

// FavouriteStore keeps a window's favourites between runs. A window
// loads them as it opens and when its hub is told to refresh, and saves
// them whole each time the user pins, unpins, renames or reorders one.
// Its methods are called on the window's own goroutine, so they should
// be quick; windows of a hub given the same store, which has to be
// comparable, as a pointer is, share its favourites as they change.
type FavouriteStore interface {
	Load() ([]Favourite, error)
	Save(favs []Favourite) error
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

// Save implements [FavouriteStore]. The settings say why they could not
// be saved themselves.
func (s prefsFavourites) Save(favs []Favourite) error {
	s.a.prefs.Favourites, s.a.prefs.FavNames = splitFavourites(favs)
	s.a.savePrefs()
	return nil
}

// splitFavourites splits favs into their paths, and the names given to
// them by path, as the settings keep them.
func splitFavourites(favs []Favourite) (paths []string, names map[string]string) {
	paths = make([]string, len(favs))
	for i, f := range favs {
		paths[i] = f.Path
		if f.Name != "" {
			if names == nil {
				names = map[string]string{}
			}
			names[f.Path] = f.Name
		}
	}
	return paths, names
}

// memFavourites keeps the favourites of a window on another file system
// for as long as the window is open, as the store it has when its
// options give none: the settings file is the computer's own.
type memFavourites struct{}

// Load implements [FavouriteStore]: there is nothing kept to load.
func (memFavourites) Load() ([]Favourite, error) { return nil, nil }

// Save implements [FavouriteStore]: the window keeps them itself.
func (memFavourites) Save([]Favourite) error { return nil }

// favStore is the store of a's favourites.
func (a *app) favStore() FavouriteStore {
	switch {
	case a.opts.Favourites != nil:
		return a.opts.Favourites
	case a.fs.ID() == "":
		return prefsFavourites{a}
	}
	return memFavourites{}
}

// sharesFavourites reports whether a and o keep their favourites in the
// same store: the one their options give, or the settings file.
func (a *app) sharesFavourites(o *app) bool {
	if a.opts.Favourites == nil || o.opts.Favourites == nil {
		return a.opts.Favourites == nil && o.opts.Favourites == nil && a.fs.ID() == "" && o.fs.ID() == ""
	}
	return a.opts.Favourites == o.opts.Favourites
}

// loadFavourites reads the favourites from their store.
func (a *app) loadFavourites() {
	favs, err := a.favStore().Load()
	if err != nil {
		a.fail("Reading the favourites: " + err.Error())
		return
	}
	a.favs = favs
}

// setFavourites makes favs the favourites: it saves them, shows them,
// and shares them with the windows that keep theirs in the same store.
func (a *app) setFavourites(favs []Favourite) {
	a.favs = slices.Clone(favs)
	if err := a.favStore().Save(slices.Clone(favs)); err != nil {
		a.fail("Saving the favourites: " + err.Error())
	}
	a.publishPlaces()
	shared := slices.Clone(favs)
	a.hub.others(a, func(o *app) {
		if !a.sharesFavourites(o) {
			return
		}
		o.favs = slices.Clone(shared)
		if o.opts.Favourites == nil {
			o.prefs.Favourites, o.prefs.FavNames = splitFavourites(shared)
		}
		o.publishPlaces()
	})
}

// favourite returns the index of the favourite at path, or -1.
func (a *app) favourite(path string) int {
	return slices.IndexFunc(a.favs, func(f Favourite) bool { return a.ps.Same(f.Path, path) })
}

// addFavourites adds the folders at paths that are not favourites yet,
// and returns how many it added.
func (a *app) addFavourites(paths []string) int {
	favs := slices.Clone(a.favs)
	added := 0
	for _, p := range paths {
		if !slices.ContainsFunc(favs, func(f Favourite) bool { return a.ps.Same(f.Path, p) }) {
			favs = append(favs, Favourite{Path: p})
			added++
		}
	}
	if added > 0 {
		a.setFavourites(favs)
	}
	return added
}

// favName is what the sidebar calls the favourite at path: the name the
// user gave it, or the folder's.
func (a *app) favName(path string) string {
	if i := a.favourite(path); i >= 0 {
		if n := strings.TrimSpace(a.favs[i].Name); n != "" {
			return n
		}
	}
	return a.ps.placeName(path)
}

// favPaths returns the paths of the favourites.
func (a *app) favPaths() []string {
	out := make([]string, len(a.favs))
	for i, f := range a.favs {
		out[i] = f.Path
	}
	return out
}
