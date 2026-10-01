package filemanager

import "github.com/marrasen/gunim"

// dialogID is the ID every dialog is mounted as, one at a time.
const dialogID gunim.ID = "dialog"

// OpView is one running file operation, as the progress panel shows it.
type OpView struct {
	ID    int
	Title string
	// Done is how far it has got, from 0 to 1, and Unknown is set while
	// it is still counting what there is to do.
	Done    float32
	Unknown bool
	Detail  string
}

// Ops is the running file operations.
type Ops struct {
	Ops []OpView
}

// OpTick is a patch: one operation moved along.
type OpTick struct {
	ID      int
	Done    float32
	Unknown bool
	Detail  string
}

// Notice is a patch that shows a toast. Undo, when not zero, is the
// operation its Undo link reverses.
type Notice struct {
	Title, Body string
	Undo        int
	// Kind is success, warning or info, for the toast's icon; empty shows none.
	Kind string
}

// CancelOp asks to stop a running operation.
type CancelOp struct {
	ID int
}

// UndoOp asks to reverse a finished operation.
type UndoOp struct {
	ID int
}

// ClashAsk is the state of the dialog that asks what to do about an item
// going where one of its name is.
type ClashAsk struct {
	Op        int
	Name      string
	Where     string
	New, Old  string
	SameKind  bool
	CanForAll bool
}

// ClashAnswered is the answer to a ClashAsk. Stop stops the operation.
type ClashAnswered struct {
	Op     int
	Choice Choice
	All    bool
	Stop   bool
}

// Choice is an answer to a name clash, on the wire.
type Choice uint8

// The answers to a name clash.
const (
	ChoiceReplace Choice = iota
	ChoiceKeepBoth
	ChoiceSkip
)

// Confirm is the state of a dialog that asks before something that
// cannot be taken back.
type Confirm struct {
	Token       int
	Title, Body string
	OK          string
}

// Confirmed answers a Confirm.
type Confirmed struct {
	Token int
	OK    bool
}

// Prompt is the state of a dialog that asks for a name.
type Prompt struct {
	Token       int
	Title, Text string
	OK          string
	// Stem is how much of Text is selected to begin with, the name
	// without its extension, counted in runes, as TextField.Select
	// counts.
	Stem int
}

// Prompted answers a Prompt.
type Prompted struct {
	Token int
	Text  string
	OK    bool
}

// ErrorBox is the state of a dialog that says an operation failed.
type ErrorBox struct {
	Title, Body string
}

// DialogClosed says a dialog that only tells has closed.
type DialogClosed struct{}

func init() {
	gunim.RegisterType[Ops]("files.ops")
	gunim.RegisterType[OpTick]("files.op-tick")
	gunim.RegisterType[Notice]("files.notice")
	gunim.RegisterType[CancelOp]("files.cancel-op")
	gunim.RegisterType[UndoOp]("files.undo-op")
	gunim.RegisterType[ClashAsk]("files.clash")
	gunim.RegisterType[ClashAnswered]("files.clash-answered")
	gunim.RegisterType[Confirm]("files.confirm")
	gunim.RegisterType[Confirmed]("files.confirmed")
	gunim.RegisterType[Prompt]("files.prompt")
	gunim.RegisterType[Prompted]("files.prompted")
	gunim.RegisterType[ErrorBox]("files.error")
	gunim.RegisterType[DialogClosed]("files.dialog-closed")
}
