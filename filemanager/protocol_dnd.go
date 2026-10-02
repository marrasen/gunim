package filemanager

import "github.com/marrasen/gunim"

// The commands of the context menus and the windows.
const (
	CmdOpenSystem = "opensystem"
	CmdDuplicate  = "duplicate"
	CmdProperties = "properties"
	CmdNewWindow  = "newwindow"
)

// DropFiles asks to move Paths into the folder Into, or to copy them
// there with Copy. FS is the ID of the file system the items are on,
// and To that of the one Into is on: the window's, unless Away. Where
// the items or Into are on another file system than the window's, the
// program carries the items, through [Options.Transfer].
type DropFiles struct {
	Paths []string
	Into  string
	Copy  bool
	FS    string
	// To is the ID of the file system Into is on: the window's when the
	// drop was planned, unless Away.
	To string
	// Away says Into is a place or a favourite on To, another file
	// system than the window's.
	Away bool
	// Style is how the items' file system writes their paths.
	Style PathStyle
}

// DragFetch asks the program to fetch the items at Paths, on the
// window's file system, to this computer, for drag ID to carry out to
// other programs: the window's file system is one they cannot reach.
type DragFetch struct {
	ID    int
	Paths []string
}

// DragFetched says drag ID's items are fetched, to Paths on this
// computer, or why not, in a few words for the drag to show, in Err.
type DragFetched struct {
	ID    int
	Paths []string
	Err   string
}

// DragFetching says how far the fetch of drag ID's items has got: Bytes
// of Total fetched, where Total is known.
type DragFetching struct {
	ID           int
	Bytes, Total int64
}

// DragFetchEnd says drag ID has ended, so its fetch, where it still
// runs, may stop.
type DragFetchEnd struct {
	ID int
}

// PinFolders asks to add the folders at Paths to the favourites.
type PinFolders struct {
	Paths []string
}

// Volumes says which volume each folder the window can drop on is on, by
// path, so a drop can tell a move from a copy. Errs says why a folder's
// volume could not be read.
type Volumes struct {
	Of   map[string]string
	Errs map[string]string
}

// ClipState says how many items Paste would paste, and whether they were
// cut, for the menus to offer Paste. The items may be on another file
// system, where the program carries them across.
type ClipState struct {
	Count int
	Cut   bool
}

// OpenWindow asks for a new window on the folder at Path.
type OpenWindow struct {
	Path string
}

// EditFavourite asks to edit the favourite at Path on the file system of
// ID FS: its name, its colour and its icon.
type EditFavourite struct {
	FS, Path string
}

// RenameFavourite asks for the same as EditFavourite, by its older name.
type RenameFavourite struct {
	FS, Path string
}

// FavouriteEdit is the Edit favourite dialog: the favourite's name, its
// colour and its icon, by name, as Favourite has them. Folder is the
// folder's own name, which an empty name stands for.
type FavouriteEdit struct {
	Token  int
	Name   string
	Folder string
	Color  string
	Icon   string
}

// FavouriteEdited answers the Edit favourite dialog: with OK, the name,
// colour and icon to give the favourite.
type FavouriteEdited struct {
	Token int
	OK    bool
	Name  string
	Color string
	Icon  string
}

// Props is the state of the Properties dialog: what the items are, where
// they are, how big, and when they changed. Attrs is set where the
// read-only and hidden attributes can be changed, as on Windows.
type Props struct {
	Token    int
	Title    string
	Name     string
	Type     string
	Location string
	Size     string
	Holds    string
	Counting bool
	Created  string
	Modified string
	Accessed string
	Attrs    bool
	ReadOnly bool
	Hidden   bool
	// Err says why something about the items could not be read.
	Err string
}

// PropsCounted is a patch to the Properties dialog: how much the items
// hold, so far or in all.
type PropsCounted struct {
	Token    int
	Size     string
	Holds    string
	Counting bool
	Err      string
}

// PropsApplied answers the Properties dialog with the attributes to set.
type PropsApplied struct {
	Token    int
	ReadOnly bool
	Hidden   bool
}

// ScriptDrag is a patch that drags in the window as a person would, for
// a script: Step is start, over or drop, on the item or place Name.
type ScriptDrag struct {
	Step string
	Name string
	Mods int
}

func init() {
	gunim.RegisterType[DropFiles]("files.drop")
	gunim.RegisterType[DragFetch]("files.drag-fetch")
	gunim.RegisterType[DragFetched]("files.drag-fetched")
	gunim.RegisterType[DragFetching]("files.drag-fetching")
	gunim.RegisterType[DragFetchEnd]("files.drag-fetch-end")
	gunim.RegisterType[PinFolders]("files.pin-folders")
	gunim.RegisterType[Volumes]("files.volumes")
	gunim.RegisterType[ClipState]("files.clip")
	gunim.RegisterType[OpenWindow]("files.open-window")
	gunim.RegisterType[RenameFavourite]("files.rename-favourite")
	gunim.RegisterType[EditFavourite]("files.edit-favourite")
	gunim.RegisterType[FavouriteEdit]("files.favourite-edit")
	gunim.RegisterType[FavouriteEdited]("files.favourite-edited")
	gunim.RegisterType[Props]("files.props")
	gunim.RegisterType[PropsCounted]("files.props-counted")
	gunim.RegisterType[PropsApplied]("files.props-applied")
	gunim.RegisterType[ScriptDrag]("files.script-drag")
}
