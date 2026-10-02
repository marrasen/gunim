package filemanager

// trashless takes the trash away, for a file system that turned out to
// have none, and tells the window, whose menus then say Delete.
func (a *app) trashless() {
	a.trash = nil
	a.shell.NoTrash = true
	a.publishShell()
}

// showFS turns the window to the folder dir on fsys, or its home folder
// when dir is empty. What the window knew of the file system it showed
// goes: the history, the index of the palette, the thumbnails, the
// volumes, the clipboard, the places and the favourites. The places
// shown stay until those of fsys are found, so the sidebar does not
// empty and fill again, and its rows that stay keep still.
func (a *app) showFS(fsys FS, dir string) {
	if fsys == nil {
		fsys = LocalFS()
	}
	a.closeViewer()
	a.search.stop()
	// The counts go on, so the answers of the old index are dropped.
	a.search = searchState{gen: a.search.gen + 1, seq: a.search.seq}
	a.opts.FS, a.fs, a.ps = fsys, fsys, fsys.Paths()
	a.trash, _ = fsys.(Trasher)
	a.thumbs.cache, a.thumbs.order, a.thumbs.bytes, a.thumbs.queue = map[thumbKey]thumbDone{}, nil, 0, nil
	a.dnd.vols, a.dnd.volErrs = nil, nil
	a.nav.back, a.nav.fwd, a.nav.path = nil, nil, ""
	a.nav.space, a.nav.spaceErr = space{}, nil

	a.syncClip()

	a.shell.FS, a.shell.Paths, a.shell.NoTrash = fsys.ID(), a.ps, a.trash == nil
	a.shell.Where = a.where()
	a.publishShell()
	a.loadFavourites()
	a.loadPlaces()
	a.startNav(dir)
}
