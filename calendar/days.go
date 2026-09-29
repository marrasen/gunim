package calendar

import (
	"image/color"
	"math"
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

// Sizes of a [Days] grid.
const (
	gutterW   = 56
	headerH   = 52
	longRowH  = 22
	dragSlack = 4
	edgeGrab  = 6
	minEventH = 18
	dayMargin = 8
)

// Days shows days side by side, by the hour: a column for each day, under a heading with its name and date, and a
// row above the hours for the events that last whole days. Events that overlap in time share their column.
//
// The pointer draws out a new event on free time, or on the row of whole days, moves an event by dragging it, and
// changes its length by dragging its bottom edge. Times snap to Step. The grid shows a change at once, and for up to
// a second while the application's events catch up.
type Days struct {
	anim.Group
	// First is the first day shown, and Count how many days: 1 for a day, 7 for a week.
	First time.Time
	Count int
	// Step is how finely times snap under the pointer; zero is 15 minutes.
	Step time.Duration
	// WorkDay is the working day, from its start to its end; the hours outside it are shaded. Zero shades none.
	WorkDay [2]time.Duration
	// OnDay turns a click on a day's heading into an intent, such as to show that day alone.
	OnDay func(day time.Time) gunim.Intent
	// OnCreate turns a span drawn out on free time into an intent, to make an event there.
	OnCreate func(start, end time.Time, allDay bool) gunim.Intent
	// OnChange turns an event moved, or made longer or shorter, into an intent.
	OnChange func(id string, start, end time.Time) gunim.Intent
	// Open runs when the user clicks an event, with the event's box in the grid's space, such as to show a card
	// about it there.
	Open func(id string, box geom.Rect, u *gunim.UI)

	events []Event
	// held are events the user changed, shown where they left them until the application agrees.
	held map[string]heldEvent

	scroll *anim.Float
	// slide carries the days in from the side they came from, as the grid steps to other days.
	slide *anim.Float
	laid  bool
	box   geom.Size
	// hourH is how tall an hour was at the last layout.
	hourH float32
	// longRows is how many rows the whole-day events take.
	longRows int
	hover    string
	drag     *dayDrag
	texts    map[textKey]text.Paragraph
}

// heldEvent is where the user left an event, and when.
type heldEvent struct {
	start, end time.Time
	at         time.Time
}

// dayDrag is a drag on the grid: drawing out a new event, or moving an event or changing its length.
type dayDrag struct {
	kind  dragKind
	id    string
	from  geom.Point
	moved bool
	// grab is how far into the event the pointer took hold of it; anchor is where a new event was begun.
	grab       time.Duration
	anchor     time.Time
	start, end time.Time
	allDay     bool
	box        geom.Rect
}

type dragKind uint8

const (
	dragCreate dragKind = iota
	dragMove
	dragResize
)

// textKey names a paragraph laid out for an event.
type textKey struct {
	s        string
	w        float32
	lines    int
	bold     bool
	fontSize float32
}

// NewDays returns a grid of count days from first.
func NewDays(first time.Time, count int) *Days {
	d := &Days{First: Day(first), Count: count, scroll: anim.NewFloat(0), slide: anim.NewFloat(0),
		held: map[string]heldEvent{}}
	d.Add(d.scroll, d.slide)
	return d
}

// SetEvents shows events, which may hold events outside the days shown.
func (d *Days) SetEvents(events []Event, u *gunim.UI) {
	d.events = events
	for id, h := range d.held {
		if time.Since(h.at) > time.Second {
			delete(d.held, id)
			continue
		}
		for _, e := range events {
			if e.ID == id && e.Start.Equal(h.start) && e.End.Equal(h.end) {
				delete(d.held, id)
			}
		}
	}
	d.texts = nil
	u.Invalidate()
}

// SetDays shows count days from first. Stepping to other days of the same count slides them in from their side.
func (d *Days) SetDays(first time.Time, count int, u *gunim.UI) {
	first, count = Day(first), max(count, 1)
	if d.laid && count == d.Count && !first.Equal(d.First) {
		d.slide.Jump(map[bool]float32{false: -1, true: 1}[first.After(d.First)])
		d.slide.Animate(0, widget.Quick.Get(u.Theme()))
	}
	d.First, d.Count = first, count
	u.Invalidate()
}

// EventBox returns where the event id shows, in the grid's space, and false when it does not show.
func (d *Days) EventBox(id string) (geom.Rect, bool) {
	for _, row := range d.longPlaces() {
		for _, lp := range row {
			if lp.e.ID == id {
				return d.longBox(lp), true
			}
		}
	}
	for i := range d.Count {
		for _, b := range d.timedBoxes(i) {
			if b.e.ID == id {
				return b.box, true
			}
		}
	}
	return geom.Rect{}, false
}

// Reveal scrolls the grid at once so the event id shows, with an hour above it.
func (d *Days) Reveal(id string) {
	for _, e := range d.shown() {
		if e.ID == id && !e.long() {
			d.scroll.Jump(d.clampScroll((float32(dayOffset(e.Start).Hours()) - 1) * d.hour()))
			return
		}
	}
}

// moving draws what paint draws slid and faded as the days step.
func moving(p *paint.Painter, box geom.Size, slide float32, draw func()) {
	if slide == 0 {
		draw()
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1 - min(abs(slide), 1)*0.7})()
	defer p.Push(paint.Translate(geom.Pt(slide*48, 0)))()
	draw()
}

// ScrollToHour scrolls the grid so the hour h, such as 7.5 for half past seven, is at its top.
func (d *Days) ScrollToHour(h float32, u *gunim.UI) {
	d.scroll.Animate(d.clampScroll(h*d.hour()), widget.Quick.Get(u.Theme()))
}

func (d *Days) step() time.Duration {
	if d.Step <= 0 {
		return 15 * time.Minute
	}
	return d.Step
}

// day returns the midnight starting day i of those shown.
func (d *Days) day(i int) time.Time { return AddDays(d.First, i) }

// geometry, all in the grid's space.
func (d *Days) colW() float32 { return (d.box.W - gutterW) / float32(max(d.Count, 1)) }
func (d *Days) colX(i int) float32 {
	return gutterW + float32(i)*d.colW()
}
func (d *Days) longTop() float32 { return headerH }
func (d *Days) bodyTop() float32 { return headerH + float32(max(d.longRows, 1))*longRowH + 6 }

// hour returns how tall an hour is.
func (d *Days) hour() float32 {
	if d.hourH <= 0 {
		return 48
	}
	return d.hourH
}

// timeY returns where a time offset into a day is down the grid, scrolled.
func (d *Days) timeY(offset time.Duration) float32 {
	return d.bodyTop() + float32(offset.Hours())*d.hour() - d.scroll.Value()
}

func (d *Days) clampScroll(v float32) float32 {
	room := 24*d.hour() - (d.box.H - d.bodyTop())
	return min(max(v, 0), max(room, 0))
}

// Layout implements [gunim.Node].
func (d *Days) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	d.box, d.hourH = c.Max, HourHeight.Get(f.Theme)
	d.longRows = len(d.longPlaces())
	if !d.laid {
		d.laid = true
		d.scroll.Jump(d.clampScroll(7.5 * d.hour()))
	}
	return d.box
}

// longPlace is a whole-day event on the row above the hours: its row, and the days it covers, from first to last.
type longPlace struct {
	e           Event
	row         int
	first, last int
}

// longPlaces sets the events that last whole days in rows, each in the first row free across its days. It returns
// the rows.
func (d *Days) longPlaces() [][]longPlace {
	var rows [][]longPlace
	for _, e := range d.shown() {
		if !e.long() {
			continue
		}
		first, last := -1, -1
		for i := range d.Count {
			if e.covers(d.day(i)) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue
		}
		row := 0
		for ; row < len(rows); row++ {
			free := true
			for _, p := range rows[row] {
				if p.first <= last && p.last >= first {
					free = false
					break
				}
			}
			if free {
				break
			}
		}
		if row == len(rows) {
			rows = append(rows, nil)
		}
		rows[row] = append(rows[row], longPlace{e: e, row: row, first: first, last: last})
	}
	return rows
}

// shown returns the events with any the user changed where they left them.
func (d *Days) shown() []Event {
	if len(d.held) == 0 && (d.drag == nil || d.drag.kind == dragCreate) {
		return d.events
	}
	out := make([]Event, 0, len(d.events))
	for _, e := range d.events {
		if h, ok := d.held[e.ID]; ok && time.Since(h.at) <= time.Second {
			e.Start, e.End = h.start, h.end
		}
		if d.drag != nil && d.drag.kind != dragCreate && d.drag.moved && d.drag.id == e.ID {
			e.Start, e.End = d.drag.start, d.drag.end
		}
		out = append(out, e)
	}
	return out
}

// timedBox is an event laid out in a day's column.
type timedBox struct {
	e   Event
	box geom.Rect
	// clipped says the event goes on before the day starts, or after it ends.
	top, bottom bool
}

// timedBoxes lays out the events of day i by the hour.
func (d *Days) timedBoxes(i int) []timedBox {
	day, next := d.day(i), d.day(i+1)
	var evs []Event
	var starts, ends []time.Time
	for _, e := range d.shown() {
		if e.long() || !e.covers(day) {
			continue
		}
		s, en := e.Start, e.End
		if s.Before(day) {
			s = day
		}
		if en.After(next) {
			en = next
		}
		evs, starts, ends = append(evs, e), append(starts, s), append(ends, en)
	}
	places := lanes(starts, ends)
	w := d.colW() - dayMargin
	out := make([]timedBox, len(evs))
	for k, p := range places {
		laneW := w / float32(max(p.lanes, 1))
		x := d.colX(i) + float32(p.lane)*laneW
		y0 := d.timeY(starts[k].Sub(day))
		y1 := max(d.timeY(ends[k].Sub(day)), y0+minEventH)
		out[k] = timedBox{e: evs[k], box: geom.Rc(x+1, y0+1, float32(p.span)*laneW-2, y1-y0-2),
			top: evs[k].Start.Before(day), bottom: evs[k].End.After(next)}
	}
	return out
}

// longBox returns where a whole-day event sits on the row above the hours.
func (d *Days) longBox(p longPlace) geom.Rect {
	x0, x1 := d.colX(p.first)+2, d.colX(p.last+1)-2
	return geom.Rc(x0, d.longTop()+float32(p.row)*longRowH+2, x1-x0, longRowH-3)
}

// Paint implements [gunim.Node].
func (d *Days) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	line := widget.MenuBorder.Get(th)
	faint := widget.PaletteHint.Get(th)
	hourH := HourHeight.Get(th)
	now := f.Now.In(d.First.Location())
	body := geom.Rect{Min: geom.Pt(0, d.bodyTop()), Max: box.Point()}

	// The hours, under everything else, clipped to the body.
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: body, Opacity: 1, Clip: true})()
		for i := range d.Count {
			day := d.day(i)
			col := geom.Rc(d.colX(i), body.Min.Y, d.colW(), body.Size().H)
			switch {
			case SameDay(day, now):
				p.RRect(col, 0, paint.Solid(TodayFill.Get(th)))
			case day.Weekday() == time.Saturday || day.Weekday() == time.Sunday:
				p.RRect(col, 0, paint.Solid(WeekendFill.Get(th)))
			}
		}
		if w := d.WorkDay; w[1] > w[0] {
			for i := range d.Count {
				off := OffHoursFill.Get(th)
				y0, y1 := d.timeY(w[0]), d.timeY(w[1])
				p.RRect(geom.Rect{Min: geom.Pt(d.colX(i), body.Min.Y), Max: geom.Pt(d.colX(i+1), y0)}, 0, paint.Solid(off))
				p.RRect(geom.Rect{Min: geom.Pt(d.colX(i), y1), Max: geom.Pt(d.colX(i+1), body.Max.Y)}, 0, paint.Solid(off))
			}
		}
		small := widget.Font.Get(th)
		for h := 0; h <= 24; h++ {
			y := d.timeY(time.Duration(h) * time.Hour)
			if y < body.Min.Y-hourH || y > box.H+hourH {
				continue
			}
			p.RRect(geom.Rc(gutterW, float32(math.Round(float64(y))), box.W-gutterW, 1), 0, paint.Solid(line))
			if h > 0 && h < 24 {
				lbl := small.Shape(clock(h, 0), EventText.Get(th))
				lbl.Paint(p, geom.Pt(gutterW-8-lbl.Advance, y-lbl.Height()/2), faint)
			}
		}
		for i := 1; i < d.Count; i++ {
			p.RRect(geom.Rc(d.colX(i), body.Min.Y, 1, body.Size().H), 0, paint.Solid(line))
		}
		moving(p, box, d.slide.Value(), func() {
			for i := range d.Count {
				for _, b := range d.timedBoxes(i) {
					d.paintEvent(p, th, b.e, b.box, false)
				}
			}
		})
		if d.drag != nil && d.drag.kind == dragCreate && !d.drag.allDay {
			d.paintGhost(p, th)
		}
		// Now, across today.
		for i := range d.Count {
			if !SameDay(d.day(i), now) {
				continue
			}
			y := d.timeY(dayOffset(now))
			c := NowInk.Get(th)
			p.RRect(geom.Rc(d.colX(i), y-1, d.colW(), 2), 1, paint.Solid(c))
			p.RRect(geom.Rc(d.colX(i)-4, y-4, 8, 8), 4, paint.Solid(c))
			f.RedrawAt(now.Truncate(time.Minute).Add(time.Minute))
		}
	}()

	// The headings and the row of whole days, over the hours.
	bold, regular := widget.BoldFont.Get(th), widget.Font.Get(th)
	for i := 1; i < d.Count; i++ {
		p.RRect(geom.Rc(d.colX(i), headerH-10, 1, d.bodyTop()-headerH+10), 0, paint.Solid(line))
	}
	p.RRect(geom.Rc(0, d.bodyTop()-1, box.W, 1), 0, paint.Solid(line))
	moving(p, box, d.slide.Value(), func() { d.paintHeads(p, th, now, bold, regular) })
}

// paintHeads draws the days' headings and the events that last whole days.
func (d *Days) paintHeads(p *paint.Painter, th *theme.Live, now time.Time, bold, regular *text.Face) {
	ink, faint := widget.Ink.Get(th), widget.PaletteHint.Get(th)
	for i := range d.Count {
		day := d.day(i)
		x := d.colX(i)
		name := regular.Shape(day.Format("Mon"), EventText.Get(th))
		num := bold.Shape(strconv.Itoa(day.Day()), 20)
		c := x + d.colW()/2
		if d.Count == 1 {
			c = x + 28
		}
		name.Paint(p, geom.Pt(c-name.Advance/2, 6), faint)
		numInk := ink
		if SameDay(day, now) {
			r := geom.Rc(c-16, 22, 32, 28)
			p.RRect(r, 14, paint.Solid(widget.Accent.Get(th)))
			numInk = widget.ButtonStrongInk.Get(th)
		}
		num.Paint(p, geom.Pt(c-num.Advance/2, 36-num.Height()/2), numInk)
	}
	for _, row := range d.longPlaces() {
		for _, lp := range row {
			d.paintEvent(p, th, lp.e, d.longBox(lp), true)
		}
	}
	if d.drag != nil && d.drag.kind == dragCreate && d.drag.allDay {
		d.paintGhost(p, th)
	}
}

// paintEvent draws e in r: a tint of its colour with a bar of it down the left, its title, and its times when
// there is room.
func (d *Days) paintEvent(p *paint.Painter, th *theme.Live, e Event, r geom.Rect, long bool) {
	c := e.Color
	fillA := uint8(0x46)
	if e.Faint {
		fillA = 0x1c
	}
	lifted := d.drag != nil && d.drag.moved && d.drag.id == e.ID
	if lifted {
		fillA = 0x80
	}
	fill := color.NRGBA{R: c.R, G: c.G, B: c.B, A: fillA}
	shadow := paint.Shadow{}
	if lifted {
		shadow = paint.Shadow{Color: color.NRGBA{A: 0x50}, Blur: 10, Offset: geom.Pt(0, 3)}
	}
	p.ShadowRRect(r, 5, paint.Solid(fill), shadow)
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 5})()
		p.RRect(geom.Rc(r.Min.X, r.Min.Y, 3, r.Size().H), 0, paint.Solid(c))
		ink := widget.Ink.Get(th)
		if e.Faint {
			ink = widget.PaletteHint.Get(th)
		}
		size := EventText.Get(th)
		w := r.Size().W - 10
		if long {
			t := d.paragraph(th, e.Title, w, 1, true, size)
			t.Paint(p, geom.Pt(r.Min.X+7, r.Center().Y-t.Size.H/2), ink)
			return
		}
		times := clock(e.Start.Hour(), e.Start.Minute()) + "–" + clock(e.End.Hour(), e.End.Minute())
		line := d.paragraph(th, "Ag", w, 1, false, size).Size.H
		switch {
		case r.Size().H < 2*line+4:
			// Title and times on one line.
			t := d.paragraph(th, e.Title+"  "+times, w, 1, true, size)
			t.Paint(p, geom.Pt(r.Min.X+7, r.Min.Y+max((r.Size().H-t.Size.H)/2, 1)), ink)
		default:
			lines := max(int((r.Size().H-6)/line)-1, 1)
			if w < 90 {
				// Too narrow to wrap words: one line, cut short.
				lines = 1
			}
			t := d.paragraph(th, e.Title, w, lines, true, size)
			t.Paint(p, geom.Pt(r.Min.X+7, r.Min.Y+3), ink)
			sub := times
			if e.Location != "" {
				sub += " · " + e.Location
			}
			s := d.paragraph(th, sub, w, 1, false, size)
			s.Paint(p, geom.Pt(r.Min.X+7, r.Min.Y+3+t.Size.H), widget.PaletteHint.Get(th))
		}
	}()
}

// paintGhost draws the event being drawn out.
func (d *Days) paintGhost(p *paint.Painter, th *theme.Live) {
	r := d.drag.box
	a := widget.Accent.Get(th)
	p.RRectStroke(r, 5, paint.Solid(color.NRGBA{R: a.R, G: a.G, B: a.B, A: 0x40}), paint.Stroke{Width: 1.5, Color: a})
	if d.drag.allDay {
		return
	}
	t := clock(d.drag.start.Hour(), d.drag.start.Minute()) + "–" + clock(d.drag.end.Hour(), d.drag.end.Minute())
	s := d.paragraph(th, t, r.Size().W-10, 1, true, EventText.Get(th))
	s.Paint(p, geom.Pt(r.Min.X+7, r.Min.Y+3), widget.Ink.Get(th))
}

// paragraph lays s out, bold or not, w wide in up to lines lines, keeping it for the next frame.
func (d *Days) paragraph(th *theme.Live, s string, w float32, lines int, bold bool, size float32) text.Paragraph {
	k := textKey{s: s, w: w, lines: lines, bold: bold, fontSize: size}
	if t, ok := d.texts[k]; ok {
		return t
	}
	face := widget.Font.Get(th)
	if bold {
		face = widget.BoldFont.Get(th)
	}
	t := face.Layout(s, text.Style{Size: size, MaxLines: lines}, max(w, 1))
	if d.texts == nil {
		d.texts = map[textKey]text.Paragraph{}
	}
	d.texts[k] = t
	return t
}

// clock writes a time of day as 09:30.
func clock(h, m int) string {
	return strconv.Itoa(h/10) + strconv.Itoa(h%10) + ":" + strconv.Itoa(m/10) + strconv.Itoa(m%10)
}

// dayAt returns the day under x, clamped to those shown.
func (d *Days) dayAt(x float32) int {
	return min(max(int((x-gutterW)/d.colW()), 0), d.Count-1)
}

// timeAt returns the time under y on day i, snapped down to the step.
func (d *Days) timeAt(i int, y float32) time.Time {
	hours := (y - d.bodyTop() + d.scroll.Value()) / d.hour()
	off := time.Duration(float64(hours) * float64(time.Hour))
	off = min(max(off, 0), 24*time.Hour)
	return d.day(i).Add(off.Truncate(d.step()))
}

// eventAt returns the event under pt, its box, and whether pt is on its bottom edge.
func (d *Days) eventAt(pt geom.Point) (Event, geom.Rect, bool, bool) {
	if pt.Y < d.bodyTop() {
		for _, row := range d.longPlaces() {
			for _, lp := range row {
				if b := d.longBox(lp); b.Contains(pt) {
					return lp.e, b, false, true
				}
			}
		}
		return Event{}, geom.Rect{}, false, false
	}
	i := d.dayAt(pt.X)
	boxes := d.timedBoxes(i)
	// The last drawn is on top.
	for k := len(boxes) - 1; k >= 0; k-- {
		b := boxes[k]
		if b.box.Contains(pt) {
			return b.e, b.box, pt.Y > b.box.Max.Y-edgeGrab && !b.bottom, true
		}
	}
	return Event{}, geom.Rect{}, false, false
}

// Handle implements [gunim.Handler].
func (d *Days) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.Scroll:
		if e.Pos.Y < d.bodyTop() {
			return false
		}
		d.scroll.Animate(d.clampScroll(d.scroll.Target()-e.Delta.Y), widget.Quick.Get(th))
		return true
	case input.PointerMove:
		if d.drag != nil {
			d.dragTo(e.Pos)
			u.Invalidate()
			return true
		}
		ev, _, _, ok := d.eventAt(e.Pos)
		id := ""
		if ok {
			id = ev.ID
		}
		if id != d.hover {
			d.hover = id
			u.Invalidate()
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		d.press(e.Pos, u)
		return true
	case input.PointerUp:
		if d.drag == nil {
			return false
		}
		d.release(u)
		return true
	case input.KeyPress:
		if e.Key == input.KeyEscape && d.drag != nil {
			d.drag = nil
			u.Invalidate()
			return true
		}
	}
	return false
}

// press starts what a press at pt starts: a day's heading picks the day, an event is taken hold of, and free time
// begins a new event.
func (d *Days) press(pt geom.Point, u *gunim.UI) {
	if pt.X < gutterW {
		return
	}
	if pt.Y < headerH {
		if d.OnDay != nil {
			u.Send(d, d.OnDay(d.day(d.dayAt(pt.X))))
		}
		return
	}
	if ev, b, edge, ok := d.eventAt(pt); ok {
		kind := dragMove
		if edge {
			kind = dragResize
		}
		d.drag = &dayDrag{kind: kind, id: ev.ID, from: pt, start: ev.Start, end: ev.End, box: b, allDay: ev.long(),
			grab: d.timeAt(d.dayAt(pt.X), pt.Y).Sub(ev.Start)}
		if ev.Fixed {
			// Only a click, to open it.
			d.drag.kind = dragMove
			d.drag.id = "fixed:" + ev.ID
		}
		return
	}
	i := d.dayAt(pt.X)
	if pt.Y < d.bodyTop() {
		day := d.day(i)
		d.drag = &dayDrag{kind: dragCreate, from: pt, anchor: day, start: day, end: AddDays(day, 1), allDay: true}
	} else {
		t := d.timeAt(i, pt.Y)
		d.drag = &dayDrag{kind: dragCreate, from: pt, anchor: t, start: t, end: t.Add(d.step())}
	}
	d.drag.box = d.ghostBox()
	u.Invalidate()
}

// dragTo follows the pointer to pt.
func (d *Days) dragTo(pt geom.Point) {
	g := d.drag
	if !g.moved && math.Abs(float64(pt.X-g.from.X))+math.Abs(float64(pt.Y-g.from.Y)) < dragSlack {
		return
	}
	g.moved = true
	i := d.dayAt(pt.X)
	switch g.kind {
	case dragCreate:
		if g.allDay {
			day := d.day(i)
			g.start, g.end = minTime(g.anchor, day), AddDays(maxTime(g.anchor, day), 1)
		} else {
			// Drawn out on the day it began, whichever way the pointer goes.
			t := d.timeAt(d.dayAt(g.from.X), pt.Y)
			if t.Before(g.anchor) {
				g.start, g.end = t, g.anchor.Add(d.step())
			} else {
				g.start, g.end = g.anchor, t.Add(d.step())
			}
			g.end = minTime(g.end, AddDays(Day(g.anchor), 1))
		}
		g.box = d.ghostBox()
	case dragMove:
		if len(g.id) > 6 && g.id[:6] == "fixed:" {
			return
		}
		ev, ok := d.event(g.id)
		if !ok {
			return
		}
		length := ev.End.Sub(ev.Start)
		if ev.long() {
			days := i - d.dayAt(g.from.X)
			g.start, g.end = AddDays(Day(ev.Start), days), AddDays(Day(ev.Start), days).Add(length)
			if !ev.AllDay {
				g.start = g.start.Add(dayOffset(ev.Start))
				g.end = g.start.Add(length)
			}
			return
		}
		t := d.timeAt(i, pt.Y).Add(-g.grab.Truncate(d.step()))
		g.start, g.end = t, t.Add(length)
	case dragResize:
		ev, ok := d.event(g.id)
		if !ok {
			return
		}
		t := d.timeAt(d.dayAt(g.from.X), pt.Y).Add(d.step())
		g.start, g.end = ev.Start, maxTime(t, ev.Start.Add(d.step()))
	}
}

// ghostBox returns where the event being drawn out shows.
func (d *Days) ghostBox() geom.Rect {
	g := d.drag
	if g.allDay {
		first, last := -1, 0
		for i := range d.Count {
			if (Event{Start: g.start, End: g.end}).covers(d.day(i)) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		first = max(first, 0)
		lp := longPlace{row: 0, first: first, last: last}
		return d.longBox(lp)
	}
	i := d.dayAt(g.from.X)
	day := d.day(i)
	y0, y1 := d.timeY(g.start.Sub(day)), d.timeY(g.end.Sub(day))
	return geom.Rc(d.colX(i)+1, y0+1, d.colW()-dayMargin-2, y1-y0-2)
}

// release ends a drag: a new event, a moved or longer one, or a click on an event.
func (d *Days) release(u *gunim.UI) {
	g := d.drag
	d.drag = nil
	u.Invalidate()
	switch {
	case g.kind == dragCreate:
		if d.OnCreate != nil {
			u.Send(d, d.OnCreate(g.start, g.end, g.allDay))
		}
	case !g.moved:
		id := g.id
		if len(id) > 6 && id[:6] == "fixed:" {
			id = id[6:]
		}
		if d.Open != nil {
			d.Open(id, g.box, u)
		}
	default:
		ev, ok := d.event(g.id)
		if !ok || (ev.Start.Equal(g.start) && ev.End.Equal(g.end)) {
			return
		}
		d.held[g.id] = heldEvent{start: g.start, end: g.end, at: time.Now()}
		if d.OnChange != nil {
			u.Send(d, d.OnChange(g.id, g.start, g.end))
		}
	}
}

// event returns the event with id, as the application last gave it.
func (d *Days) event(id string) (Event, bool) {
	for _, e := range d.events {
		if e.ID == id {
			if h, ok := d.held[id]; ok {
				e.Start, e.End = h.start, h.end
			}
			return e, true
		}
	}
	return Event{}, false
}

// Cursor implements [gunim.CursorShaper].
func (d *Days) Cursor(pt geom.Point) input.Cursor {
	if d.drag != nil {
		switch d.drag.kind {
		case dragResize:
			return input.CursorResizeV
		case dragMove:
			if d.drag.moved {
				return input.CursorMove
			}
		}
		return input.CursorArrow
	}
	if pt.X < gutterW {
		return input.CursorArrow
	}
	if pt.Y < headerH {
		return input.CursorHand
	}
	if ev, _, edge, ok := d.eventAt(pt); ok {
		switch {
		case ev.Fixed:
			return input.CursorHand
		case edge:
			return input.CursorResizeV
		}
		return input.CursorHand
	}
	return input.CursorArrow
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
