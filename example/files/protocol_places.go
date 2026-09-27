package main

import "github.com/marrasen/gunim"

// Place is a folder the sidebar offers: one of the user's, a volume or a
// favourite.
type Place struct {
	Name, Path string
	// Kind is home, desktop, documents, downloads, pictures, drive or
	// favourite.
	Kind string
	// Free and Total are a volume's space in bytes, when it has one.
	Free, Total uint64
	// Err says why the place cannot be read.
	Err string
}

// Places is the sidebar: the places the system has, the user's
// favourites, and the folder showing.
type Places struct {
	Places     []Place
	Favourites []Place
	Current    string
}

// FavouritesReordered carries the favourites' paths in the order a drag
// left them in.
type FavouritesReordered struct {
	Paths []string
}

// Unpin takes a favourite off the sidebar.
type Unpin struct {
	Path string
}

func init() {
	gunim.RegisterType[Places]("files.places")
	gunim.RegisterType[FavouritesReordered]("files.favourites-reordered")
	gunim.RegisterType[Unpin]("files.unpin")
}
