package main

import "github.com/marrasen/gunim"

// PaletteQuery carries what is typed in the palette. Seq counts the
// queries, so an answer to one gone by is left alone.
type PaletteQuery struct {
	Seq  int
	Text string
}

// PaletteHit is one thing the palette offers: a file or a folder under
// the folder showing, a command, a place, or a folder that could not be
// read.
type PaletteHit struct {
	Title, Detail, Hint string
	// Key says what picking it does: file:, cmd: or go: and the rest.
	Key string
	// At holds the runes of Title the query matched.
	At      []int
	Problem bool
}

// PaletteResults answers a PaletteQuery, again each time the index
// grows while it is built.
type PaletteResults struct {
	Seq    int
	Hits   []PaletteHit
	Status string
}

// PalettePicked says a hit was picked; Ctrl opens a file with its
// program.
type PalettePicked struct {
	Key  string
	Ctrl bool
}

// OpenPalette is a patch that opens the palette with Query typed.
type OpenPalette struct {
	Query string
}

func init() {
	gunim.RegisterType[PaletteQuery]("files.palette-query")
	gunim.RegisterType[PaletteHit]("files.palette-hit")
	gunim.RegisterType[PaletteResults]("files.palette-results")
	gunim.RegisterType[PalettePicked]("files.palette-picked")
	gunim.RegisterType[OpenPalette]("files.open-palette")
}
