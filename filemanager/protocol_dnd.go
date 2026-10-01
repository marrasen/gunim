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
// and To that of the one Into is on, the window's: where they differ,
// the program carries the items across, through [Options.Transfer].
type DropFiles struct {
	Paths []string
	Into  string
	Copy  bool
	FS    string
	// To is the ID of the file system Into is on: the window's when the
	// drop was planned.
	To string
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

// RenameFavourite asks for a new name for the favourite at Path on the
// file system of ID FS.
type RenameFavourite struct {
	FS, Path string
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
	gunim.RegisterType[PinFolders]("files.pin-folders")
	gunim.RegisterType[Volumes]("files.volumes")
	gunim.RegisterType[ClipState]("files.clip")
	gunim.RegisterType[OpenWindow]("files.open-window")
	gunim.RegisterType[RenameFavourite]("files.rename-favourite")
	gunim.RegisterType[Props]("files.props")
	gunim.RegisterType[PropsCounted]("files.props-counted")
	gunim.RegisterType[PropsApplied]("files.props-applied")
	gunim.RegisterType[ScriptDrag]("files.script-drag")
}
