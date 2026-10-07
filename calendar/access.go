package calendar

import (
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

// What each view tells a screen reader, and what it does when a screen reader asks. See [gunim.Accessible].

// eventParts returns an item for each event a view shows, in the order its keys walk them, with the chosen one
// selected, and the index of that one plus one as the view's Active. box finds an event's box in the view's space.
func eventParts(stops []stop, sprites map[string]*sprite, order []string, selected string,
	box func(id string) (geom.Rect, bool)) (parts []access.Info, active int) {
	byID := make(map[string]*sprite, len(order))
	for _, k := range order {
		if s := sprites[k]; s != nil && !s.gone {
			byID[s.e.ID] = s
		}
	}
	for _, st := range stops {
		part := access.Info{Role: access.RoleListItem, Name: st.id, Actions: []string{access.ActionPress}}
		if s := byID[st.id]; s != nil {
			part.Name = s.e.Start.Format("Mon 2 Jan") + ", " + eventTip(s.e)
		}
		part.Bounds, _ = box(st.id)
		if st.id == selected {
			part.State = access.StateSelected
			active = len(parts) + 1
		}
		parts = append(parts, part)
	}
	return parts, active
}

// openStop chooses the event of part i of stops, as eventParts lists them, and opens it as Enter does.
func openStop(i int, stops []stop, choose func(id string), open func(id string, box geom.Rect, u *gunim.UI) gunim.Intent,
	n gunim.Node, u *gunim.UI) bool {
	if i < 0 || i >= len(stops) {
		return false
	}
	s := stops[i]
	choose(s.id)
	if open != nil {
		send(u, n, open(s.id, s.box, u))
	}
	return true
}

// span names the days from first to last, such as "Mon 5 Oct – Sun 11 Oct 2026", or one day alone.
func span(first, last time.Time) string {
	if SameDay(first, last) {
		return first.Format("Monday 2 January 2006")
	}
	return first.Format("Mon 2 Jan") + " – " + last.Format("Mon 2 Jan 2006")
}

// Access implements [gunim.Accessible]: a list of the events shown, named by the days, the chosen event active.
func (d *Days) Access() access.Info {
	parts, active := eventParts(d.stops(), d.sprites, d.order, d.selected, d.EventBox)
	return access.Info{Role: access.RoleList, Name: span(d.First, d.day(max(d.Count, 1)-1)),
		Description: strconv.Itoa(len(parts)) + " events", Parts: parts, Active: active}
}

// AccessAct implements [gunim.AccessActor]: pressing an event chooses it and opens it, as Enter does.
func (d *Days) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	return openStop(r.Part, d.stops(), func(id string) { d.choose(id, u) }, d.OnOpen, d, u)
}

// Access implements [gunim.Accessible]: a list of the events shown, named by the month, the chosen event active.
func (m *Month) Access() access.Info {
	parts, active := eventParts(m.stops(), m.sprites, m.order, m.selected, m.EventBox)
	return access.Info{Role: access.RoleList, Name: MonthStart(m.Month).Format("January 2006"),
		Description: strconv.Itoa(len(parts)) + " events", Parts: parts, Active: active}
}

// AccessAct implements [gunim.AccessActor]: pressing an event chooses it and opens it, as Enter does.
func (m *Month) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	return openStop(r.Part, m.stops(), func(id string) { m.SetSelected(id, u) }, m.OnOpen, m, u)
}

// Access implements [gunim.Accessible]: a table of the month showing, its two arrows first, then a cell for each
// day, the days marked selected and the first of them active.
func (m *MiniMonth) Access() access.Info {
	back, next := m.arrows()
	info := access.Info{Role: access.RoleTable, Name: m.month.Format("January 2006"), Parts: []access.Info{
		{Role: access.RoleButton, Name: "Month before", Bounds: back, Actions: []string{access.ActionPress}},
		{Role: access.RoleButton, Name: "Month after", Bounds: next, Actions: []string{access.ActionPress}},
	}}
	for i := range 42 {
		day := m.day(i)
		part := access.Info{Role: access.RoleCell, Name: day.Format("Monday 2 January 2006"), Bounds: m.cell(i),
			Actions: []string{access.ActionPress}}
		if !day.Before(m.From) && !day.After(m.To) {
			part.State = access.StateSelected
		}
		if SameDay(day, m.From) {
			info.Active = len(info.Parts) + 1
		}
		info.Parts = append(info.Parts, part)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing an arrow turns the month, and pressing a day picks it.
func (m *MiniMonth) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	switch {
	case r.Part == 0:
		m.show(m.month.AddDate(0, -1, 0), u)
	case r.Part == 1:
		m.show(m.month.AddDate(0, 1, 0), u)
	case r.Part >= 2 && r.Part < 2+42:
		m.pick(m.day(r.Part-2), u)
	default:
		return false
	}
	return true
}

// Access implements [gunim.Accessible]: a drop-down holding the day, open while the small month shows.
func (f *DateField) Access() access.Info {
	info := access.Info{Role: access.RoleComboBox, Name: f.Label, Value: f.value.Format("Monday 2 January 2006"),
		State: access.StateExpandable | access.StateHasPopup, Actions: []string{access.ActionPress}}
	if f.popup != nil && f.popup.Open() {
		info.State |= access.StateExpanded
	}
	if f.Disabled {
		info.State |= access.StateDisabled
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: press opens the small month, or closes it.
func (f *DateField) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || f.Disabled {
		return false
	}
	if f.popup != nil && f.popup.Open() {
		f.close(u)
	} else {
		f.open(u)
	}
	return true
}

// Access implements [gunim.Accessible]: a text field holding the time, which opens a list of times.
func (f *TimeField) Access() access.Info {
	info := f.TextField.Access()
	info.State |= access.StateHasPopup | access.StateExpandable
	if f.list != nil && f.list.Open() {
		info.State |= access.StateExpanded
	}
	return info
}
