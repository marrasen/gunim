package calendar

import (
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// Sizes of a [MiniMonth]: its cells at most, its heading, and its week numbers' column.
const (
	miniCell  = 34
	miniHeadH = 36
	miniWeekW = 26
)

// MiniMonth is a small month for picking a day: the month's name with arrows to the months either side, the days
// in weeks with the weeks' numbers, today ringed, and the days From to To marked, such as the week a calendar shows.
// A click picks a day, and the arrow keys, Page Up and Page Down move the day picked while it has the keyboard.
type MiniMonth struct {
	anim.Group
	// From and To are the first and last days marked.
	From, To time.Time
	// FirstWeekday is the day weeks start on.
	FirstWeekday time.Weekday
	// OnPick turns a day picked into an intent. Pick, when set, runs as well, such as for a field to take the day.
	OnPick func(day time.Time) gunim.Intent
	Pick   func(day time.Time, u *gunim.UI)

	// month is the month showing, which the arrows move without picking anything.
	month time.Time
	hover int
	// slide eases the days in from the side the month came from.
	slide *anim.Float
	// side is a day's cell's side at the last layout.
	side float32
}

// NewMiniMonth returns a small month showing day's month, with day marked, and weeks starting on Monday.
func NewMiniMonth(day time.Time) *MiniMonth {
	m := &MiniMonth{From: Day(day), To: Day(day), FirstWeekday: time.Monday, month: MonthStart(day), hover: -1,
		slide: anim.NewFloat(0)}
	m.Add(m.slide)
	return m
}

// SetMarked marks the days from and to, and shows from's month when it is not showing already.
func (m *MiniMonth) SetMarked(from, to time.Time, u *gunim.UI) {
	m.From, m.To = Day(from), Day(to)
	if !SameDay(MonthStart(from), m.month) && !SameDay(MonthStart(to), m.month) {
		m.show(MonthStart(from), u)
	}
	u.Invalidate()
}

// show turns to the month starting at month, sliding the days in from the side it lies.
func (m *MiniMonth) show(month time.Time, u *gunim.UI) {
	if month.Before(m.month) {
		m.slide.Jump(-1)
	} else if month.After(m.month) {
		m.slide.Jump(1)
	}
	m.slide.Animate(0, widget.Quick.Get(u.Theme()))
	m.month = month
	u.Invalidate()
}

func (m *MiniMonth) first() time.Time { return WeekStart(m.month, m.FirstWeekday) }
func (m *MiniMonth) day(i int) time.Time {
	return AddDays(m.first(), i)
}

// cell returns day i's box.
func (m *MiniMonth) cell(i int) geom.Rect {
	s := m.side
	return geom.Rc(miniWeekW+float32(i%7)*s, miniHeadH+22+float32(i/7)*s, s, s)
}

// arrows returns the boxes of the arrows to the month before and after.
func (m *MiniMonth) arrows() (back, next geom.Rect) {
	right := miniWeekW + 7*m.side
	return geom.Rc(right-2*28, 4, 28, 28), geom.Rc(right-28, 4, 28, 28)
}

// Layout implements [gunim.Node]: cells as large as the width allows, up to miniCell.
func (m *MiniMonth) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	m.side = miniCell
	if c.Max.W > 0 {
		m.side = min(miniCell, float32(int((c.Max.W-miniWeekW)/7)))
	}
	return c.Constrain(geom.Sz(miniWeekW+7*m.side, miniHeadH+22+6*m.side))
}

// Paint implements [gunim.Node].
func (m *MiniMonth) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	ink, faint := widget.Ink.Get(th), widget.PaletteHint.Get(th)
	regular, bold := widget.Font.Get(th), widget.BoldFont.Get(th)
	now := f.Now.In(m.month.Location())
	title := bold.Shape(m.month.Format("January 2006"), widget.TextSize.Get(th))
	title.Paint(p, geom.Pt(8, 4+(28-title.Height())/2), ink)
	back, next := m.arrows()
	widget.PaintIcon(p, th, icon.ChevronLeft, back.Inset(geom.Uniform(6)), ink)
	widget.PaintIcon(p, th, icon.ChevronRight, next.Inset(geom.Uniform(6)), ink)
	size := EventText.Get(th)
	for dd := range 7 {
		n := regular.Shape(m.day(dd).Format("Mon")[:2], size)
		c := m.cell(dd)
		n.Paint(p, geom.Pt(c.Center().X-n.Advance/2, miniHeadH), faint)
	}
	grid := geom.Rc(0, miniHeadH+20, box.W, 6*m.side+4)
	defer p.Layer(paint.LayerOpts{Bounds: grid, Opacity: 1, Clip: true})()
	defer p.Push(paint.Translate(geom.Pt(m.slide.Value()*40, 0)))()
	for w := range 6 {
		_, wk := m.day(w * 7).ISOWeek()
		n := regular.Shape(strconv.Itoa(wk), 11)
		c := m.cell(w * 7)
		n.Paint(p, geom.Pt(miniWeekW-6-n.Advance, c.Center().Y-n.Height()/2), faint)
	}
	// The marked days join into one band along each week.
	marked := widget.MenuHot.Get(th)
	for w := range 6 {
		first, last := -1, -1
		for dd := range 7 {
			if day := m.day(w*7 + dd); !day.Before(m.From) && !day.After(m.To) {
				if first < 0 {
					first = dd
				}
				last = dd
			}
		}
		if first >= 0 {
			a, b := m.cell(w*7+first), m.cell(w*7+last)
			band := geom.Rect{Min: geom.Pt(a.Min.X+2, a.Min.Y+3), Max: geom.Pt(b.Max.X-2, b.Max.Y-3)}
			p.RRect(band, band.Size().H/2, paint.Solid(marked))
		}
	}
	for i := range 42 {
		day, c := m.day(i), m.cell(i)
		in := !day.Before(m.From) && !day.After(m.To)
		if i == m.hover && !in {
			p.RRect(c.Inset(geom.Uniform(3)), 13, paint.Solid(widget.WindowButtonHot.Get(th)))
		}
		dayInk := ink
		if day.Month() != m.month.Month() {
			dayInk = faint
		}
		n := regular.Shape(strconv.Itoa(day.Day()), size)
		if SameDay(day, now) {
			p.RRect(c.Inset(geom.Uniform(3)), 13, paint.Solid(widget.Accent.Get(th)))
			dayInk = widget.ButtonStrongInk.Get(th)
		}
		n.Paint(p, geom.Pt(c.Center().X-n.Advance/2, c.Center().Y-n.Height()/2), dayInk)
	}
}

// at returns the day under pt, or -1.
func (m *MiniMonth) at(pt geom.Point) int {
	for i := range 42 {
		if m.cell(i).Contains(pt) {
			return i
		}
	}
	return -1
}

// pick picks day, turning to its month.
func (m *MiniMonth) pick(day time.Time, u *gunim.UI) {
	if !SameDay(MonthStart(day), m.month) {
		m.show(MonthStart(day), u)
	}
	if m.Pick != nil {
		m.Pick(day, u)
	}
	if m.OnPick != nil {
		u.Send(m, m.OnPick(day))
	}
}

// Handle implements [gunim.Handler].
func (m *MiniMonth) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if i := m.at(e.Pos); i != m.hover {
			m.hover = i
			u.Invalidate()
		}
	case input.PointerLeave:
		m.hover = -1
		u.Invalidate()
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		back, next := m.arrows()
		switch {
		case back.Contains(e.Pos):
			m.show(m.month.AddDate(0, -1, 0), u)
		case next.Contains(e.Pos):
			m.show(m.month.AddDate(0, 1, 0), u)
		default:
			if i := m.at(e.Pos); i >= 0 {
				m.pick(m.day(i), u)
			}
		}
		return true
	case input.KeyPress:
		by := map[input.Key]int{input.KeyLeft: -1, input.KeyRight: 1, input.KeyUp: -7, input.KeyDown: 7}[e.Key]
		switch {
		case by != 0:
			m.pick(AddDays(m.From, by), u)
		case e.Key == input.KeyPageUp:
			m.pick(m.From.AddDate(0, -1, 0), u)
		case e.Key == input.KeyPageDown:
			m.pick(m.From.AddDate(0, 1, 0), u)
		case e.Key == input.KeyEnter:
			m.pick(m.From, u)
		default:
			return false
		}
		return true
	}
	return false
}

// Focusable implements [gunim.Focusable].
func (m *MiniMonth) Focusable() bool { return true }

// Cursor implements [gunim.CursorShaper].
func (m *MiniMonth) Cursor(pt geom.Point) input.Cursor {
	back, next := m.arrows()
	if m.at(pt) >= 0 || back.Contains(pt) || next.Contains(pt) {
		return input.CursorHand
	}
	return input.CursorArrow
}
