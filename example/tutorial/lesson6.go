package main

import (
	_ "embed"
	"fmt"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/widget"
)

// Lesson 6: a dialog, and views that mount and unmount.
//
// The application puts a view on screen with Client.Mount and takes it
// off with Client.Unmount. The node stays in the tree while it animates
// out, and the engine unlinks it once it reports settled. A dialog
// closes itself on the frame its button is released, and tells the
// application afterwards, so the interface answers at once and the
// application hears in its own time.

//go:embed lesson6.go
var lesson6Source string

// The vocabulary the two halves share.
type (
	// Trash is what the view shows.
	Trash struct{ Files int }
	// EmptyAsked travels when the Empty button is pressed.
	EmptyAsked struct{}
	// Confirm is the dialog's state.
	Confirm struct{ Files int }
	// Emptied travels when the dialog is accepted.
	Emptied struct{}
	// Kept travels when the dialog is dismissed.
	Kept struct{}
	// Restored travels when the files are put back.
	Restored struct{}
)

func init() {
	gunim.RegisterType[Trash]("tutorial.trash")
	gunim.RegisterType[EmptyAsked]("tutorial.empty-asked")
	gunim.RegisterType[Confirm]("tutorial.confirm")
	gunim.RegisterType[Emptied]("tutorial.emptied")
	gunim.RegisterType[Kept]("tutorial.kept")
	gunim.RegisterType[Restored]("tutorial.restored")
}

var lesson6 = lesson{
	Title:  lessonTitles[5],
	File:   lessonFile(5),
	Source: lesson6Source,
	View:   "lesson6",
	Register: func(w *gunim.Window) {
		gunim.RegisterView(w, "lesson6", buildLesson6, (*trashPage).show)
		// The dialog is a view of its own, mounted at the root so it
		// floats over everything. Its buttons send Accept and Dismiss
		// and it removes itself, so its update function is nil.
		gunim.RegisterView(w, "confirm", func(s Confirm) *widget.Dialog {
			d := widget.NewDialog(fmt.Sprintf("Delete %d files for good?", s.Files))
			d.Icon = icon.Trash2
			d.Danger = true
			d.SetButtons("Empty the trash", "Keep them")
			d.Accept, d.Dismiss = Emptied{}, Kept{}
			return d
		}, nil)
	},
	State: func(a *app) any { return Trash{Files: a.trash} },
	Handle: func(a *app, c gunim.Client, in gunim.Intent) bool {
		switch in.(type) {
		case EmptyAsked:
			// Mount builds the view with its state and puts it under
			// the parent. Focus gives it the keyboard, so Enter and
			// Escape answer it.
			_ = c.Mount(gunim.Root, "confirm", "confirm", Confirm{Files: a.trash})
			_ = c.Focus("confirm")
		case Emptied:
			a.trash = 0
		case Kept:
			// The dialog has already closed itself, so the state stays
			// as it is.
		case Restored:
			a.trash = 12
		default:
			return false
		}
		return true
	},
}

const lesson6Intro = `The application puts a view on screen with ` + "`Client.Mount`" + `, naming the parent, an ID, the view and its state. ` + "`Client.Unmount`" + ` starts the view's exit. The node stays in the tree while it animates out, and the engine unlinks it once its ` + "`Transition`" + ` reports settled. Mount the same ID again while it is leaving and it swings back with the velocity it had.

The dialog is a view of its own, mounted at the root so it floats over the page. Local interaction stays local: the dialog closes itself on the frame its button is released, and tells the application afterwards with ` + "`Accept`" + ` or ` + "`Dismiss`" + `. The application then changes its state and the page below updates.

Switching lessons in this window works the same way: the shell's slot gets the new page mounted while the old one is unmounted, and the two cross over.

Press Empty the trash. Escape or Keep them leaves the files where they are. Once the trash is emptied, the window sends a green ring out past its own edges, onto the desktop: a ` + "`widget.Echo`" + `, drawn in a popup window larger than this one.`

// trashPage is the lesson's view, with handles on what changes.
type trashPage struct {
	*page
	files   *widget.Label
	empty   *widget.Button
	restore *widget.Button
	// echo pings past the window's edges when the trash is emptied.
	echo  widget.Echo
	shown int
}

func buildLesson6(s Trash) *trashPage {
	files := widget.NewLabel("")
	files.Size = widget.HeadingSize
	empty := widget.NewButton("Empty the trash")
	empty.Icon, empty.Kind, empty.On = icon.Trash2, widget.ButtonDanger, EmptyAsked{}
	restore := widget.NewButton("Put the files back")
	restore.Icon, restore.On = icon.Undo2, Restored{}
	row := widget.Row(empty, restore, files)
	row.Cross = widget.CrossCenter
	return &trashPage{
		page:    newPage(5, lesson6Intro, row, lesson6Source),
		files:   files,
		empty:   empty,
		restore: restore,
		shown:   s.Files,
	}
}

func (p *trashPage) show(s Trash, u *gunim.UI) {
	switch s.Files {
	case 0:
		p.files.Text = "The trash is empty"
	case 1:
		p.files.Text = "The trash holds 1 file"
	default:
		p.files.Text = fmt.Sprintf("The trash holds %d files", s.Files)
	}
	p.empty.Disabled = s.Files == 0
	p.restore.Disabled = s.Files > 0
	if s.Files == 0 && p.shown > 0 {
		p.echo.Ping(u, widget.EchoDone)
	}
	p.shown = s.Files
	u.Invalidate()
}
