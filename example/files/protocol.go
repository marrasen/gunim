// Command files is a file manager: a real one, which copies, moves,
// renames and trashes, built on gunim.
//
// The window and the application are two halves that speak only in
// values. The protocol files hold the vocabulary, one file per area. The
// app files are the application half, which does all the disk work. The
// view files are the window half. main.go joins them.
//
//	CGO_ENABLED=0 go run ./example/files
//	go run ./example/files -dir /some/folder
package main

import "github.com/marrasen/gunim"

// browserID is the ID of the view that holds the whole window.
const browserID gunim.ID = "browser"

// Shell is the state of the window as a whole.
type Shell struct {
	Light       bool
	ShowHidden  bool
	ShowPreview bool
	// Sidebar is the sidebar's width.
	Sidebar float32
}

// Crumb is one folder of the path bar.
type Crumb struct {
	Name, Path string
}

// Listing is the folder the main area shows. Gen changes each time its
// rows change, by a new listing, a sort or a filter.
type Listing struct {
	Gen    int
	Path   string
	Title  string
	Crumbs []Crumb
	// Total is how many rows show, and All how many entries the folder
	// holds.
	Total, All int
	Sort       SortBy
	Desc       bool
	Filter     string
	// Travel is the way the user went to get here: 1 into a folder or
	// forward, -1 up or back, and 0 elsewhere or nowhere.
	Travel  int
	Loading bool
	// Err says why the folder cannot be shown.
	Err                        string
	CanBack, CanForward, CanUp bool
}

// Row is one entry as the grid shows it.
type Row struct {
	Name     string
	Kind     Kind
	Dir      bool
	Hidden   bool
	Broken   bool
	Size     string
	Modified string
	Type     string
	// Tint picks the colour of the row's mark.
	Tint Tint
}

// Tint is a group of kinds of file that share a colour.
type Tint uint8

// The groups of files.
const (
	TintOther Tint = iota
	TintFolder
	TintImage
	TintVideo
	TintAudio
	TintArchive
	TintDocument
	TintCode
	TintProgram
)

// RowBlock carries rows of listing Gen, from Start on.
type RowBlock struct {
	Gen, Start int
	Rows       []Row
}

// Selection sets the rows selected in listing Gen, as runs of rows, and
// the row the keyboard is on.
type Selection struct {
	Gen    int
	Runs   [][2]int
	Cursor int
}

// Band is what the overview strip shows for a stretch of rows: the
// share of each tint, by Tint, and the first name.
type Band struct {
	Shares []float32
	First  string
}

// Bands is the overview strip of listing Gen.
type Bands struct {
	Gen   int
	Bands []Band
}

// Status is the status bar.
type Status struct {
	Left, Right string
}

// Banner is a line under the path bar, for an error that stopped
// something. An empty Text hides it.
type Banner struct {
	Seq  int
	Text string
}

// NeedRows asks for the blocks of listing Gen that start at Starts.
type NeedRows struct {
	Gen    int
	Starts []int
}

// Selected says which rows of listing Gen are selected, and which row
// the keyboard is on.
type Selected struct {
	Gen    int
	Runs   [][2]int
	Cursor int
}

// Activated says a row of listing Gen was double clicked or had Enter
// pressed on it.
type Activated struct {
	Gen, Row int
}

// Navigate asks to show the folder at Path.
type Navigate struct {
	Path string
}

// Command asks for one of the commands of the menus and keys, by name.
type Command struct {
	Name string
}

// The commands.
const (
	CmdBack       = "back"
	CmdForward    = "forward"
	CmdUp         = "up"
	CmdHome       = "home"
	CmdRefresh    = "refresh"
	CmdOpen       = "open"
	CmdCopy       = "copy"
	CmdCut        = "cut"
	CmdPaste      = "paste"
	CmdTrash      = "trash"
	CmdDelete     = "delete"
	CmdRename     = "rename"
	CmdNewFolder  = "newfolder"
	CmdUndo       = "undo"
	CmdPin        = "pin"
	CmdHidden     = "hidden"
	CmdPreview    = "preview"
	CmdTheme      = "theme"
	CmdCopyPath   = "copypath"
	CmdReveal     = "reveal"
	CmdCloseApp   = "close"
	CmdSortName   = "sort.name"
	CmdSortSize   = "sort.size"
	CmdSortTime   = "sort.time"
	CmdSortType   = "sort.type"
	CmdSelectNone = "selectnone"
)

// SortClicked says a column's title was clicked.
type SortClicked struct {
	Column int
}

// FilterChanged carries the filter field's text as it is typed.
type FilterChanged struct {
	Text string
}

// WindowFocused says the window has the keyboard back.
type WindowFocused struct{}

// FocusListing is a patch that gives the keyboard to the listing, as a
// dialog closes.
type FocusListing struct{}

// CloseAsked travels when the user asks to close the window.
type CloseAsked struct{}

// SidebarMoved carries the sidebar's width once its divider is let go.
type SidebarMoved struct {
	Width float32
}

func init() {
	gunim.RegisterType[Shell]("files.shell")
	gunim.RegisterType[Listing]("files.listing")
	gunim.RegisterType[RowBlock]("files.rows")
	gunim.RegisterType[Selection]("files.selection")
	gunim.RegisterType[Bands]("files.bands")
	gunim.RegisterType[Status]("files.status")
	gunim.RegisterType[Banner]("files.banner")
	gunim.RegisterType[NeedRows]("files.need-rows")
	gunim.RegisterType[Selected]("files.selected")
	gunim.RegisterType[Activated]("files.activated")
	gunim.RegisterType[Navigate]("files.navigate")
	gunim.RegisterType[Command]("files.command")
	gunim.RegisterType[SortClicked]("files.sort")
	gunim.RegisterType[FilterChanged]("files.filter")
	gunim.RegisterType[WindowFocused]("files.focused")
	gunim.RegisterType[SidebarMoved]("files.sidebar-moved")
	gunim.RegisterType[CloseAsked]("files.close-asked")
	gunim.RegisterType[FocusListing]("files.focus-listing")
}
