package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
)

// dndState is what the app knows for drops: the volume of each folder a
// window can drop on, and why a volume could not be read.
type dndState struct {
	vols    map[string]string
	volErrs map[string]string
	// finding is set while volumes are being read, and again when more
	// were asked for meanwhile.
	finding, again bool
}

// handleDnd takes the intents of drag and drop, the context menus and
// the windows.
func (a *app) handleDnd(in gunim.Intent) bool {
	switch v := in.(type) {
	case DropFiles:
		a.dropFiles(v)
	case PinFolders:
		a.pinFolders(v.Paths)
	case OpenWindow:
		a.openWindow(v.Path)
	case RenameFavourite:
		a.renameFavourite(v.Path)
	case PropsApplied:
		a.answered(v)
	case Command:
		switch v.Name {
		case CmdOpenSystem:
			a.openSystem()
		case CmdDuplicate:
			if paths := a.selectedPaths(); len(paths) > 0 {
				a.startOp(job{kind: OpCopy, srcs: paths, dest: a.nav.path}, "Duplicating "+what(paths))
			}
		case CmdProperties:
			a.properties()
		case CmdNewWindow:
			a.openWindow(a.nav.path)
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// dropFiles moves or copies what was dropped into the folder it was
// dropped on, as an operation with progress and undo.
func (a *app) dropFiles(v DropFiles) {
	if len(v.Paths) == 0 || v.Into == "" {
		return
	}
	for _, p := range v.Paths {
		if samePath(filepath.Dir(p), v.Into) {
			a.fail(filepath.Base(p) + " is already in " + placeName(v.Into) + ".")
			return
		}
		if err := into(p, v.Into); err != nil {
			a.fail(err.Error() + ".")
			return
		}
	}
	srcs := slices.Clone(v.Paths)
	if v.Copy {
		a.startOp(job{kind: OpCopy, srcs: srcs, dest: v.Into}, "Copying "+what(srcs)+" to "+placeName(v.Into))
		return
	}
	a.startOp(job{kind: OpMove, srcs: srcs, dest: v.Into}, "Moving "+what(srcs)+" to "+placeName(v.Into))
}

// pinFolders adds the folders among paths to the favourites, once each
// is read, and says which items were left out as not folders.
func (a *app) pinFolders(paths []string) {
	paths = slices.Clone(paths)
	go func() {
		var dirs, files []string
		for _, p := range paths {
			info, err := os.Stat(p)
			if err != nil {
				a.post(func() { a.fail("Pinning to favourites: " + err.Error()) })
				return
			}
			if info.IsDir() {
				dirs = append(dirs, p)
			} else {
				files = append(files, filepath.Base(p))
			}
		}
		a.post(func() {
			added := 0
			for _, p := range dirs {
				if !slices.ContainsFunc(a.prefs.Favourites, func(f string) bool { return samePath(f, p) }) {
					a.prefs.Favourites = append(a.prefs.Favourites, p)
					added++
				}
			}
			if added > 0 {
				a.savePrefs()
				a.publishPlaces()
			}
			n := Notice{Title: "Pinned " + plural(added, "folder"), Kind: "success"}
			if len(files) > 0 {
				n.Body = strings.Join(files, ", ") + " left out: only folders can be pinned."
			}
			a.patch(n)
		})
	}()
}

// renameFavourite asks for a name for the favourite at path.
func (a *app) renameFavourite(path string) {
	name := a.favName(path)
	a.prompt(Prompt{Title: "Rename favourite", Text: name, OK: "Rename", Stem: utf8.RuneCountInString(name)}, func(n string) {
		if a.prefs.FavNames == nil {
			a.prefs.FavNames = map[string]string{}
		}
		n = strings.TrimSpace(n)
		if n == "" || n == placeName(path) {
			delete(a.prefs.FavNames, path)
		} else {
			a.prefs.FavNames[path] = n
		}
		a.savePrefs()
		a.publishPlaces()
	})
}

// openSystem opens the items selected with the system's programs, or
// the folder showing when none is selected.
func (a *app) openSystem() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		paths = []string{a.nav.path}
	}
	go func() {
		for _, p := range paths {
			if err := a.c.Open(p); err != nil {
				a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", p, err)) })
				return
			}
		}
	}()
}

// publishVolumes finds the volume of each folder in s the window can
// drop on, in the background, and sends them: the folder showing and
// the folders along its path, the links to folders in it, the places
// and the favourites.
func (a *app) publishVolumes(s Places) {
	d := &a.dnd
	if d.vols == nil {
		d.vols, d.volErrs = map[string]string{}, map[string]string{}
	}
	var missing []string
	add := func(p string) {
		if _, ok := d.vols[p]; !ok && p != "" && !slices.Contains(missing, p) {
			missing = append(missing, p)
		}
	}
	add(s.Current)
	if s.Current != "" {
		for _, c := range crumbs(s.Current) {
			add(c.Path)
		}
	}
	if samePath(s.Current, a.nav.path) {
		for _, e := range a.nav.all {
			if e.Kind == KindLink && e.Dir {
				add(filepath.Join(s.Current, e.Name))
			}
		}
	}
	for _, p := range slices.Concat(s.Places, s.Favourites) {
		add(p.Path)
	}
	if len(missing) > 0 && d.finding {
		d.again = true
	}
	if len(missing) == 0 || d.finding {
		a.patch(Volumes{Of: cloneNames(d.vols), Errs: cloneNames(d.volErrs)})
		return
	}
	d.finding = true
	go func() {
		vols, errs := map[string]string{}, map[string]string{}
		for _, p := range missing {
			v, err := volumeOf(p)
			if err != nil {
				errs[p] = err.Error()
				continue
			}
			vols[p] = v
		}
		a.post(func() {
			d.finding = false
			for p, v := range vols {
				d.vols[p] = v
				delete(d.volErrs, p)
			}
			for p, e := range errs {
				d.volErrs[p] = e
			}
			a.patch(Volumes{Of: cloneNames(d.vols), Errs: cloneNames(d.volErrs)})
			if d.again {
				d.again = false
				a.publishPlaces()
			}
		})
	}()
}

// scriptDnd runs a script's step of drag and drop, and reports whether
// it knew the step: drag-start, drag-over and menu name an item or
// place, with +ctrl or +shift after the name to hold the key, drop lets
// go, and add adds an item to the selection.
func (a *app) scriptDnd(verb, arg string) bool {
	name, key, _ := strings.Cut(arg, "+")
	mods := map[string]input.Mods{"ctrl": input.ModControl, "shift": input.ModShift}[key]
	step := map[string]string{"drag-start": "start", "drag-over": "over", "drop": "drop", "menu": "menu"}[verb]
	switch {
	case step != "":
		a.patch(ScriptDrag{Step: step, Name: name, Mods: int(mods)})
	case verb == "add":
		a.nav.sel[arg] = true
		a.publishSelection()
		a.publishStatus()
		a.showPreview()
	default:
		return false
	}
	return true
}
