package filemanager

import (
	"context"
	"errors"
	"slices"

	"github.com/marrasen/gunim/zipcrypt"
)

// zipKeys returns the passwords of the archives the windows of the hub
// opened.
func (h *Hub) zipKeys() *zipKeys {
	if h == nil {
		return nil
	}
	return &h.keys
}

// archived returns fsys with its archives opened as folders, as the
// window shows it.
func (a *app) archived(fsys FS) FS { return withArchives(fsys, a.hub.zipKeys()) }

// archiveOf returns the archive the item at p is in, on the window's
// file system, or with self is, and "" for none. It may ask the file
// system, so it runs on a goroutine of its own, but for p the folder the
// window shows, which the window knows.
func (a *app) archiveOf(p string, self bool) string {
	ar, ok := a.fs.(*archives)
	if !ok {
		return ""
	}
	at, _ := ar.inArchive(p, self)
	return at
}

// anyInArchive reports whether any of paths, of the window's file
// system, is in an archive.
func (a *app) anyInArchive(paths []string) bool {
	if a.nav.archive != "" && slices.ContainsFunc(paths, func(p string) bool { return a.ps.inside(p, a.nav.archive) }) {
		return true
	}
	return slices.ContainsFunc(paths, func(p string) bool { return a.archiveOf(p, false) != "" })
}

// opensAsFolder reports whether e, an item of the folder showing, is an
// archive the window walks into.
func (a *app) opensAsFolder(e entry) bool {
	if _, ok := a.fs.(*archives); !ok {
		return false
	}
	return !e.Dir && !e.Broken && e.Err == "" && browsable(e.base())
}

// findArchive finds, in the background, which archive the folder at
// path is in, once the window has turned to it, and tells the window.
func (a *app) findArchive(path string) {
	ar, ok := a.fs.(*archives)
	if !ok || !namesArchive(a.ps, path) {
		a.inFolderOf("")
		return
	}
	moves := a.nav.moves
	go func() {
		at, _ := ar.inArchive(path, true)
		locked := at != "" && ar.locked(at)
		a.post(func() {
			if a.nav.moves != moves {
				return
			}
			a.inFolderOf(at)
			if locked {
				a.askUnlock(ar, at)
			}
		})
	}()
}

// namesArchive reports whether a name along p is one an archive has.
func namesArchive(ps PathStyle, p string) bool {
	for {
		if browsable(ps.Base(p)) {
			return true
		}
		up := ps.Dir(p)
		if up == p || up == "." || up == "" {
			return false
		}
		p = up
	}
}

// inFolderOf takes the archive the folder showing is in, "" for none:
// its files open with the computer's programs only fetched, and nothing
// in it changes.
func (a *app) inFolderOf(archive string) {
	if a.nav.archive == archive {
		return
	}
	a.nav.archive = archive
	a.shell.Fetches, a.shell.OpenWith = a.fetches(), a.openWithOn()
	a.publishShell()
	a.publishListing()
}

// askUnlock asks for the password of the archive at p, which ar opens,
// until it is right or the user gives none.
func (a *app) askUnlock(ar *archives, p string) {
	moves := a.nav.moves
	ask := PasswordAsk{FS: a.fs.ID(), Path: p, Where: a.shown(p)}
	ctx, cancel := a.untilStopped()
	go func() {
		defer cancel()
		for {
			pw, err := a.askPassword(ctx, 0, ask)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.post(func() {
						if a.nav.moves == moves {
							a.patch(Notice{Title: a.ps.Base(p) + " stays locked",
								Body: "Its files show, but open only with its password. Go into it again to type it.", Kind: "info"})
						}
					})
				}
				return
			}
			err = ar.unlock(p, pw.Text)
			if errors.Is(err, zipcrypt.ErrPassword) {
				ask.Wrong = true
				continue
			}
			if err != nil {
				a.post(func() { a.fail("Opening " + a.shown(p) + ": " + err.Error()) })
				return
			}
			if pw.Worked != nil {
				pw.Worked()
			}
			// What showed of it before, as a preview, may show now.
			a.post(func() {
				if a.nav.moves == moves {
					a.showPreview()
				}
			})
			return
		}
	}()
}

// readOnlyHere refuses the command name where it would change what the
// archive showing holds, and says why. It reports whether it did.
func (a *app) readOnlyHere(name string) bool {
	if !a.mayChange() && (changes(name) || name == CmdUndo) {
		a.refuseReadOnly()
		return true
	}
	if a.nav.archive == "" || !changes(name) {
		return false
	}
	a.refuseArchive(a.nav.archive)
	return true
}

// refuseReadOnly says that the files of a window that is read only do
// not change.
func (a *app) refuseReadOnly() {
	a.patch(Notice{Title: readOnlyHint, Body: "Nothing here can be changed.", Kind: "info"})
}

// mayChange reports whether the window may change its files: it is not
// read only. Every change asks it, whatever asks for the change.
func (a *app) mayChange() bool { return !a.shell.ReadOnly }

// refused reports whether a change is refused, as the window is read
// only, and says so where it is.
func (a *app) refused() bool {
	if a.mayChange() {
		return false
	}
	a.refuseReadOnly()
	return true
}

// setReadOnly shows the files to look at only, or to change too. As it
// turns read only, the questions open that lead to a change are called
// off, and what offers to undo is offered no more.
func (a *app) setReadOnly(on bool) {
	if a.shell.ReadOnly == on {
		return
	}
	a.shell.ReadOnly = on
	a.opts.ReadOnly = on
	a.publishShell()
	if on {
		a.ops.undo = nil
		a.dropAsked()
	}
	a.publishOps()
	a.publishUsage()
	a.publishTrashCan()
}

// dropAsked calls off the questions shown that no operation waits on,
// as a name for a new folder, or whether to delete: each is answered as
// closed.
func (a *app) dropAsked() {
	var kept, gone []*dialog
	for _, d := range a.ops.dialogs {
		if d.op == 0 {
			gone = append(gone, d)
		} else {
			kept = append(kept, d)
		}
	}
	if len(gone) == 0 {
		return
	}
	shown := a.ops.dialogs[0]
	a.ops.dialogs = kept
	if shown.op == 0 {
		a.send(a.c.Unmount(shown.id))
		if len(kept) > 0 {
			a.mountDialog(kept[0])
		} else {
			a.patch(FocusListing{})
		}
	}
	for _, d := range gone {
		d.answer(DialogClosed{})
	}
}

// refuseArchive says that what the archive at p holds does not change.
func (a *app) refuseArchive(p string) {
	a.patch(Notice{Title: a.ps.Base(p) + " can only be read here",
		Body: "Copy what you need out of it, and change it there.", Kind: "info"})
}

// archiveSource returns the file system to read the items at paths from,
// of the file system of ID id, where they are in an archive, and nil
// where they are not, or no window shows that file system.
func (a *app) archiveSource(id string, paths []string) FS {
	fsys := a.fs
	if id != a.fs.ID() {
		if fsys = a.hub.fsOf(id); fsys == nil {
			return nil
		}
	}
	ar, ok := fsys.(*archives)
	if !ok {
		return nil
	}
	for _, p := range paths {
		if _, in := ar.inArchive(p, false); in {
			return ar
		}
	}
	return nil
}
