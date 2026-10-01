package filemanager

import "github.com/marrasen/gunim"

// Place is a folder the sidebar offers: one of the user's, a volume or a
// favourite.
type Place struct {
	Name, Path string
	// Kind is home, desktop, documents, downloads, pictures, drive or
	// favourite, and picks the colour of the place's mark.
	Kind string
	// Free and Total are a volume's space in bytes, when it has one.
	Free, Total uint64
	// Err says why the place cannot be read.
	Err string
	// Note is a line under the name, such as whether a server is
	// connected. Err and the space show in its place.
	Note string
	// Group is the heading the place shows under, such as the name of
	// a computer, and empty for the first, which is Places. A group
	// starts where the group of the place before it ends.
	Group string
	// FS is the ID of the file system the place is on, empty for the
	// computer's own. A click on a place on another file system than
	// the window's asks for a Visit.
	FS string
}

// Places is the sidebar: the places the system has, the user's
// favourites, and the folder showing.
type Places struct {
	Places     []Place
	Favourites []Place
	Current    string
}

// FavouritesReordered carries the favourites in the order a drag left
// them in.
type FavouritesReordered struct {
	Favourites []FavouriteAt
}

// FavouriteAt names a favourite: the folder at Path on the file system
// of ID FS.
type FavouriteAt struct {
	FS, Path string
}

// Unpin takes the favourite at Path on the file system of ID FS off the
// sidebar.
type Unpin struct {
	FS, Path string
}

// Visit asks to go to the folder at Path on the file system of ID FS,
// which is not the one the window shows.
type Visit struct {
	FS, Path string
}

func init() {
	gunim.RegisterType[Places]("files.places")
	gunim.RegisterType[FavouritesReordered]("files.favourites-reordered")
	gunim.RegisterType[Unpin]("files.unpin")
	gunim.RegisterType[Visit]("files.visit")
}
