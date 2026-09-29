package main

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// editorBody is the editor's form, which Tab moves through.
type editorBody struct {
	*widget.Form
}

// newEditor makes the dialog that writes a new event, or changes one: its title first, then when it is, how it
// repeats, where, its calendar and notes. Ticking All day folds the times away, and the length of the event shows
// beside its end.
func newEditor(d Draft) *widget.Dialog {
	title := "New event"
	if d.ID != "" {
		title = "Edit event"
	}
	dlg := widget.NewDialog(title)
	name := widget.NewTextField()
	name.SetText(d.Title)
	name.Placeholder = "Add a title"
	var calNames []string
	for _, c := range d.Calendars {
		calNames = append(calNames, c.Name)
	}
	cal := widget.NewDropdown(calNames...)
	cal.Selected = max(slices.IndexFunc(d.Calendars, func(c Calendar) bool { return c.ID == d.Calendar }), 0)
	allDay := widget.NewCheckbox("All day")
	allDay.On = d.AllDay
	end := d.End
	if d.AllDay && end.After(d.Start) && end.Equal(calendar.Day(end)) && !calendar.SameDay(end, d.Start) {
		// An all-day event ends at the midnight after its last day, which the field shows as that last day.
		end = calendar.AddDays(end, -1)
	}
	startDay, endDay := calendar.NewDateField(d.Start), calendar.NewDateField(end)
	startTime := calendar.NewTimeField(d.Start.Sub(calendar.Day(d.Start)))
	endTime := calendar.NewTimeField(d.End.Sub(calendar.Day(d.End)))
	if d.AllDay {
		startTime.TextField.SetText("09:00")
		endTime.TextField.SetText("10:00")
	}
	lasts := widget.NewLabel("")
	lasts.Color = widget.PaletteHint
	showLength := func(u *gunim.UI) {
		s, e, ok := times(startDay, startTime, endDay, endTime)
		switch {
		case allDay.On:
			days := int(endDay.Value().Sub(startDay.Value()).Hours()/24+0.5) + 1
			if days <= 1 {
				lasts.SetText("")
			} else {
				lasts.SetText(strconv.Itoa(days) + " days")
			}
		case ok && e.After(s):
			lasts.SetText(lasting(e.Sub(s)))
		default:
			lasts.SetText("")
		}
		if u != nil {
			u.Invalidate()
		}
	}
	showLength(nil)
	// Moving the start moves the end with it, keeping the event's length.
	length := d.End.Sub(d.Start)
	moveEnd := func(u *gunim.UI) {
		t, _ := startTime.Value()
		e := startDay.Value().Add(t).Add(length)
		if allDay.On {
			e = startDay.Value().Add(end.Sub(calendar.Day(d.Start)))
		}
		endDay.SetValue(calendar.Day(e), u)
		if !allDay.On {
			endTime.SetValue(e.Sub(calendar.Day(e)), u)
		}
		showLength(u)
	}
	startDay.OnChange = func(_ time.Time, u *gunim.UI) { moveEnd(u) }
	startTime.OnChange = func(_ time.Duration, u *gunim.UI) { moveEnd(u) }
	endTime.OnChange = func(_ time.Duration, u *gunim.UI) {
		if s, e, ok := times(startDay, startTime, endDay, endTime); ok && e.After(s) {
			length = e.Sub(s)
		}
		showLength(u)
	}
	endDay.OnChange = func(_ time.Time, u *gunim.UI) { endTime.OnChange(0, u) }
	startClock, endClock := newFold(startTime, !d.AllDay), newFold(endTime, !d.AllDay)
	allDay.OnFlip(func(on bool, u *gunim.UI) {
		startClock.open(!on, u)
		endClock.open(!on, u)
		showLength(u)
	})
	repeat := widget.NewDropdown(repeatNames...)
	repeat.Selected = int(d.Repeat)
	where := widget.NewTextField()
	where.SetText(d.Location)
	where.Placeholder = "A room, an address or a link"
	notes := widget.NewTextArea()
	notes.Rows = 3
	notes.SetText(d.Notes)

	starts := widget.Row(startDay, startClock)
	ends := widget.Row(endDay, endClock, lasts)
	ends.Cross = widget.CrossCenter
	form := widget.NewForm()
	if d.Series {
		note := widget.NewLabel("This event repeats. Changes here go to every time it happens.")
		note.Color, note.MaxLines = widget.PaletteHint, 2
		form.Add("", note)
	}
	form.Add("", name).
		Add("", allDay).
		Add("Starts", starts).
		Add("Ends", ends).
		Add("Repeats", repeat).
		Add("Where", where).
		Add("Calendar", cal).
		Add("Notes", notes)
	dlg.Body = &editorBody{Form: form}
	dlg.SetButtons("Save", "Cancel")
	dlg.Dismiss = EditorClosed{}
	dlg.Check = func() string {
		if strings.TrimSpace(name.Text()) == "" {
			return "Give the event a title."
		}
		s, e, ok := times(startDay, startTime, endDay, endTime)
		switch {
		case allDay.On && endDay.Value().Before(startDay.Value()):
			return "The event ends before it starts."
		case allDay.On:
		case !ok:
			return "Write the times as 9:30, or 14:00."
		case !e.After(s):
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

// times reads the start and end the fields hold, and reports false when a time is not one.
func times(startDay *calendar.DateField, startTime *calendar.TimeField, endDay *calendar.DateField,
	endTime *calendar.TimeField,
) (time.Time, time.Time, bool) {
	st, ok1 := startTime.Value()
	et, ok2 := endTime.Value()
	return startDay.Value().Add(st), endDay.Value().Add(et), ok1 && ok2
}

// newDeleteDialog asks whether to delete only this time of an event that repeats, or every time.
func newDeleteDialog(s DeleteAsk) *widget.Dialog {
	d := widget.NewDialog("Delete “" + s.Title + "”?")
	d.Danger = true
	d.Dismiss = DeleteAnswered{ID: s.ID}
	body := widget.NewLabel("This event repeats. Delete only this time, or every time it happens?")
	body.Color, body.MaxLines = widget.PaletteHint, 3
	d.Body = body
	d.SetButtons("Every time", "Cancel")
	d.Accept = DeleteAnswered{ID: s.ID, OK: true, All: true}
	d.AddButton("Only this time", func() gunim.Intent { return DeleteAnswered{ID: s.ID, OK: true} })
	return d
}

// newChangeDialog asks whether moving one time of a repeating event moves only that time, or every time.
func newChangeDialog(s ChangeAsk) *widget.Dialog {
	d := widget.NewDialog("Move “" + s.Title + "”?")
	d.Dismiss = ChangeAnswered{ID: s.ID}
	body := widget.NewLabel("This event repeats. Move only this time, or every time it happens?")
	body.Color, body.MaxLines = widget.PaletteHint, 3
	d.Body = body
	d.SetButtons("Only this time", "Cancel")
	d.Accept = ChangeAnswered{ID: s.ID, OK: true}
	d.AddButton("Every time", func() gunim.Intent { return ChangeAnswered{ID: s.ID, OK: true, All: true} })
	return d
}

// fold holds a field that folds away sideways, narrowing and fading, and unfolds again.
type fold struct {
	anim.Group
	child gunim.Node
	t     *anim.Float
}

func newFold(child gunim.Node, open bool) *fold {
	f := &fold{child: child, t: anim.NewFloat(0)}
	if open {
		f.t.Jump(1)
	}
	f.Add(f.t)
	return f
}

// open unfolds the field, or folds it away.
func (f *fold) open(on bool, u *gunim.UI) {
	f.t.Animate(map[bool]float32{false: 0, true: 1}[on], widget.Settle.Get(u.Theme()))
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (f *fold) Children() []gunim.Node { return []gunim.Node{f.child} }

// Layout implements [gunim.Node]: as wide as the field times how open it is.
func (f *fold) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(gunim.Loose(c.Max))
	kid.Place(geom.Point{})
	t := min(max(f.t.Value(), 0), 1)
	return geom.Sz(s.W*t, s.H)
}

// Paint implements [gunim.Node].
func (f *fold) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	t := min(max(f.t.Value(), 0), 1)
	if t <= 0.01 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: geom.Pt(box.W, box.H)}, Opacity: t, Clip: true})()
	kids.At(0).Paint(p)
}
