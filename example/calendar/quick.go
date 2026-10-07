package main

import (
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// quickW is how wide the popover that names a new event is.
const quickW = 320

// quickCard is the popover beside an event just drawn out: a field for its title, when it is, and its calendar.
// Enter saves it, Escape gives it up, and More options opens the full editor on what it holds.
type quickCard struct {
	popCard
	name *widget.TextField
	cal  *widget.Dropdown
	d    Draft
	done func(*gunim.UI)
}

// newQuickCard makes the popover for d. It runs done as it saves or gives up, and more as it hands over to the full
// editor.
func newQuickCard(d Draft, left bool, done, more func(*gunim.UI)) *quickCard {
	q := &quickCard{d: d, done: done}
	q.name = widget.NewTextField()
	q.name.Placeholder = "Add a title"
	cals := make([]widget.MenuItem, 0, len(d.Calendars))
	for _, c := range d.Calendars {
		cals = append(cals, widget.MenuItem{Label: c.Name, Swatch: c.Color})
	}
	q.cal = widget.NewDropdown(cals)
	q.cal.SetSelected(max(slices.IndexFunc(d.Calendars, func(c Calendar) bool { return c.ID == d.Calendar }), 0), nil)
	ev := calendar.Event{Start: d.Start, End: d.End, AllDay: d.AllDay}
	day, hours := when2(ev)
	whenLabel := widget.NewLabel(day + " · " + hours)
	whenLabel.Color, whenLabel.MaxLines = widget.PaletteHint, 2
	options := widget.NewButton("More options")
	options.Ghost = true
	options.OnActivate(func(u *gunim.UI) {
		u.Send(q, MoreAsked{Draft: q.draft()})
		more(u)
	})
	save := widget.NewButton("Save")
	save.Kind = widget.ButtonPrimary
	save.OnActivate(func(u *gunim.UI) { q.save(u) })
	spacer := widget.NewSpacer()
	buttons := widget.Row(options, spacer, save).Grow(spacer, 1)
	buttons.Cross = widget.CrossCenter
	col := widget.Column(q.name, line(icon.Clock, whenLabel), q.cal, buttons)
	col.Cross = widget.CrossStretch
	q.popCard = newPopCard(widget.NewPad(col), quickW, left)
	return q
}

// draft returns the event as the popover holds it.
func (q *quickCard) draft() Draft {
	d := q.d
	d.Title = strings.TrimSpace(q.name.Text())
	if q.cal.Selected() < len(d.Calendars) {
		d.Calendar = d.Calendars[q.cal.Selected()].ID
	}
	if d.AllDay {
		// The editor and save take an all-day event's end as its last day.
		d.End = calendar.AddDays(d.End, -1)
	}
	return d
}

// save saves the event, or keeps the keyboard in the title field while it is empty.
func (q *quickCard) save(u *gunim.UI) {
	d := q.draft()
	if d.Title == "" {
		u.Focus(q.name)
		return
	}
	u.Send(q, Saved{Draft: d})
	q.done(u)
}

// Handle implements [gunim.Handler]: Enter saves, and Escape gives the event up.
func (q *quickCard) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	switch k.Key {
	case input.KeyEnter, input.KeyKPEnter:
		q.save(u)
	case input.KeyEscape:
		q.done(u)
	default:
		return false
	}
	return true
}
