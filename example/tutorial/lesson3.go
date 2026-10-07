package main

import (
	_ "embed"
	"slices"
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/widget"
)

// Lesson 3: a keyed list.
//
// Published state is worth animating only if the list can tell what
// changed. widget.Sync does that from keys: a row that arrived grows
// in, a row that went collapses while its neighbours close the gap,
// and the rest spring to where they belong now.

//go:embed lesson3.go
var lesson3Source string

// The vocabulary the two halves share.
type (
	// Todo is what the view shows.
	Todo struct{ Items []Item }
	// Item is one row. Its ID is what the list keys rows by, so a row
	// keeps its identity when its text changes.
	Item struct {
		ID   int
		Text string
	}
	// Added travels when text is submitted in the field.
	Added struct{ Text string }
	// Removed travels when a row's cross is pressed.
	Removed struct{ ID int }
	// Reversed travels when the order is turned round.
	Reversed struct{}
)

func init() {
	gunim.RegisterType[Todo]("tutorial.todo")
	gunim.RegisterType[Added]("tutorial.added")
	gunim.RegisterType[Removed]("tutorial.removed")
	gunim.RegisterType[Reversed]("tutorial.reversed")
}

var lesson3 = lesson{
	Title:  lessonTitles[2],
	File:   lessonFile(2),
	Source: lesson3Source,
	View:   "lesson3",
	Register: func(w *gunim.Window) {
		gunim.RegisterView(w, "lesson3", buildLesson3, (*todoPage).show)
	},
	State: func(a *app) any { return Todo{Items: slices.Clone(a.items)} },
	Handle: func(a *app, _ gunim.Client, in gunim.Intent) bool {
		switch in := in.(type) {
		case Added:
			a.add(in.Text)
		case Removed:
			a.items = slices.DeleteFunc(a.items, func(it Item) bool { return it.ID == in.ID })
		case Reversed:
			slices.Reverse(a.items)
		default:
			return false
		}
		return true
	},
}

// add puts an item with text at the end of the list, with a fresh ID.
func (a *app) add(text string) {
	if text == "" {
		return
	}
	a.nextID++
	a.items = append(a.items, Item{ID: a.nextID, Text: text})
}

const lesson3Intro = `The application publishes the whole list every time. ` + "`widget.Sync`" + ` compares it with the rows on screen by key, so it knows which row is new, which is gone, and which moved. A new row grows in, a gone row collapses while its neighbours close the gap, and the rest spring to their new places.

The key is the item's ID. Keep IDs stable and rows keep their identity through every change; key by text and an edit would look like a removal and an arrival.

Type something and press Enter to add a row. Press a row's cross to take it out. Reverse turns the order round and every row slides to its new place.

The state handed over is a clone, because sending a value hands it over: the application leaves it alone afterwards and keeps its own.`

// todoPage is the lesson's view, with a handle on the list.
type todoPage struct {
	*page
	list *widget.List
}

func buildLesson3(Todo) *todoPage {
	field := widget.NewTextField()
	field.Placeholder = "Something to do, then Enter"
	// OnSubmit turns the text into an intent. Emptying the field is
	// local to the window, so it happens here, on the way out.
	field.OnSubmit = func(text string) gunim.Intent {
		field.SetText("", nil)
		return Added{Text: text}
	}
	reverse := widget.NewButton("Reverse")
	reverse.Icon = icon.ArrowUpDown
	reverse.On = Reversed{}
	header := widget.Row(field, reverse).Grow(field, 1)
	header.Cross = widget.CrossCenter
	list := widget.NewList()
	col := widget.Column(header, list)
	col.Cross = widget.CrossStretch
	return &todoPage{
		page: newPage(2, lesson3Intro, col, lesson3Source),
		list: list,
	}
}

// show syncs the list to the state. newRow builds a row for an item
// seen for the first time; the nil at the end says rows show what they
// were built with, since an item's text stays put here.
func (p *todoPage) show(s Todo, u *gunim.UI) {
	widget.Sync(p.list, u, s.Items,
		func(it Item) widget.Key { return widget.Key(strconv.Itoa(it.ID)) },
		newRow, nil)
}

// newRow makes the row for one item: its text, and a cross that
// removes it.
func newRow(it Item) *widget.Card {
	text := widget.NewLabel(it.Text)
	remove := widget.NewIconButton(icon.X, "Remove")
	remove.On = Removed{ID: it.ID}
	row := widget.Row(text, remove).Grow(text, 1)
	row.Cross = widget.CrossCenter
	return widget.NewCard(row)
}
