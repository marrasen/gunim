package main

import (
	"image/color"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Theme tokens of the calendar's window.
var (
	SidebarFill = theme.Color("cal.sidebar", color.NRGBA{R: 0x1a, G: 0x1d, B: 0x24, A: 0xff})
	PaneFill    = theme.Color("cal.pane", color.NRGBA{R: 0x1d, G: 0x20, B: 0x28, A: 0xff})
	TitleSize   = theme.Length("cal.title.size", 20)
	sidebarW    = theme.Length("cal.sidebar.width", 276)
	CardTitle   = theme.Length("cal.card.title", 17)
	noGap       = theme.Length("cal.gap.none", 0)
)

func darkTheme() theme.Theme { return widget.Dark() }

func lightTheme() theme.Theme {
	return widget.Light().With(append([]theme.Entry{
		theme.Set(SidebarFill, color.NRGBA{R: 0xe9, G: 0xec, B: 0xf2, A: 0xff}),
		theme.Set(PaneFill, color.NRGBA{R: 0xfa, G: 0xfb, B: 0xfd, A: 0xff}),
	}, calendar.Light...)...)
}

// registerViews is the window half: the calendar, the editor, the delete dialog and the themes.
func registerViews(w *gunim.Window) {
	w.RegisterTheme(darkTheme())
	w.RegisterTheme(lightTheme())
	gunim.RegisterView(w, "cal", buildCal, (*calView).set)
	gunim.RegisterView(w, "editor", newEditor, nil)
	gunim.RegisterView(w, "delete", newDeleteDialog, nil)
}

// calView is the window: a toolbar, a sidebar with a small month and the calendars, and the days.
type calView struct {
	root *widget.Flex

	title   *widget.Label
	views   *widget.Segmented
	invites *widget.Label
	mini    *calendar.MiniMonth
	cals    *widget.List
	days    *calendar.Days
	month   *calendar.Month
	area    *switcher

	state Cal
	// card is the card about the event cardID, while it is open.
	card   *gunim.Popup
	cardID string
}

func buildCal(s Cal) *calView {
	v := &calView{state: s}
	today := widget.NewButton("Today")
	today.On = TodayAsked{}
	back := widget.NewIconButton(icon.ChevronLeft, "Back")
	back.On = Stepped{By: -1}
	next := widget.NewIconButton(icon.ChevronRight, "Next")
	next.On = Stepped{By: 1}
	v.title = widget.NewLabel(s.Title)
	v.title.Face, v.title.Size = widget.BoldFont, TitleSize
	v.views = widget.NewSegmented("Day", "Week", "Month")
	v.views.OnChange = func(i int) gunim.Intent { return ViewChosen{View: View(i)} }
	themeButton := widget.NewIconButton(icon.SunMoon, "Switch theme")
	themeButton.On = ThemeToggled{}
	spacer := widget.NewSpacer()
	bar := widget.Row(today, back, next, v.title, spacer, v.views, themeButton).Grow(spacer, 1)
	bar.Cross = widget.CrossCenter

	add := widget.NewButton("New event")
	add.Icon, add.Kind, add.On = icon.Plus, widget.ButtonPrimary, NewAsked{}
	v.mini = calendar.NewMiniMonth(s.Day)
	v.mini.OnPick = func(day time.Time) gunim.Intent { return DayPicked{Day: day} }
	caption := widget.NewLabel("Calendars")
	caption.Face, caption.Color = widget.BoldFont, widget.PaletteHint
	v.cals = widget.NewList()
	v.cals.OnClick = func(k widget.Key) gunim.Intent { return CalendarToggled{ID: string(k)} }
	v.invites = widget.NewLabel("")
	v.invites.Color = widget.PaletteHint
	side := widget.Column(add, v.mini, caption, v.cals, v.invites)
	side.Cross = widget.CrossStretch
	sidebar := &panel{child: widget.NewPad(side), fill: SidebarFill, width: sidebarW}

	v.days = calendar.NewDays(s.Day, 7)
	v.days.OnDay = func(day time.Time) gunim.Intent { return DayOpened{Day: day} }
	v.days.OnCreate = func(start, end time.Time, allDay bool) gunim.Intent {
		return EventDrawn{Start: start, End: end, AllDay: allDay}
	}
	v.days.OnChange = func(id string, start, end time.Time) gunim.Intent {
		return EventChanged{ID: id, Start: start, End: end}
	}
	v.days.Open = func(id string, box geom.Rect, u *gunim.UI) { v.openCard(v.days, id, box, u) }
	v.month = calendar.NewMonth(s.Day)
	v.month.OnDay = func(day time.Time) gunim.Intent { return DayOpened{Day: day} }
	v.month.OnCreate = func(day time.Time) gunim.Intent {
		return EventDrawn{Start: day, End: calendar.AddDays(day, 1), AllDay: true}
	}
	v.month.OnChange = func(id string, start, end time.Time) gunim.Intent {
		return EventChanged{ID: id, Start: start, End: end}
	}
	v.month.Open = func(id string, box geom.Rect, u *gunim.UI) { v.openCard(v.month, id, box, u) }
	v.area = newSwitcher(v.days, v.month)

	main := widget.Column(widget.NewPad(bar), v.area).Grow(v.area, 1)
	main.Cross = widget.CrossStretch
	pane := &panel{child: main, fill: PaneFill}
	v.root = widget.Row(sidebar, pane).Grow(pane, 1)
	v.root.Cross, v.root.Gap = widget.CrossStretch, noGap
	return v
}

// set shows s.
func (v *calView) set(s Cal, u *gunim.UI) {
	v.state = s
	if v.card != nil {
		if _, ok := v.event(v.cardID); !ok {
			v.closeCard()
		}
	}
	v.title.SetText(s.Title)
	v.views.SetSelected(int(s.View), u)
	from, to := shown(s)
	v.mini.SetMarked(from, calendar.AddDays(to, -1), u)
	widget.Sync(v.cals, u, s.Calendars,
		func(c Calendar) widget.Key { return widget.Key(c.ID) },
		newCalRow,
		func(r *calRow, c Calendar, u *gunim.UI) { r.set(c, u) })
	switch s.Invites {
	case 0:
		v.invites.SetText("")
	case 1:
		v.invites.SetText("1 invitation waits for an answer")
	default:
		v.invites.SetText(strconv.Itoa(s.Invites) + " invitations wait for an answer")
	}
	if s.View == MonthView {
		v.month.SetMonth(s.Day, u)
		v.month.SetEvents(s.Events, u)
		v.area.show(1, u)
	} else {
		v.days.SetDays(from, map[View]int{DayView: 1, WeekView: 7}[s.View], u)
		v.days.SetEvents(s.Events, u)
		v.area.show(0, u)
	}
	u.Invalidate()
}

// shown returns the first day the view shows, and the day after the last.
func shown(s Cal) (time.Time, time.Time) {
	switch s.View {
	case DayView:
		return s.Day, calendar.AddDays(s.Day, 1)
	case WeekView:
		f := calendar.WeekStart(s.Day, time.Monday)
		return f, calendar.AddDays(f, 7)
	}
	return calendar.MonthStart(s.Day), calendar.MonthStart(s.Day).AddDate(0, 1, 0)
}

// Children implements [gunim.Composite].
func (v *calView) Children() []gunim.Node { return []gunim.Node{v.root} }

// Layout implements [gunim.Node].
func (v *calView) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(c.Max))
	kid.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (v *calView) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// Handle implements [gunim.Handler]: the keys that move about in time, pick the view, and begin an event.
func (v *calView) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok || k.Mods&(input.ModControl|input.ModAlt) != 0 {
		return false
	}
	var intent gunim.Intent
	switch k.Key {
	case input.KeyT:
		intent = TodayAsked{}
	case input.KeyD:
		intent = ViewChosen{View: DayView}
	case input.KeyW:
		intent = ViewChosen{View: WeekView}
	case input.KeyM:
		intent = ViewChosen{View: MonthView}
	case input.KeyN:
		intent = NewAsked{}
	case input.KeyPageUp, input.KeyLeft:
		intent = Stepped{By: -1}
	case input.KeyPageDown, input.KeyRight:
		intent = Stepped{By: 1}
	default:
		return false
	}
	u.Send(v, intent)
	return true
}

// openCard shows the card about the event id beside its box in from.
func (v *calView) openCard(from gunim.Node, id string, box geom.Rect, u *gunim.UI) {
	if v.card != nil && v.card.Open() {
		v.card.Close()
	}
	ev, ok := v.event(id)
	if !ok {
		return
	}
	v.cardID = id
	c := newEventCard(ev, v.state.Details[id], func(u *gunim.UI) { v.closeCard() })
	v.card = u.OpenPopup(from, c, gunim.PopupOptions{Anchor: box, Max: geom.Sz(360, 600),
		Dismiss: func(*gunim.UI) { v.closeCard() }})
}

func (v *calView) closeCard() {
	if v.card != nil {
		v.card.Close()
		v.card = nil
	}
}

// event returns the event with id in the state shown.
func (v *calView) event(id string) (calendar.Event, bool) {
	for _, e := range v.state.Events {
		if e.ID == id {
			return e, true
		}
	}
	return calendar.Event{}, false
}

// calRow is one of the calendars in the sidebar: a box in its colour, ticked while it shows, and its name.
type calRow struct {
	anim.Group
	c    Calendar
	on   *anim.Float
	name text.Run
}

func newCalRow(c Calendar) *calRow {
	r := &calRow{c: c, on: anim.NewFloat(0)}
	r.Add(r.on)
	if c.Shown {
		r.on.Jump(1)
	}
	return r
}

func (r *calRow) set(c Calendar, u *gunim.UI) {
	r.c = c
	r.on.Animate(map[bool]float32{false: 0, true: 1}[c.Shown], widget.Quick.Get(u.Theme()))
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (r *calRow) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	r.name = widget.Font.Get(f.Theme).Shape(r.c.Name, widget.TextSize.Get(f.Theme))
	return geom.Sz(c.Max.W, 30)
}

// Paint implements [gunim.Node].
func (r *calRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	on := min(max(r.on.Value(), 0), 1)
	b := geom.Rc(8, box.H/2-8, 16, 16)
	c := r.c.Color
	p.RRectStroke(b, 4, paint.Solid(color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(255 * on)}),
		paint.Stroke{Width: 1.5, Color: c})
	if on > 0.01 {
		widget.PaintIcon(p, th, icon.Check, b.Inset(geom.Uniform(2)), color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: uint8(255 * on)})
	}
	ink := widget.Ink.Get(th)
	if on < 0.5 {
		ink = widget.PaletteHint.Get(th)
	}
	r.name.Paint(p, geom.Pt(34, (box.H-r.name.Height())/2), ink)
}

// Cursor implements [gunim.CursorShaper].
func (r *calRow) Cursor(geom.Point) input.Cursor { return input.CursorHand }

// panel fills its box and holds its child, at a width from a token when it has one.
type panel struct {
	child gunim.Node
	fill  theme.Token[color.NRGBA]
	width theme.Token[float32]
}

// Children implements [gunim.Composite].
func (p *panel) Children() []gunim.Node { return []gunim.Node{p.child} }

// Layout implements [gunim.Node].
func (p *panel) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	if p.width.Key() != "" {
		size.W = p.width.Get(f.Theme)
	}
	kid := kids.At(0)
	kid.Layout(gunim.Tight(size))
	kid.Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node].
func (p *panel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(p.fill.Get(f.Theme)))
	kids.At(0).Paint(pt)
}

// switcher shows one of its children, crossfading as the choice changes. The others stay built.
type switcher struct {
	anim.Group
	kids  []gunim.Node
	shown int
	vis   []*anim.Float
}

func newSwitcher(kids ...gunim.Node) *switcher {
	s := &switcher{kids: kids}
	for i := range kids {
		f := anim.NewFloat(0)
		if i == 0 {
			f.Jump(1)
		}
		s.vis = append(s.vis, f)
		s.Add(f)
	}
	return s
}

// show crossfades to child i.
func (s *switcher) show(i int, u *gunim.UI) {
	if i == s.shown {
		return
	}
	s.shown = i
	for k, f := range s.vis {
		f.Animate(map[bool]float32{false: 0, true: 1}[k == i], widget.Quick.Get(u.Theme()))
	}
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (s *switcher) Children() []gunim.Node { return s.kids }

// Layout implements [gunim.Node].
func (s *switcher) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for kid := range kids.All {
		kid.Layout(gunim.Tight(c.Max))
		kid.Place(geom.Point{})
	}
	return c.Max
}

// Paint implements [gunim.Node]: a child out of sight is not drawn, so it takes no clicks.
func (s *switcher) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	for i := range s.kids {
		o := min(max(s.vis[i].Value(), 0), 1)
		switch {
		case o <= 0.01:
		case o >= 0.99:
			kids.At(i).Paint(p)
		default:
			func() {
				defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: o})()
				kids.At(i).Paint(p)
			}()
		}
	}
}
