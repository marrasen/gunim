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
	longRowH  = 24
	dragSlack = 5
	edgeGrab  = 7
	minEventH = 20
	dayMargin = 8
)

// Days shows days side by side, by the hour: a column for each day, under a heading with its name and date, and a
// row above the hours for the events that last whole days. Events that overlap in time share their column.
//
// Everything moves: an event glides to where it goes, fades in as it arrives and out as it leaves, lifts under the
// pointer, and morphs into place as the grid goes from one day to a week. Stepping to other days slides them in.
//
// The pointer draws out a new event on free time, or on the row of whole days, moves an event by dragging it, and
// changes its length by dragging its bottom edge. Times snap to Snap. A drag that ends where it began is a click,
// which opens the event. The grid shows a change at once, and for up to a second while the application catches up.
type Days struct {
	anim.Group
	// First is the first day shown, and Count how many days: 1 for a day, 7 for a week.
	First time.Time
	Count int
	// Snap is how finely times snap under the pointer; zero is 15 minutes.
	Snap time.Duration
	// WorkDay is the working day, from its start to its end; the hours outside it are shaded. Zero shades none.
	WorkDay [2]time.Duration
	// OnDay turns a click on a day's heading into an intent, such as to show that day alone.
	OnDay func(day time.Time) gunim.Intent
	// OnCreate turns a span drawn out on free time into an intent, to make an event there.
	OnCreate func(start, end time.Time, allDay bool) gunim.Intent
	// Create, when set, runs in place of OnCreate with the span drawn out and its box in the grid's space, such as
	// to ask for a title beside it. The drawn event stays until [Days.ClearGhost].
	Create func(start, end time.Time, allDay bool, box geom.Rect, u *gunim.UI)
	// OnChange turns an event moved, or made longer or shorter, into an intent.
	OnChange func(id string, start, end time.Time) gunim.Intent
	// Open runs when the user clicks an event, with the event's box in the grid's space, such as to show a card
	// about it there.
	Open func(id string, box geom.Rect, u *gunim.UI)
	// OnEdit turns a double click on an event into an intent, such as to open it in an editor.
	OnEdit func(id string) gunim.Intent
	// Busy, when set, is asked before a press on free time begins an event. True lets the press do nothing more,
	// such as one that only closed a card about an event.
	Busy func() bool

	events []Event
	// held are events the user changed, shown where they left them until the application agrees.
	held map[string]heldEvent
	// sprites are the events as drawn, by key, and order the order they are drawn in.
	sprites map[string]*sprite
	order   []string
	// selected is the event marked as chosen, such as the one a card is open on.
	selected string

	scroll *anim.Float
	// slide carries the days in from the side they came from, as the grid steps to other days.
	slide *anim.Float
	// ghost is the event being drawn out, and ghostIn how much of it shows.
	ghost   *anim.Rect
	ghostIn *anim.Float
	laid    bool
	box     geom.Size
	// was is what the grid showed at the last layout, to tell a step from a change of view.
	was struct {
		box   geom.Size
		first time.Time
		count int
	}
	// hourH is how tall an hour was at the last layout.
	hourH float32
	// heads fades the headings in as the grid changes how many days it shows.
	heads *anim.Float
	// th is the theme at the last layout, and pointer where the pointer is while it drags, for the drag to go
	// on as the grid scrolls under it.
	th      *theme.Live
	pointer geom.Point
	// longRows is how many rows the whole-day events take.
	longRows int
	hover    string
	drag     *dayDrag
	// ghostHeld is the drag that drew out an event being named, whose ghost stays until ClearGhost.
	ghostHeld *dayDrag
	texts     map[textKey]text.Paragraph
}

// heldEvent is where the user left an event, and when.
type heldEvent struct {
	start, end time.Time
	at         time.Time
}

// sprite is an event as the grid draws it: its box, which glides to where the event goes, how far it has faded in
// or out, and how much it is lifted under the pointer. A timed event's box is in the hours' own space, with
// midnight at zero and no scroll, so scrolling does not set it moving.
type sprite struct {
	e           Event
	long        bool
	top, bottom bool
	rect        *anim.Rect
	in, lift    *anim.Float
	gone        bool
}

func (s *sprite) step(dt time.Duration) bool {
	a, b, c := s.rect.Step(dt), s.in.Step(dt), s.lift.Step(dt)
	return a || b || c
}

// dayDrag is a drag on the grid: drawing out a new event, or moving an event or changing its length.
type dayDrag struct {
	kind  dragKind
	id    string
	fixed bool
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
	dragResizeTop
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
		ghost: anim.NewRect(geom.Rect{}), ghostIn: anim.NewFloat(0), held: map[string]heldEvent{},
		sprites: map[string]*sprite{}, heads: anim.NewFloat(1)}
	d.Add(d.scroll, d.slide, d.ghost, d.ghostIn, d.heads)
	return d
}

// Step implements [gunim.Animator], stepping the events as drawn too.
func (d *Days) Step(dt time.Duration) bool {
	moving := d.Group.Step(dt)
	if d.edgeScroll(dt) {
		moving = true
	}
	for k, s := range d.sprites {
		if s.step(dt) {
			moving = true
		}
		if s.gone && !s.in.Active() {
			delete(d.sprites, k)
		}
	}
	return moving
}

// edgeScroll scrolls the grid while a drag holds the pointer near the top or bottom of the hours, faster the nearer
// it is, and carries the drag along. It reports whether it scrolled.
func (d *Days) edgeScroll(dt time.Duration) bool {
	if d.drag == nil || d.drag.allDay || !d.drag.moved {
		return false
	}
	const zone = 40
	top, bottom := d.bodyTop(), d.box.H
	speed := float32(0)
	switch y := d.pointer.Y; {
	case y < top+zone:
		speed = -(top + zone - y) / zone
	case y > bottom-zone:
		speed = (y - (bottom - zone)) / zone
	default:
		return false
	}
	to := d.clampScroll(d.scroll.Value() + min(max(speed, -1), 1)*900*float32(dt.Seconds()))
	if to == d.scroll.Value() {
		return false
	}
	d.scroll.Jump(to)
	d.dragTo(d.pointer, d.th)
	return true
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

// SetDays shows count days from first. Stepping to other days of the same count slides them in from their side;
// changing the count morphs each event into its new place.
func (d *Days) SetDays(first time.Time, count int, u *gunim.UI) {
	first, count = Day(first), max(count, 1)
	if d.laid && count == d.Count && !first.Equal(d.First) {
		d.slide.Jump(map[bool]float32{false: -1, true: 1}[first.After(d.First)])
		d.slide.Animate(0, widget.Settle.Get(u.Theme()))
	}
	if d.laid && count != d.Count {
		d.heads.Jump(0)
		d.heads.Animate(1, widget.Settle.Get(u.Theme()))
	}
	d.First, d.Count = first, count
	u.Invalidate()
}

// Select marks the event id as chosen, lifting it and ringing it in the accent, or marks none for an empty id.
func (d *Days) Select(id string, u *gunim.UI) {
	d.selected = id
	d.aimLifts(u.Theme())
	u.Invalidate()
}

// EventBox returns where the event id shows, in the grid's space, and false when it does not show.
func (d *Days) EventBox(id string) (geom.Rect, bool) {
	for _, k := range d.order {
		if s := d.sprites[k]; s != nil && !s.gone && s.e.ID == id {
			r := s.rect.Target()
			if !s.long {
				r = r.Add(geom.Pt(0, d.bodyTop()-d.scroll.Target()))
			}
			return r, true
		}
	}
	return geom.Rect{}, false
}

// Reveal scrolls the grid so the event id shows, with an hour above it, unless it shows already.
func (d *Days) Reveal(id string, u *gunim.UI) {
	for _, e := range d.shown() {
		if e.ID != id || e.long() {
			continue
		}
		y0 := d.hourY(dayOffset(e.Start))
		y1 := d.hourY(dayOffset(e.End))
		view := d.box.H - d.bodyTop()
		if y0 >= d.scroll.Target() && y1 <= d.scroll.Target()+view {
			return
		}
		d.scroll.Animate(d.clampScroll(y0-d.hour()), widget.Settle.Get(u.Theme()))
		return
	}
}

// moving draws what draw draws slid and faded as the days step.
func moving(p *paint.Painter, box geom.Size, slide float32, draw func()) {
	if abs(slide) < 0.001 {
		draw()
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1 - min(abs(slide), 1)*0.8})()
	defer p.Push(paint.Translate(geom.Pt(slide*72, 0)))()
	draw()
}

// ScrollToHour scrolls the grid so the hour h, such as 7.5 for half past seven, is at its top.
func (d *Days) ScrollToHour(h float32, u *gunim.UI) {
	d.scroll.Animate(d.clampScroll(h*d.hour()), widget.Quick.Get(u.Theme()))
}

func (d *Days) step() time.Duration {
	if d.Snap <= 0 {
		return 15 * time.Minute
	}
	return d.Snap
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

// hourY returns where a time offset into a day is in the hours' own space.
func (d *Days) hourY(offset time.Duration) float32 { return float32(offset.Hours()) * d.hour() }

// timeY returns where a time offset into a day is down the grid, scrolled.
func (d *Days) timeY(offset time.Duration) float32 {
	return d.bodyTop() + d.hourY(offset) - d.scroll.Value()
}

// onScreen turns a sprite's box into the grid's space.
func (d *Days) onScreen(s *sprite, r geom.Rect) geom.Rect {
	if s.long {
		return r
	}
	return r.Add(geom.Pt(0, d.bodyTop()-d.scroll.Value()))
}

func (d *Days) clampScroll(v float32) float32 {
	room := 24*d.hour() - (d.box.H - d.bodyTop())
	return min(max(v, 0), max(room, 0))
}

// Layout implements [gunim.Node]: it sends each event gliding to its place.
func (d *Days) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	d.box, d.hourH, d.th = c.Max, HourHeight.Get(f.Theme), f.Theme
	d.longRows = len(d.longPlaces())
	if !d.laid {
		d.scroll.Jump(d.clampScroll(7.5 * d.hour()))
	}
	stepped := d.laid && d.Count == d.was.count && !d.First.Equal(d.was.first)
	jump := !d.laid || d.box != d.was.box
	d.place(f.Theme, jump || stepped, stepped)
	d.laid = true
	d.was.box, d.was.first, d.was.count = d.box, d.First, d.Count
	return d.box
}

// target is where an event goes.
type target struct {
	key         string
	e           Event
	long        bool
	box         geom.Rect
	top, bottom bool
}

// place sends each event's sprite to where it goes: at once with jump, as when the window changes size, and
// gliding otherwise. Sprites of events no longer shown fade out, or go at once with drop, as the grid steps and
// slides the new days in.
func (d *Days) place(th *theme.Live, jump, drop bool) {
	seen := map[string]bool{}
	d.order = d.order[:0]
	for _, t := range d.targets() {
		seen[t.key] = true
		d.order = append(d.order, t.key)
		s, ok := d.sprites[t.key]
		switch {
		case !ok:
			s = &sprite{rect: anim.NewRect(t.box), in: anim.NewFloat(0), lift: anim.NewFloat(0)}
			if jump {
				s.in.Jump(1)
			} else {
				s.in.Animate(1, widget.Bounce.Get(th))
			}
			d.sprites[t.key] = s
		case s.gone:
			s.gone = false
			s.in.Animate(1, widget.Bounce.Get(th))
			s.rect.Jump(t.box)
		case jump:
			s.rect.Jump(t.box)
		case s.rect.Target() != t.box:
			motion := widget.Settle.Get(th)
			if d.drag != nil && d.drag.id == t.e.ID {
				// Under the pointer it follows closely, snapping from step to step.
				motion = widget.Quick.Get(th)
			}
			s.rect.Animate(t.box, motion)
		}
		s.e, s.long, s.top, s.bottom = t.e, t.long, t.top, t.bottom
	}
	for k, s := range d.sprites {
		if seen[k] {
			continue
		}
		if drop {
			delete(d.sprites, k)
			continue
		}
		if !s.gone {
			s.gone = true
			s.in.Animate(0, widget.Quick.Get(th))
		}
		d.order = append(d.order, k)
	}
	d.aimLifts(th)
}

// aimLifts lifts the event under the pointer and the one chosen, and lets the rest down.
func (d *Days) aimLifts(th *theme.Live) {
	for _, s := range d.sprites {
		up := !s.gone && (s.e.ID == d.hover || s.e.ID == d.selected || (d.drag != nil && d.drag.moved && s.e.ID == d.drag.id))
		s.lift.Animate(map[bool]float32{false: 0, true: 1}[up], widget.Quick.Get(th))
	}
}

// targets returns where every event shown goes: the whole-day ones on their rows, then each day's timed ones.
func (d *Days) targets() []target {
	var out []target
	for _, row := range d.longPlaces() {
		for _, lp := range row {
			out = append(out, target{key: lp.e.ID + "#long", e: lp.e, long: true, box: d.longBox(lp)})
		}
	}
	for i := range d.Count {
		key := "@" + d.day(i).Format(time.DateOnly)
		for _, b := range d.timedBoxes(i) {
			out = append(out, target{key: b.e.ID + key, e: b.e, box: b.box, top: b.top, bottom: b.bottom})
		}
	}
	return out
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

// shown returns the events with any the user changed where they left them, and the one being dragged where the
// pointer has it.
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

// timedBoxes lays out the events of day i by the hour, in the hours' own space, in the order to draw them.
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
	places, order := lanes(starts, ends)
	w := d.colW() - dayMargin
	// Each step of indent moves an event over by a fifth of the column, up to 28 pixels.
	step := min(w/5, 28)
	out := make([]timedBox, 0, len(evs))
	for _, k := range order {
		p := places[k]
		inW := max(w-float32(p.indent)*step, w/3)
		laneW := inW / float32(max(p.lanes, 1))
		x := d.colX(i) + (w - inW) + float32(p.lane)*laneW
		y0 := d.hourY(starts[k].Sub(day))
		y1 := max(d.hourY(ends[k].Sub(day)), y0+minEventH)
		out = append(out, timedBox{e: evs[k], box: geom.Rc(x+1, y0+1, float32(p.span)*laneW-2, y1-y0-2),
			top: evs[k].Start.Before(day), bottom: evs[k].End.After(next)})
	}
	return out
}

// longBox returns where a whole-day event sits on the row above the hours.
func (d *Days) longBox(p longPlace) geom.Rect {
	x0, x1 := d.colX(p.first)+2, d.colX(p.last+1)-2
	return geom.Rc(x0, d.longTop()+float32(p.row)*longRowH+2, x1-x0, longRowH-4)
}

// Paint implements [gunim.Node].
func (d *Days) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	line := widget.MenuBorder.Get(th)
	faint := widget.PaletteHint.Get(th)
	hourH := d.hour()
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
			off := OffHoursFill.Get(th)
			y0, y1 := d.timeY(w[0]), d.timeY(w[1])
			p.RRect(geom.Rect{Min: geom.Pt(gutterW, body.Min.Y), Max: geom.Pt(box.W, y0)}, 0, paint.Solid(off))
			p.RRect(geom.Rect{Min: geom.Pt(gutterW, y1), Max: geom.Pt(box.W, body.Max.Y)}, 0, paint.Solid(off))
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
			for _, k := range d.order {
				if s := d.sprites[k]; s != nil && !s.long {
					d.paintSprite(p, th, s)
				}
			}
			if !d.ghost.Target().Empty() && !d.ghostAllDay() {
				d.paintGhost(p, th, d.ghost.Value().Add(geom.Pt(0, d.bodyTop()-d.scroll.Value())))
			}
		})
		// Now, faint across the days shown and bold across today.
		for i := range d.Count {
			if !SameDay(d.day(i), now) {
				continue
			}
			faintNow := NowInk.Get(th)
			faintNow.A = 0x50
			p.RRect(geom.Rc(gutterW, d.timeY(dayOffset(now))-0.5, box.W-gutterW, 1), 0, paint.Solid(faintNow))
		}
		for i := range d.Count {
			if !SameDay(d.day(i), now) {
				continue
			}
			y := d.timeY(dayOffset(now))
			c := NowInk.Get(th)
			p.RRect(geom.Rc(d.colX(i), y-1, d.colW(), 2), 1, paint.Solid(c))
			p.RRect(geom.Rc(d.colX(i)-5, y-5, 10, 10), 5, paint.Solid(c))
			f.RedrawAt(now.Truncate(time.Minute).Add(time.Minute))
		}
	}()

	// The headings and the row of whole days, over the hours.
	for i := 1; i < d.Count; i++ {
		p.RRect(geom.Rc(d.colX(i), headerH-10, 1, d.bodyTop()-headerH+10), 0, paint.Solid(line))
	}
	p.RRect(geom.Rc(0, d.bodyTop()-1, box.W, 1), 0, paint.Solid(line))
	moving(p, box, d.slide.Value(), func() {
		if h := min(max(d.heads.Value(), 0), 1); h < 0.99 {
			func() {
				defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, 0, box.W, headerH), Opacity: h})()
				d.paintHeads(p, th, now)
			}()
		} else {
			d.paintHeads(p, th, now)
		}
		for _, k := range d.order {
			if s := d.sprites[k]; s != nil && s.long {
				d.paintSprite(p, th, s)
			}
		}
		if !d.ghost.Target().Empty() && d.ghostAllDay() {
			d.paintGhost(p, th, d.ghost.Value())
		}
	})
}

// ghostAllDay reports whether the event being drawn out, or held while it is named, lasts whole days.
func (d *Days) ghostAllDay() bool {
	g := d.drag
	if g == nil || g.kind != dragCreate {
		g = d.ghostHeld
	}
	return g != nil && g.allDay
}

// ghostOnScreen returns the drawn event's box in the grid's space.
func (d *Days) ghostOnScreen() geom.Rect {
	if d.ghostAllDay() {
		return d.ghost.Target()
	}
	return d.ghost.Target().Add(geom.Pt(0, d.bodyTop()-d.scroll.Target()))
}

// ClearGhost lets the event drawn out for [Days.Create] fade, once it is named or given up.
func (d *Days) ClearGhost(u *gunim.UI) {
	d.ghostHeld = nil
	d.ghostIn.Animate(0, widget.Settle.Get(u.Theme()))
	u.Invalidate()
}

// paintHeads draws the days' headings.
func (d *Days) paintHeads(p *paint.Painter, th *theme.Live, now time.Time) {
	ink, faint := widget.Ink.Get(th), widget.PaletteHint.Get(th)
	bold, regular := widget.BoldFont.Get(th), widget.Font.Get(th)
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
			r := geom.Rc(c-17, 22, 34, 28)
			p.RRect(r, 14, paint.Solid(widget.Accent.Get(th)))
			numInk = widget.ButtonStrongInk.Get(th)
		}
		num.Paint(p, geom.Pt(c-num.Advance/2, 36-num.Height()/2), numInk)
	}
}

// eventColors returns an event's fill, the bar down its side, and its text's ink, lit by lift from 0 to 1.
func eventColors(th *theme.Live, e Event, lift float32) (fill, bar, ink, sub color.NRGBA) {
	c := e.Color
	a := 0.42 + 0.18*lift
	if e.Faint {
		a = 0.10 + 0.12*lift
	}
	fill = color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(255 * a)}
	bar = c
	ink = widget.Ink.Get(th)
	sub = ink
	sub.A = 0xc8
	return fill, bar, ink, sub
}

// paintSprite draws an event as it moves: faded and a little shrunk while it comes in or goes out, and lifted with
// a shadow under the pointer, while dragged, or chosen.
func (d *Days) paintSprite(p *paint.Painter, th *theme.Live, s *sprite) {
	in := min(max(s.in.Value(), 0), 1)
	if in <= 0.01 {
		return
	}
	lift := min(max(s.lift.Value(), 0), 1)
	r := d.onScreen(s, s.rect.Value())
	shrink := (1 - in) * 6
	r = r.Inset(geom.Uniform(min(shrink, r.Size().H/3)))
	if in < 0.99 {
		defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(-12)), Opacity: in})()
	}
	e := s.e
	fill, bar, ink, sub := eventColors(th, e, lift)
	shadow := paint.Shadow{}
	if lift > 0.01 {
		shadow = paint.Shadow{Color: color.NRGBA{A: uint8(0x60 * lift)}, Blur: 12 * lift, Offset: geom.Pt(0, 3*lift)}
	}
	// A solid base under the tint, edged in the base's colour, keeps an event on top of another readable.
	base := EventBase.Get(th)
	p.ShadowRRect(r.Inset(geom.Uniform(-1)), 7, paint.Solid(base), shadow)
	p.RRect(r, 6, paint.Solid(fill))
	if e.Faint {
		stripes(p, r, 6, bar)
	}
	if hover := e.ID == d.hover && d.drag == nil; hover && s.resizable() && r.Size().H >= 44 {
		// A grip at the bottom says the event's length can be pulled.
		g := bar
		g.A = uint8(float32(g.A) * lift)
		p.RRect(geom.Rc(r.Center().X-10, r.Max.Y-4, 20, 2.5), 1.25, paint.Solid(g))
	}
	if e.ID == d.selected {
		p.RRectStroke(r.Inset(geom.Uniform(-1.5)), 7.5, paint.Fill{}, paint.Stroke{Width: 2, Color: widget.Accent.Get(th)})
	}
	defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 6})()
	p.RRect(geom.Rc(r.Min.X, r.Min.Y, 4, r.Size().H), 0, paint.Solid(bar))
	size := EventText.Get(th)
	w := r.Size().W - 12
	if s.long {
		t := d.paragraph(th, e.Title, w, 1, true, size)
		t.Paint(p, geom.Pt(r.Min.X+9, r.Center().Y-t.Size.H/2), ink)
		return
	}
	times := clock(e.Start.Hour(), e.Start.Minute()) + "–" + clock(e.End.Hour(), e.End.Minute())
	line := d.paragraph(th, "Ag", w, 1, false, size).Size.H
	if r.Size().H < 2*line+4 {
		// Title and times on one line.
		t := d.paragraph(th, e.Title, w, 1, true, size)
		t.Paint(p, geom.Pt(r.Min.X+9, r.Min.Y+max((r.Size().H-t.Size.H)/2, 1)), ink)
		if rest := w - t.Size.W - 8; rest > 40 {
			tt := d.paragraph(th, times, rest, 1, false, size)
			tt.Paint(p, geom.Pt(r.Min.X+9+t.Size.W+8, r.Min.Y+max((r.Size().H-tt.Size.H)/2, 1)), sub)
		}
		return
	}
	lines := max(int((r.Size().H-6)/line)-1, 1)
	if w < 90 {
		// Too narrow to wrap words: one line, cut short.
		lines = 1
	}
	t := d.paragraph(th, e.Title, w, lines, true, size)
	t.Paint(p, geom.Pt(r.Min.X+9, r.Min.Y+4), ink)
	subText := times
	if e.Location != "" {
		subText += " · " + e.Location
	}
	st := d.paragraph(th, subText, w, 1, false, size)
	st.Paint(p, geom.Pt(r.Min.X+9, r.Min.Y+4+t.Size.H), sub)
}

// stripes shades r, rounded by radius, with thin diagonal stripes of c, for an event waiting for an answer.
func stripes(p *paint.Painter, r geom.Rect, radius float32, c color.NRGBA) {
	defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: radius})()
	c.A = 0x55
	diag := r.Size().W + r.Size().H
	defer p.Push(paint.Rotate(-math.Pi/4, r.Center()))()
	for x := r.Center().X - diag/2; x < r.Center().X+diag/2; x += 9 {
		p.RRect(geom.Rc(x, r.Center().Y-diag/2, 3, diag), 0, paint.Solid(c))
	}
}

// paintGhost draws the event being drawn out, in r.
func (d *Days) paintGhost(p *paint.Painter, th *theme.Live, r geom.Rect) {
	in := min(max(d.ghostIn.Value(), 0), 1)
	if in <= 0.01 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(-8)), Opacity: in})()
	a := widget.Accent.Get(th)
	p.ShadowRRect(r, 6, paint.Solid(color.NRGBA{R: a.R, G: a.G, B: a.B, A: 0x70}),
		paint.Shadow{Color: color.NRGBA{A: 0x50}, Blur: 10, Offset: geom.Pt(0, 3)})
	p.RRectStroke(r, 6, paint.Fill{}, paint.Stroke{Width: 1.5, Color: a})
	g := d.drag
	if g == nil || g.kind != dragCreate {
		g = d.ghostHeld
	}
	if g == nil || g.allDay {
		return
	}
	t := clock(g.start.Hour(), g.start.Minute()) + "–" + clock(g.end.Hour(), g.end.Minute())
	s := d.paragraph(th, t, r.Size().W-12, 1, true, EventText.Get(th))
	s.Paint(p, geom.Pt(r.Min.X+9, r.Min.Y+4), widget.Ink.Get(th))
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

// resizable reports whether the event's length can be pulled by its edges: a timed one, tall enough to hold a click
// apart from its edges, that the user may change.
func (s *sprite) resizable() bool {
	return !s.long && !s.e.Fixed && s.rect.Target().Size().H >= 30
}

// edge is the edge of an event the pointer is on.
type edge uint8

const (
	noEdge edge = iota
	topEdge
	bottomEdge
)

// eventAt returns the event under pt, its box on screen, and the edge pt is on.
func (d *Days) eventAt(pt geom.Point) (Event, geom.Rect, edge, bool) {
	inHours := pt.Y >= d.bodyTop()
	for k := len(d.order) - 1; k >= 0; k-- {
		s := d.sprites[d.order[k]]
		if s == nil || s.gone || s.long == inHours {
			continue
		}
		r := d.onScreen(s, s.rect.Target())
		if !r.Contains(pt) {
			continue
		}
		on := noEdge
		switch {
		case !s.resizable():
		case pt.Y > r.Max.Y-edgeGrab && !s.bottom:
			on = bottomEdge
		case pt.Y < r.Min.Y+edgeGrab && !s.top:
			on = topEdge
		}
		return s.e, r, on, true
	}
	return Event{}, geom.Rect{}, noEdge, false
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
			d.pointer = e.Pos
			d.dragTo(e.Pos, th)
			u.Invalidate()
			return true
		}
		d.pointer = e.Pos
		d.hoverAt(e.Pos, th)
	case input.PointerLeave:
		if d.drag == nil && d.hover != "" {
			d.hover = ""
			d.aimLifts(th)
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		d.pointer = e.Pos
		d.press(e, th, u)
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
			d.ghostIn.Animate(0, widget.Quick.Get(th))
			d.aimLifts(th)
			u.Invalidate()
			return true
		}
	}
	return false
}

// hoverAt lifts the event under pt.
func (d *Days) hoverAt(pt geom.Point, th *theme.Live) {
	id := ""
	if ev, _, _, ok := d.eventAt(pt); ok {
		id = ev.ID
	}
	if id != d.hover {
		d.hover = id
		d.aimLifts(th)
	}
}

// press starts what a press at pt starts: a day's heading picks the day, an event is taken hold of, and free time
// begins a new event.
func (d *Days) press(e input.PointerDown, th *theme.Live, u *gunim.UI) {
	pt := e.Pos
	if pt.X < gutterW {
		return
	}
	if pt.Y < headerH {
		if d.OnDay != nil {
			u.Send(d, d.OnDay(d.day(d.dayAt(pt.X))))
		}
		return
	}
	if ev, b, on, ok := d.eventAt(pt); ok {
		if e.Clicks == 2 && d.OnEdit != nil && !ev.Fixed {
			u.Send(d, d.OnEdit(ev.ID))
			return
		}
		kind := dragMove
		switch on {
		case bottomEdge:
			kind = dragResize
		case topEdge:
			kind = dragResizeTop
		}
		d.drag = &dayDrag{kind: kind, id: ev.ID, fixed: ev.Fixed, from: pt, start: ev.Start, end: ev.End, box: b,
			allDay: ev.long(), grab: d.timeAt(d.dayAt(pt.X), pt.Y).Sub(ev.Start)}
		return
	}
	if d.Busy != nil && d.Busy() {
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
	d.ghost.Jump(d.ghostBoxOf(d.drag))
	d.ghostIn.Animate(1, widget.Quick.Get(th))
	u.Invalidate()
}

// dragTo follows the pointer to pt.
func (d *Days) dragTo(pt geom.Point, th *theme.Live) {
	g := d.drag
	if !g.moved && math.Hypot(float64(pt.X-g.from.X), float64(pt.Y-g.from.Y)) < dragSlack {
		return
	}
	if !g.moved {
		g.moved = true
		d.aimLifts(th)
	}
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
		d.ghost.Animate(d.ghostBoxOf(d.drag), widget.Quick.Get(th))
	case dragMove:
		if g.fixed {
			return
		}
		ev, ok := d.event(g.id)
		if !ok {
			return
		}
		length := ev.End.Sub(ev.Start)
		if ev.long() {
			days := i - d.dayAt(g.from.X)
			g.start = AddDays(Day(ev.Start), days)
			if !ev.AllDay {
				g.start = g.start.Add(dayOffset(ev.Start))
			}
			g.end = g.start.Add(length)
			return
		}
		t := d.timeAt(i, pt.Y).Add(-g.grab.Truncate(d.step()))
		g.start, g.end = t, t.Add(length)
	case dragResize:
		if g.fixed {
			return
		}
		ev, ok := d.event(g.id)
		if !ok {
			return
		}
		t := d.timeAt(d.dayAt(g.from.X), pt.Y).Add(d.step())
		g.start, g.end = ev.Start, maxTime(t, ev.Start.Add(d.step()))
	case dragResizeTop:
		if g.fixed {
			return
		}
		ev, ok := d.event(g.id)
		if !ok {
			return
		}
		t := d.timeAt(d.dayAt(g.from.X), pt.Y)
		g.start, g.end = minTime(t, ev.End.Add(-d.step())), ev.End
	}
}

// ghostBoxOf returns where the event being drawn out goes: in the grid's space for one of whole days, and in the
// hours' own space otherwise.
func (d *Days) ghostBoxOf(g *dayDrag) geom.Rect {
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
		return d.longBox(longPlace{row: 0, first: max(first, 0), last: last})
	}
	i := d.dayAt(g.from.X)
	day := d.day(i)
	y0, y1 := d.hourY(g.start.Sub(day)), d.hourY(g.end.Sub(day))
	return geom.Rc(d.colX(i)+1, y0+1, d.colW()-dayMargin-2, y1-y0-2)
}

// release ends a drag: a new event, a moved or longer one, or a click on an event, which a drag that ends where
// it began counts as.
func (d *Days) release(u *gunim.UI) {
	g := d.drag
	d.drag = nil
	th := u.Theme()
	d.aimLifts(th)
	u.Invalidate()
	if g.kind == dragCreate {
		if !g.moved && !g.allDay {
			// A click, not a drag, draws out an hour.
			g.end = minTime(g.start.Add(time.Hour), AddDays(Day(g.start), 1))
			d.ghost.Animate(d.ghostBoxOf(g), widget.Bounce.Get(th))
		}
		if d.Create != nil {
			// The drawn event stays while it is being named.
			d.ghostHeld = g
			d.Create(g.start, g.end, g.allDay, d.ghostOnScreen(), u)
			return
		}
		d.ghostIn.Animate(0, widget.Settle.Get(th))
		if d.OnCreate != nil {
			u.Send(d, d.OnCreate(g.start, g.end, g.allDay))
		}
		return
	}
	ev, ok := d.event(g.id)
	if !ok {
		return
	}
	if !g.moved || g.fixed || (ev.Start.Equal(g.start) && ev.End.Equal(g.end)) {
		if d.Open != nil {
			if b, ok := d.EventBox(g.id); ok {
				g.box = b
			}
			d.Open(g.id, g.box, u)
		}
		return
	}
	d.held[g.id] = heldEvent{start: g.start, end: g.end, at: time.Now()}
	if d.OnChange != nil {
		u.Send(d, d.OnChange(g.id, g.start, g.end))
	}
}

// event returns the event with id, as the application last gave it, or as the user left it.
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
		case dragResize, dragResizeTop:
			return input.CursorResizeV
		case dragMove:
			if d.drag.moved && !d.drag.fixed {
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
	if _, _, on, ok := d.eventAt(pt); ok {
		if on != noEdge {
			return input.CursorResizeV
		}
		return input.CursorHand
	}
	return input.CursorCrosshair
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
