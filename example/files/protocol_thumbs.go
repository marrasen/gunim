package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/paint"
)

// The commands of the icon view.
const (
	CmdViewDetails = "view.details"
	CmdViewIcons   = "view.icons"
)

// ViewMode says how the folder at Path shows, as details or as icons, and how wide an icon's tile is.
type ViewMode struct {
	Path  string
	Icons bool
	Tile  float32
}

// NeedThumbs asks for thumbnails Size pixels across for the rows of listing Gen that Rows lists.
type NeedThumbs struct {
	Gen, Size int
	Rows      []int
}

// Thumb is the thumbnail of the item Name in the folder Dir, at most Size pixels across, or Err says why there is
// none.
type Thumb struct {
	Dir, Name string
	Size      int
	Image     *paint.Image
	Err       string
}

// TileSized carries the width of an icon's tile once the user changes it.
type TileSized struct {
	Size float32
}

func init() {
	gunim.RegisterType[ViewMode]("files.view-mode")
	gunim.RegisterType[NeedThumbs]("files.need-thumbs")
	gunim.RegisterType[Thumb]("files.thumb")
	gunim.RegisterType[TileSized]("files.tile-sized")
}
