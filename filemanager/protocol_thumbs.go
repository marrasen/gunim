package filemanager

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/paint"
)

// The commands of the icon view and the viewer.
const (
	CmdViewDetails = "view.details"
	CmdViewIcons   = "view.icons"
	CmdViewer      = "viewer"
	CmdViewerNext  = "viewer.next"
	CmdViewerPrev  = "viewer.prev"
	CmdViewerClose = "viewer.close"
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

// OpenViewer asks to show the picture in row Row of listing Gen large.
type OpenViewer struct {
	Gen, Row int
}

// ViewerStep asks the viewer for the next picture in the folder, or with Dir -1 the one before.
type ViewerStep struct {
	Dir int
}

// ViewerWants asks for the picture of viewer state Seq decoded to fill W by H pixels.
type ViewerWants struct {
	Seq, W, H int
}

// ViewerClosed says the viewer showing state Seq closed itself.
type ViewerClosed struct {
	Seq int
}

// Viewing is what the viewer shows: the picture at Path, number Index of Count in the folder. Seq changes with each
// new picture, and Travel says which way the viewer went to it.
type Viewing struct {
	Seq          int
	Travel       int
	Path, Name   string
	Index, Count int
	// W and H are the picture's own size, once it is known.
	W, H int
	// Thumb stands in until Image, decoded to fit the screen, arrives.
	Thumb, Image *paint.Image
	Loading      bool
	Err          string
}

func init() {
	gunim.RegisterType[ViewMode]("files.view-mode")
	gunim.RegisterType[NeedThumbs]("files.need-thumbs")
	gunim.RegisterType[Thumb]("files.thumb")
	gunim.RegisterType[TileSized]("files.tile-sized")
	gunim.RegisterType[OpenViewer]("files.open-viewer")
	gunim.RegisterType[ViewerStep]("files.viewer-step")
	gunim.RegisterType[ViewerWants]("files.viewer-wants")
	gunim.RegisterType[ViewerClosed]("files.viewer-closed")
	gunim.RegisterType[Viewing]("files.viewing")
}
