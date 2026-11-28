package filemanager

import (
	"fmt"
	"slices"
)

// The top: a folder the window keeps inside, as Options.Top says, on the
// file system it opened on. Nothing above it shows: the path bar starts
// at it, Up stops at it, and the places and favourites outside it are
// left out. A path typed, or a folder asked for in any other way, that is
// not inside it is refused.

// topState is the top, where the window has one.
type topState struct {
	// path is the top, whole, and empty for none; fs is the ID of the
	// file system it is on, and name what the window calls it.
	path, fs, name string
}

// setTop takes the top o asks for.
func (a *app) setTop(o Options) error {
	if o.Top == "" {
		return nil
	}
	abs, err := a.ps.Abs(o.Top)
	if err != nil {
		return fmt.Errorf("files: the top %s: %w", o.Top, err)
	}
	a.top = topState{path: abs, fs: o.FS.ID(), name: o.TopName}
	if a.top.name == "" {
		a.top.name = a.ps.placeName(abs)
	}
	return nil
}

// topHere reports whether the window keeps inside a top on the file
// system it shows.
func (a *app) topHere() bool { return a.top.path != "" && a.fs.ID() == a.top.fs }

// aboveTop reports whether the folder at p is outside the top.
func (a *app) aboveTop(p string) bool {
	return a.topHere() && !a.ps.inside(p, a.top.path)
}

// atTop reports whether p is the top itself.
func (a *app) atTop(p string) bool { return a.topHere() && a.ps.Same(p, a.top.path) }

// placeName is what the window calls the folder at p: the top by its
// name, and any other as the path style names it.
func (a *app) placeName(p string) string {
	if a.atTop(p) {
		return a.top.name
	}
	return a.ps.placeName(p)
}

// refuseAbove says the folder at p cannot be gone to, being above the top.
func (a *app) refuseAbove(p string) {
	a.fail(a.ps.Show(p) + " is outside " + a.top.name + ".")
}

// topCrumbs are the folders of the path bar from the top down to the
// folder at p, the top by its name.
func (a *app) topCrumbs(p string) []Crumb {
	all := crumbs(a.ps, p)
	if !a.topHere() {
		return all
	}
	i := slices.IndexFunc(all, func(c Crumb) bool { return a.ps.Same(c.Path, a.top.path) })
	if i < 0 {
		return all
	}
	out := slices.Clone(all[i:])
	out[0].Name = a.top.name
	return out
}

// withinTop are the places of ps the window may go to: those on another
// file system, and on its own those inside the top.
func (a *app) withinTop(ps []Place) []Place {
	if !a.topHere() {
		return ps
	}
	return slices.DeleteFunc(slices.Clone(ps), func(p Place) bool {
		return p.FS == a.fs.ID() && p.Path != TrashPath && a.aboveTop(p.Path)
	})
}

// topPlace is the one place of a window with a top: the top.
func (a *app) topPlace() Place {
	return Place{Name: a.top.name, Path: a.top.path, Kind: "home", FS: a.top.fs}
}
