package filemanager

import (
	"errors"
	"slices"

	"github.com/marrasen/gunim"
)

// handlePlaces takes the intents of the sidebar.
func (a *app) handlePlaces(in gunim.Intent) bool {
	switch v := in.(type) {
	case FavouritesReordered:
		favs := make([]Favourite, 0, len(v.Paths))
		for _, p := range v.Paths {
			f := Favourite{Path: p}
			if i := a.favourite(p); i >= 0 {
				f = a.favs[i]
			}
			favs = append(favs, f)
		}
		a.setFavourites(favs)
	case Unpin:
		a.setFavourites(slices.DeleteFunc(slices.Clone(a.favs), func(f Favourite) bool { return a.ps.Same(f.Path, v.Path) }))
	case Visit:
		a.visit(v)
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
			paths = append(paths, a.ps.Join(a.nav.path, e.Name))
		}
	}
	if len(paths) == 0 {
		paths = []string{a.nav.path}
	}
	a.addFavourites(paths)
}

// visit goes to a place on another file system, as the options say, on a
// goroutine of its own, as it may open a window.
func (a *app) visit(v Visit) {
	if a.opts.Visit == nil {
		a.fail(v.Path + " is on another file system, which this window cannot show.")
		return
	}
	go a.opts.Visit(a.win, v.FS, v.Path)
}

// loadPlaces finds the places in the background, as a drive can be slow
// to answer.
func (a *app) loadPlaces() {
	find := a.opts.Places
	if find == nil {
		find = a.defaultPlaces
	}
	a.placesGen++
	gen := a.placesGen
	go func() {
		ps, err := find()
		a.post(func() {
			if gen != a.placesGen {
				// A later lookup has begun, and has the last word.
				return
			}
			if err != nil {
				a.fail("Finding the places for the sidebar: " + err.Error())
			}
			a.places = ps
			a.publishPlaces()
		})
	}()
}

// defaultPlaces are the places of a window whose options give none: the
// user's folders and the drives of the computer, for its own file
// system, and the home folder and the top for another.
func (a *app) defaultPlaces() ([]Place, error) {
	if a.fs.ID() == "" {
		return LocalPlaces()
	}
	home, err := a.fs.Home()
	if err != nil {
		return nil, err
	}
	top := a.ps.VolumeName(home) + a.ps.Sep()
	out := []Place{{Name: "Home", Path: home, Kind: "home", FS: a.fs.ID()}, {Name: top, Path: top, Kind: "drive", FS: a.fs.ID()}}
	if sr, ok := a.fs.(SpaceReporter); ok {
		if free, total, err := sr.Space(top); errors.Is(err, errors.ErrUnsupported) {
			// The file system cannot say after all.
		} else if err != nil {
			out[1].Err = rootCause(err).Error()
		} else {
			out[1].Free, out[1].Total = free, total
		}
	}
	return out, nil
}

func (a *app) publishPlaces() {
	s := Places{Places: a.places, Current: a.nav.path}
	for _, f := range a.favs {
		s.Favourites = append(s.Favourites, Place{Name: a.favName(f.Path), Path: f.Path, Kind: "favourite", FS: a.fs.ID()})
	}
	a.patch(s)
	a.publishVolumes(s)
}
