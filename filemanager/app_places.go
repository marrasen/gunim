package filemanager

import (
	"path/filepath"
	"slices"

	"github.com/marrasen/gunim"
)

// handlePlaces takes the intents of the sidebar.
func (a *app) handlePlaces(in gunim.Intent) bool {
	switch v := in.(type) {
	case FavouritesReordered:
		a.prefs.Favourites = slices.Clone(v.Paths)
		a.savePrefs()
		a.publishPlaces()
	case Unpin:
		a.prefs.Favourites = slices.DeleteFunc(a.prefs.Favourites, func(p string) bool { return samePath(p, v.Path) })
		a.savePrefs()
		a.publishPlaces()
	case Command:
		if v.Name != CmdPin {
			return false
		}
		a.pin()
	default:
		return false
	}
	return true
}

// pin adds the folders selected, or the folder showing, to the
// favourites.
func (a *app) pin() {
	var paths []string
	for _, e := range a.selectedEntries() {
		if e.Dir {
			paths = append(paths, filepath.Join(a.nav.path, e.Name))
		}
	}
	if len(paths) == 0 {
		paths = []string{a.nav.path}
	}
	for _, p := range paths {
		if !slices.ContainsFunc(a.prefs.Favourites, func(f string) bool { return samePath(f, p) }) {
			a.prefs.Favourites = append(a.prefs.Favourites, p)
		}
	}
	a.savePrefs()
	a.publishPlaces()
}

// loadPlaces finds the places in the background, as a drive can be slow
// to answer.
func (a *app) loadPlaces() {
	go func() {
		ps, err := gatherPlaces()
		a.post(func() {
			if err != nil {
				a.fail("Finding the places for the sidebar: " + err.Error())
			}
			a.places = ps
			a.publishPlaces()
		})
	}()
}

func (a *app) publishPlaces() {
	s := Places{Places: a.places, Current: a.nav.path}
	for _, f := range a.prefs.Favourites {
		s.Favourites = append(s.Favourites, Place{Name: a.favName(f), Path: f, Kind: "favourite"})
	}
	a.patch(s)
	a.publishVolumes(s)
}
