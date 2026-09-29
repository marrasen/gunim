package main

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// cardW is how wide an event's card is.
const cardW = 330

// newEventCard makes the card about e: its title, when it is, how it repeats, where, its calendar, who invited the
// user, and its notes, with buttons to answer an invitation, edit and delete. The buttons run done as they act.
func newEventCard(e calendar.Event, d Details, done func(*gunim.UI)) gunim.Node {
	title := widget.NewLabel(e.Title)
	title.Face, title.Size, title.MaxLines = widget.BoldFont, CardTitle, 2
	rows := []gunim.Node{title, line(icon.Clock, when(e))}
	if d.Repeat != Never {
		rows = append(rows, line(icon.Repeat, repeatNames[d.Repeat]))
	}
	if e.Location != "" {
		rows = append(rows, line(icon.MapPin, e.Location))
	}
	rows = append(rows, &calLine{name: d.Calendar, color: e})
	if d.From != "" {
		rows = append(rows, line(icon.Mail, "Invited by "+d.From))
	}
	if d.Notes != "" {
		n := widget.NewLabel(d.Notes)
		n.MaxLines, n.Color = 6, widget.PaletteHint
		rows = append(rows, n)
	}
	button := func(label string, ic *icon.Icon, v gunim.Intent) *widget.Button {
		b := widget.NewButton(label)
		b.Icon = ic
		b.OnActivate(func(u *gunim.UI) {
			u.Send(b, v)
			done(u)
		})
		return b
	}
	var buttons []gunim.Node
	if d.Answer != NotInvited {
		going := button("Going", icon.Check, Answered{ID: e.ID, Answer: Going})
		if d.Answer == NoAnswer {
			going.Kind = widget.ButtonPrimary
		}
		buttons = append(buttons, going, button("Not going", icon.X, Answered{ID: e.ID, Answer: NotGoing}))
	}
	if !e.Fixed {
		buttons = append(buttons, widget.NewSpacer(),
			button("Edit", icon.Pencil, EditAsked{ID: e.ID}),
			button("Delete", icon.Trash2, DeleteAsked{ID: e.ID}))
	}
	if len(buttons) > 0 {
		bar := widget.Row(buttons...)
		for _, b := range buttons {
			if s, ok := b.(*widget.Spacer); ok {
				bar.Grow(s, 1)
			}
		}
		rows = append(rows, bar)
	}
	col := widget.Column(rows...)
	col.Cross = widget.CrossStretch
	return &cardBox{child: widget.NewCard(widget.NewPad(col))}
}

// when says when e is, such as "Tuesday 29 September · 09:15–09:30".
func when(e calendar.Event) string {
	day := "Monday 2 January"
	last := calendar.AddDays(calendar.Day(e.End.Add(-time.Nanosecond)), 0)
	switch {
	case e.AllDay && calendar.SameDay(e.Start, last):
		return e.Start.Format(day)
	case e.AllDay:
		return e.Start.Format(day) + " – " + last.Format(day)
	case calendar.SameDay(e.Start, e.End) || e.End.Equal(calendar.AddDays(calendar.Day(e.Start), 1)):
		return e.Start.Format(day) + " · " + e.Start.Format("15:04") + "–" + e.End.Format("15:04")
	}
	return e.Start.Format(day+" 15:04") + " – " + e.End.Format(day+" 15:04")
}

// line is a row of the card: an icon and a text.
func line(ic *icon.Icon, s string) gunim.Node {
	l := widget.NewLabel(s)
	l.MaxLines = 2
	r := widget.Row(widget.NewIcon(ic, ""), l).Grow(l, 1)
	r.Cross = widget.CrossCenter
	return r
}

// calLine is the card's row naming the event's calendar, with a dot of its colour.
type calLine struct {
	name  string
	color calendar.Event
}

// Layout implements [gunim.Node].
func (c *calLine) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return geom.Sz(cs.Max.W, widget.ControlHeight.Get(f.Theme))
}

// Paint implements [gunim.Node].
func (c *calLine) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	s := widget.IconSize.Get(th)
	p.RRect(geom.Rc(s/2-5, box.H/2-5, 10, 10), 5, paint.Solid(c.color.Color))
	t := widget.Font.Get(th).Shape(c.name, widget.TextSize.Get(th))
	t.Paint(p, geom.Pt(s+8, (box.H-t.Height())/2), widget.Ink.Get(th))
}

// cardBox holds the card at its width, as tall as it needs.
type cardBox struct{ child gunim.Node }

// Children implements [gunim.Composite].
func (b *cardBox) Children() []gunim.Node { return []gunim.Node{b.child} }

// Layout implements [gunim.Node].
func (b *cardBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(cardW, 0), Max: geom.Sz(cardW, c.Max.H)})
	kid.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (b *cardBox) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}
