package main

import (
	"slices"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/widget"
)

// editorBody is the editor's form, which Tab moves through.
type editorBody struct {
	*widget.Form
}

// newEditor makes the dialog that writes a new event, or changes one.
func newEditor(d Draft) *widget.Dialog {
	title := "New event"
	if d.ID != "" {
		title = "Edit event"
	}
	dlg := widget.NewDialog(title)
	name := widget.NewTextField()
	name.SetText(d.Title)
	name.Placeholder = "Title"
	var calNames []string
	for _, c := range d.Calendars {
		calNames = append(calNames, c.Name)
	}
	cal := widget.NewDropdown(calNames...)
	cal.Selected = max(slices.IndexFunc(d.Calendars, func(c Calendar) bool { return c.ID == d.Calendar }), 0)
	allDay := widget.NewCheckbox("All day")
	allDay.On = d.AllDay
	end := d.End
	if d.AllDay {
		// An all-day event ends at the midnight after its last day, which the field shows as that last day.
		end = calendar.AddDays(end, -1)
	}
	startDay, endDay := calendar.NewDateField(d.Start), calendar.NewDateField(end)
	startTime := calendar.NewTimeField(d.Start.Sub(calendar.Day(d.Start)))
	endTime := calendar.NewTimeField(d.End.Sub(calendar.Day(d.End)))
	// Moving the start moves the end with it, keeping the event's length.
	length := d.End.Sub(d.Start)
	startDay.OnChange = func(day time.Time, u *gunim.UI) {
		t, _ := startTime.Value()
		e := day.Add(t).Add(length)
		endDay.SetValue(calendar.Day(e), u)
		endTime.SetValue(e.Sub(calendar.Day(e)), u)
	}
	startTime.OnChange = startTimeChanged(startDay, startTime, endDay, endTime, &length)
	endTime.OnChange = func(time.Duration, *gunim.UI) {
		if s, e, ok := times(startDay, startTime, endDay, endTime); ok && e.After(s) {
			length = e.Sub(s)
		}
	}
	repeat := widget.NewDropdown(repeatNames...)
	repeat.Selected = int(d.Repeat)
	where := widget.NewTextField()
	where.SetText(d.Location)
	where.Placeholder = "A room, an address or a link"
	notes := widget.NewTextArea()
	notes.Rows = 3
	notes.SetText(d.Notes)

	starts := widget.Row(startDay, startTime)
	ends := widget.Row(endDay, endTime)
	form := widget.NewForm().
		Add("Title", name).
		Add("Calendar", cal).
		Add("", allDay).
		Add("Starts", starts).
		Add("Ends", ends).
		Add("Repeats", repeat).
		Add("Where", where).
		Add("Notes", notes)
	dlg.Body = &editorBody{Form: form}
	if d.Series {
		note := widget.NewLabel("This event repeats. Changes here go to every time it happens.")
		note.Color, note.MaxLines = widget.PaletteHint, 2
		form.Add("", note)
	}
	dlg.SetButtons("Save", "Cancel")
	dlg.Dismiss = EditorClosed{}
	dlg.Check = func() string {
		if strings.TrimSpace(name.Text()) == "" {
			return "Give the event a title."
		}
		s, e, ok := times(startDay, startTime, endDay, endTime)
		switch {
		case !ok && !allDay.On:
			return "Write the times as 9:30, or 14:00."
		case allDay.On && endDay.Value().Before(startDay.Value()):
			return "The event ends before it starts."
		case !allDay.On && !e.After(s):
			return "The event ends before it starts."
		}
		return ""
	}
	dlg.OnAccept = func() gunim.Intent {
		out := Draft{ID: d.ID, Title: strings.TrimSpace(name.Text()), Location: strings.TrimSpace(where.Text()),
			Notes: strings.TrimSpace(notes.Text()), AllDay: allDay.On, Repeat: Repeat(repeat.Selected)}
		if cal.Selected < len(d.Calendars) {
			out.Calendar = d.Calendars[cal.Selected].ID
		}
		if out.AllDay {
			out.Start, out.End = startDay.Value(), endDay.Value()
		} else {
			out.Start, out.End, _ = times(startDay, startTime, endDay, endTime)
		}
		return Saved{Draft: out}
	}
	return dlg
}

// startTimeChanged moves the end with the start, keeping the event's length.
func startTimeChanged(startDay *calendar.DateField, startTime *calendar.TimeField, endDay *calendar.DateField,
	endTime *calendar.TimeField, length *time.Duration,
) func(time.Duration, *gunim.UI) {
	return func(t time.Duration, u *gunim.UI) {
		e := startDay.Value().Add(t).Add(*length)
		endDay.SetValue(calendar.Day(e), u)
		endTime.SetValue(e.Sub(calendar.Day(e)), u)
	}
}

// times reads the start and end the fields hold, and reports false when a time is not one.
func times(startDay *calendar.DateField, startTime *calendar.TimeField, endDay *calendar.DateField,
	endTime *calendar.TimeField,
) (time.Time, time.Time, bool) {
	st, ok1 := startTime.Value()
	et, ok2 := endTime.Value()
	return startDay.Value().Add(st), endDay.Value().Add(et), ok1 && ok2
}

// newDeleteDialog asks whether to delete an event, and for one that repeats, whether only this time.
func newDeleteDialog(s DeleteAsk) *widget.Dialog {
	d := widget.NewDialog("Delete “" + s.Title + "”?")
	d.Danger = true
	d.Dismiss = DeleteAnswered{ID: s.ID}
	if s.Series {
		body := widget.NewLabel("This event repeats. Delete only this time, or every time it happens?")
		body.Color, body.MaxLines = widget.PaletteHint, 3
		d.Body = body
		d.SetButtons("Every time", "Cancel")
		d.Accept = DeleteAnswered{ID: s.ID, OK: true, All: true}
		d.AddButton("Only this time", func() gunim.Intent { return DeleteAnswered{ID: s.ID, OK: true} })
		return d
	}
	body := widget.NewLabel("The event goes from the calendar.")
	body.Color = widget.PaletteHint
	d.Body = body
	d.SetButtons("Delete", "Cancel")
	d.Accept = DeleteAnswered{ID: s.ID, OK: true}
	return d
}
