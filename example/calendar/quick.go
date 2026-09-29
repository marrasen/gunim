package main

import (
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// quickW is how wide the popover that names a new event is.
const quickW = 320

// quickCard is the popover beside an event just drawn out: a field for its title, when it is, and its calendar.
// Enter saves it, Escape gives it up, and More options opens the full editor on what it holds.
type quickCard struct {
	anim.Group
	child  gunim.Node
	name   *widget.TextField
	cal    *widget.Dropdown
	d      Draft
	left   bool
	done   func(*gunim.UI)
	in     *anim.Float
	margin float32
}

func newQuickCard(d Draft, left bool, done func(*gunim.UI)) *quickCard {
	q := &quickCard{d: d, left: left, done: done, in: anim.NewFloat(0)}
	q.Add(q.in)
	q.name = widget.NewTextField()
	q.name.Placeholder = "Add a title"
	var names []string
	for _, c := range d.Calendars {
		names = append(names, c.Name)
	}
	q.cal = widget.NewDropdown(names...)
	q.cal.Selected = max(slices.IndexFunc(d.Calendars, func(c Calendar) bool { return c.ID == d.Calendar }), 0)
	ev := calendar.Event{Start: d.Start, End: d.End, AllDay: d.AllDay}
	day, hours := when2(ev)
	whenLabel := widget.NewLabel(day + " · " + hours)
	whenLabel.Color, whenLabel.MaxLines = widget.PaletteHint, 2
	more := widget.NewButton("More options")
	more.Ghost = true
	more.OnActivate(func(u *gunim.UI) {
		u.Send(q, MoreAsked{Draft: q.draft()})
		q.done(u)
	})
	save := widget.NewButton("Save")
	save.Kind = widget.ButtonPrimary
	save.OnActivate(func(u *gunim.UI) { q.save(u) })
	spacer := widget.NewSpacer()
	buttons := widget.Row(more, spacer, save).Grow(spacer, 1)
	buttons.Cross = widget.CrossCenter
	col := widget.Column(q.name, line(icon.Clock, whenLabel), q.cal, buttons)
	col.Cross = widget.CrossStretch
	q.child = widget.NewPad(col)
	return q
}

// draft returns the event as the popover holds it.
func (q *quickCard) draft() Draft {
	d := q.d
	d.Title = strings.TrimSpace(q.name.Text())
	if q.cal.Selected < len(d.Calendars) {
		d.Calendar = d.Calendars[q.cal.Selected].ID
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

// Children implements [gunim.Composite].
func (q *quickCard) Children() []gunim.Node { return []gunim.Node{q.child} }

// PopupPadding implements [gunim.PopupPadder]: room for the shadow.
func (q *quickCard) PopupPadding() geom.Insets { return geom.Uniform(q.margin) }

// Layout implements [gunim.Node].
func (q *quickCard) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	q.margin = 0
	if f.Transparent {
		q.margin = widget.MenuMargin.Get(f.Theme)
	}
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(quickW, 0), Max: geom.Sz(quickW, max(cs.Max.H-2*q.margin, 0))})
	kid.Place(geom.Pt(q.margin, q.margin))
	return geom.Sz(s.W+2*q.margin, s.H+2*q.margin)
}

// Paint implements [gunim.Node]: it grows out of the side facing the event, as the event's card does.
func (q *quickCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	t := min(max(q.in.Value(), 0), 1)
	card := geom.Rect{Min: geom.Pt(q.margin, q.margin), Max: geom.Pt(box.W-q.margin, box.H-q.margin)}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: t})()
	pivot := geom.Pt(card.Min.X, card.Min.Y+24)
	if q.left {
		pivot.X = card.Max.X
	}
	defer p.Push(paint.Scale(0.92+0.08*t, pivot))()
	radius := widget.MenuRadius.Get(th)
	if q.margin > 0 {
		p.ShadowRRect(card, radius, paint.Solid(widget.MenuFill.Get(th)), paint.Shadow{Offset: geom.Pt(0, 4),
			Blur: q.margin * 0.7, Color: widget.MenuShadow.Get(th)})
	} else {
		p.RRect(card, radius, paint.Solid(widget.MenuFill.Get(th)))
	}
	p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: widget.MenuBorder.Get(th)})
	kids.At(0).Paint(p)
}

// Transition implements [gunim.Transitioner].
func (q *quickCard) Transition(pr gunim.Presence, f gunim.Frame) bool {
	switch pr {
	case gunim.Entering:
		q.in.Animate(1, widget.Bounce.Get(f.Theme))
	case gunim.Exiting:
		q.in.Animate(0, widget.Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !q.in.Active()
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
