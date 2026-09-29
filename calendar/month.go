package calendar

import (
	"image/color"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Sizes of a [Month].
const (
	monthHeadH = 28
	chipH      = 20
	chipGap    = 2
	dayNumH    = 26
)

// Month shows a month as six weeks of days, each with its events: those lasting whole days as bars across the
// days they cover, and the rest as a line each with its time. A day with more than it has room for says how many
// more.
//
// A click on a day's number, or on how many more it has, picks the day. A double click on free room in a day
// begins an event there. Dragging an event to another day moves it there, keeping its time of day.
type Month struct {
	anim.Group
	// Month is any day of the month shown, and FirstWeekday the day weeks start on.
	Month        time.Time
	FirstWeekday time.Weekday
	// OnDay turns a click on a day's number into an intent, such as to show that day alone.
	OnDay func(day time.Time) gunim.Intent
	// OnCreate turns a double click on a day into an intent, to make an event that day.
	OnCreate func(day time.Time) gunim.Intent
	// OnChange turns an event dragged to another day into an intent.
	OnChange func(id string, start, end time.Time) gunim.Intent
	// Open runs when the user clicks an event, with its box in the month's space.
	Open func(id string, box geom.Rect, u *gunim.UI)

	events []Event
	box    geom.Size
	// slide carries the days in from the side they came from, as the month steps.
	slide *anim.Float
	laid  bool
	hover string
	drag  *monthDrag
	texts map[textKey]text.Paragraph
}

// monthDrag is an event taken hold of: where the pointer took it, and the day it is over.
type monthDrag struct {
	id    string
	fixed bool
	from  geom.Point
	box   geom.Rect
	moved bool
	to    int
	start int
}

// NewMonth returns the month holding day, with weeks starting on Monday.
func NewMonth(day time.Time) *Month {
	m := &Month{Month: MonthStart(day), FirstWeekday: time.Monday, slide: anim.NewFloat(0)}
	m.Add(m.slide)
	return m
}

// SetMonth shows the month holding day, sliding it in from its side.
func (m *Month) SetMonth(day time.Time, u *gunim.UI) {
	month := MonthStart(day)
	if m.laid && !month.Equal(m.Month) {
		m.slide.Jump(map[bool]float32{false: -1, true: 1}[month.After(m.Month)])
		m.slide.Animate(0, widget.Quick.Get(u.Theme()))
	}
	m.Month = month
	u.Invalidate()
}

// EventBox returns where the event id first shows, in the month's space, and false when it does not show.
func (m *Month) EventBox(id string) (geom.Rect, bool) {
	for _, ch := range m.chips() {
		if ch.more == 0 && ch.e.ID == id {
			return ch.box, true
		}
	}
	return geom.Rect{}, false
}

// SetEvents shows events, which may hold events outside the weeks shown.
func (m *Month) SetEvents(events []Event, u *gunim.UI) {
	m.events = events
	m.texts = nil
	u.Invalidate()
}

// first returns the first day shown: the start of the week holding the month's first day.
func (m *Month) first() time.Time { return WeekStart(MonthStart(m.Month), m.FirstWeekday) }

// day returns the midnight of cell i, counting from the top left.
func (m *Month) day(i int) time.Time { return AddDays(m.first(), i) }

func (m *Month) cellW() float32 { return m.box.W / 7 }
func (m *Month) cellH() float32 { return (m.box.H - monthHeadH) / 6 }

// cell returns cell i's box.
func (m *Month) cell(i int) geom.Rect {
	return geom.Rc(float32(i%7)*m.cellW(), monthHeadH+float32(i/7)*m.cellH(), m.cellW(), m.cellH())
}

// cellAt returns the cell under pt, or -1.
func (m *Month) cellAt(pt geom.Point) int {
	if pt.Y < monthHeadH || pt.X < 0 || pt.X >= m.box.W || pt.Y >= m.box.H {
		return -1
	}
	return min(int(pt.X/m.cellW()), 6) + 7*min(int((pt.Y-monthHeadH)/m.cellH()), 5)
}

// Layout implements [gunim.Node].
func (m *Month) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	m.box, m.laid = c.Max, true
	return m.box
}

// chip is an event laid out in the month: its box, and whether it is a bar across whole days.
type chip struct {
	e   Event
	box geom.Rect
	bar bool
	// more is set on the line that says how many more a day has, for the day in cell.
	more, cell int
}

// chips lays out the month's events. Each week's bars take rows from the top of its days, each in the first row
// free across its days, and each day's timed events go in the rows left under them.
func (m *Month) chips() []chip {
	var out []chip
	rows := max(int((m.cellH()-dayNumH-4)/(chipH+chipGap)), 1)
	evs := slices.Clone(m.shown())
	slices.SortStableFunc(evs, func(a, b Event) int {
		if a.long() != b.long() {
			return map[bool]int{true: -1, false: 1}[a.long()]
		}
		return a.Start.Compare(b.Start)
	})
	for week := range 6 {
		// taken[row][day] says the row is used on that day, and bars that a bar uses it.
		taken := make([][7]bool, rows)
		bars := make([][7]bool, rows)
		hidden := [7]int{}
		place := func(e Event, first, last int, bar bool) {
			for r := range rows {
				free := true
				for dd := first; dd <= last; dd++ {
					if taken[r][dd] {
						free = false
					}
				}
				if !free {
					continue
				}
				for dd := first; dd <= last; dd++ {
					taken[r][dd], bars[r][dd] = true, bar
				}
				c0, c1 := m.cell(week*7+first), m.cell(week*7+last)
				y := c0.Min.Y + dayNumH + float32(r)*(chipH+chipGap)
				out = append(out, chip{e: e, bar: bar, box: geom.Rc(c0.Min.X+3, y, c1.Max.X-c0.Min.X-6, chipH)})
				return
			}
			for dd := first; dd <= last; dd++ {
				hidden[dd]++
			}
		}
		for _, e := range evs {
			first, last := -1, -1
			for dd := range 7 {
				if e.covers(m.day(week*7 + dd)) {
					if first < 0 {
						first = dd
					}
					last = dd
				}
			}
			if first < 0 {
				continue
			}
			if e.long() {
				place(e, first, last, true)
				continue
			}
			for dd := first; dd <= last; dd++ {
				place(e, dd, dd, false)
			}
		}
		// A day with events that did not fit gives up its last line to say how many more, unless a bar holds it.
		for dd := range 7 {
			if hidden[dd] == 0 || bars[rows-1][dd] {
				continue
			}
			cell := week*7 + dd
			c := m.cell(cell)
			y := c.Min.Y + dayNumH + float32(rows-1)*(chipH+chipGap)
			n := hidden[dd]
			out = slices.DeleteFunc(out, func(ch chip) bool {
				if !ch.bar && ch.box.Min.Y == y && ch.box.Min.X == c.Min.X+3 {
					n++
					return true
				}
				return false
			})
			out = append(out, chip{more: n, cell: cell, box: geom.Rc(c.Min.X+3, y, c.Size().W-6, chipH)})
		}
	}
	return out
}

// shown returns the events, with the one being dragged on the day it is over.
func (m *Month) shown() []Event {
	g := m.drag
	if g == nil || !g.moved || g.fixed {
		return m.events
	}
	out := slices.Clone(m.events)
	for i, e := range out {
		if e.ID == g.id {
			out[i].Start, out[i].End = m.moved(e, g)
		}
	}
	return out
}

// moved returns where e goes when g drags it from its day to the day under the pointer.
func (m *Month) moved(e Event, g *monthDrag) (time.Time, time.Time) {
	days := g.to - g.start
	s := AddDays(Day(e.Start), days).Add(dayOffset(e.Start))
	return s, s.Add(e.End.Sub(e.Start))
}

// Paint implements [gunim.Node].
func (m *Month) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	line := widget.MenuBorder.Get(th)
	faint := widget.PaletteHint.Get(th)
	ink := widget.Ink.Get(th)
	now := f.Now.In(m.Month.Location())
	regular, bold := widget.Font.Get(th), widget.BoldFont.Get(th)
	size := EventText.Get(th)
	for dd := range 7 {
		name := regular.Shape(m.day(dd).Format("Mon"), size)
		name.Paint(p, geom.Pt(float32(dd)*m.cellW()+8, (monthHeadH-name.Height())/2), faint)
	}
	for i := range 42 {
		c := m.cell(i)
		day := m.day(i)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			p.RRect(c, 0, paint.Solid(WeekendFill.Get(th)))
		}
		p.RRect(geom.Rc(c.Min.X, c.Min.Y, c.Size().W, 1), 0, paint.Solid(line))
		if i%7 > 0 {
			p.RRect(geom.Rc(c.Min.X, c.Min.Y, 1, c.Size().H), 0, paint.Solid(line))
		}
		label := strconv.Itoa(day.Day())
		if day.Day() == 1 {
			label = day.Format("2 Jan")
		}
		num := bold.Shape(label, size)
		numInk := ink
		if day.Month() != m.Month.Month() {
			numInk = faint
		}
		at := geom.Pt(c.Min.X+8, c.Min.Y+(dayNumH-num.Height())/2)
		if SameDay(day, now) {
			pill := geom.Rc(at.X-5, c.Min.Y+3, max(num.Advance+10, dayNumH-6), dayNumH-6)
			p.RRect(pill, (dayNumH-6)/2, paint.Solid(widget.Accent.Get(th)))
			numInk = widget.ButtonStrongInk.Get(th)
			at.X = pill.Center().X - num.Advance/2
		}
		num.Paint(p, at, numInk)
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1, Clip: true})()
	moving(p, box, m.slide.Value(), func() {
		for _, ch := range m.chips() {
			m.paintChip(p, th, ch)
		}
	})
	f.RedrawAt(AddDays(Day(now), 1))
}

// paintChip draws a chip: a bar filled with the event's colour, a line with a dot of it and the time, or how many
// more a day has.
func (m *Month) paintChip(p *paint.Painter, th *theme.Live, ch chip) {
	size := EventText.Get(th)
	r := ch.box
	ink := widget.Ink.Get(th)
	if ch.more > 0 {
		t := m.paragraph(th, strconv.Itoa(ch.more)+" more", r.Size().W-8, true, size)
		t.Paint(p, geom.Pt(r.Min.X+6, r.Center().Y-t.Size.H/2), widget.PaletteHint.Get(th))
		return
	}
	e := ch.e
	lifted := m.drag != nil && m.drag.moved && m.drag.id == e.ID
	if lifted || m.hover == e.ID {
		p.RRect(r, 4, paint.Solid(widget.MenuHot.Get(th)))
	}
	c := e.Color
	if e.Faint {
		ink = widget.PaletteHint.Get(th)
	}
	if ch.bar {
		a := uint8(0x55)
		if e.Faint {
			a = 0x22
		}
		p.RRect(r, 4, paint.Solid(color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}))
		t := m.paragraph(th, e.Title, r.Size().W-12, true, size)
		t.Paint(p, geom.Pt(r.Min.X+6, r.Center().Y-t.Size.H/2), ink)
		return
	}
	p.RRect(geom.Rc(r.Min.X+5, r.Center().Y-3.5, 7, 7), 3.5, paint.Solid(c))
	s := clock(e.Start.Hour(), e.Start.Minute()) + " " + e.Title
	t := m.paragraph(th, s, r.Size().W-20, false, size)
	t.Paint(p, geom.Pt(r.Min.X+17, r.Center().Y-t.Size.H/2), ink)
}

// paragraph lays s out on one line w wide, keeping it for the next frame.
func (m *Month) paragraph(th *theme.Live, s string, w float32, bold bool, size float32) text.Paragraph {
	k := textKey{s: s, w: w, lines: 1, bold: bold, fontSize: size}
	if t, ok := m.texts[k]; ok {
		return t
	}
	face := widget.Font.Get(th)
	if bold {
		face = widget.BoldFont.Get(th)
	}
	t := face.Layout(s, text.Style{Size: size, MaxLines: 1}, max(w, 1))
	if m.texts == nil {
		m.texts = map[textKey]text.Paragraph{}
	}
	m.texts[k] = t
	return t
}

// chipAt returns the chip under pt.
func (m *Month) chipAt(pt geom.Point) (chip, bool) {
	cs := m.chips()
	for k := len(cs) - 1; k >= 0; k-- {
		if cs[k].box.Contains(pt) {
			return cs[k], true
		}
	}
	return chip{}, false
}

// Handle implements [gunim.Handler].
func (m *Month) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if g := m.drag; g != nil {
			if !g.moved && math.Abs(float64(e.Pos.X-g.from.X))+math.Abs(float64(e.Pos.Y-g.from.Y)) < dragSlack {
				return true
			}
			g.moved = true
			if c := m.cellAt(e.Pos); c >= 0 {
				g.to = c
			}
			u.Invalidate()
			return true
		}
		id := ""
		if ch, ok := m.chipAt(e.Pos); ok && ch.more == 0 {
			id = ch.e.ID
		}
		if id != m.hover {
			m.hover = id
			u.Invalidate()
		}
	case input.PointerLeave:
		if m.hover != "" {
			m.hover = ""
			u.Invalidate()
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		m.press(e, u)
		return true
	case input.PointerUp:
		g := m.drag
		if g == nil {
			return false
		}
		m.drag = nil
		u.Invalidate()
		if !g.moved {
			if m.Open != nil {
				m.Open(g.id, g.box, u)
			}
			return true
		}
		if g.fixed || g.to == g.start || m.OnChange == nil {
			return true
		}
		for _, ev := range m.events {
			if ev.ID == g.id {
				s, en := m.moved(ev, g)
				u.Send(m, m.OnChange(g.id, s, en))
			}
		}
		return true
	}
	return false
}

// press takes hold of a chip, picks a day by its number or its count of more, or with a double click begins an
// event on a day.
func (m *Month) press(e input.PointerDown, u *gunim.UI) {
	if ch, ok := m.chipAt(e.Pos); ok {
		if ch.more > 0 {
			if m.OnDay != nil {
				u.Send(m, m.OnDay(m.day(ch.cell)))
			}
			return
		}
		c := m.cellAt(e.Pos)
		m.drag = &monthDrag{id: ch.e.ID, fixed: ch.e.Fixed, from: e.Pos, box: ch.box, start: c, to: c}
		return
	}
	c := m.cellAt(e.Pos)
	if c < 0 {
		return
	}
	switch {
	case e.Pos.Y < m.cell(c).Min.Y+dayNumH && m.OnDay != nil:
		u.Send(m, m.OnDay(m.day(c)))
	case e.Clicks == 2 && m.OnCreate != nil:
		u.Send(m, m.OnCreate(m.day(c)))
	}
}

// Cursor implements [gunim.CursorShaper].
func (m *Month) Cursor(pt geom.Point) input.Cursor {
	if m.drag != nil && m.drag.moved && !m.drag.fixed {
		return input.CursorMove
	}
	if _, ok := m.chipAt(pt); ok {
		return input.CursorHand
	}
	if c := m.cellAt(pt); c >= 0 && pt.Y < m.cell(c).Min.Y+dayNumH {
		return input.CursorHand
	}
	return input.CursorArrow
}
