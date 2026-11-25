package widget

import (
	"slices"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// BottomBar tokens.
var (
	// BarHeight is the bar's least height, which grows under large text, and BarPadding the room above and below a
	// place's pill and label.
	BarHeight  = theme.Length("bar.height", 64)
	BarPadding = theme.Length("bar.padding", 8)
	// BarPillWidth and BarPillHeight size the pill behind the current place's icon, and BarLabelGap is the room
	// between the pill and the label.
	BarPillWidth  = theme.Length("bar.pill.width", 56)
	BarPillHeight = theme.Length("bar.pill.height", 32)
	BarLabelGap   = theme.Length("bar.label.gap", 4)
	// BarIconSize is a place's icon, and BarLabelSize its label's text.
	BarIconSize  = theme.Length("bar.icon", 24)
	BarLabelSize = theme.FontSize("bar.label.size", 12)
	// BarRailWidth is the rail's width, and BarRailFrom the least width at which a bar stands as a rail.
	BarRailWidth = theme.Length("bar.rail.width", 80)
	BarRailFrom  = theme.Length("bar.rail.from", 600)
	// BarDot is the size of the dot a place shows for news with no number.
	BarDot = theme.Length("bar.dot", 8)
)

// BarItem is one place of a [BottomBar].
type BarItem struct {
	// Key names the place across SetItems, so the current place and the focus ring stay on it as places move.
	Key   Key
	Label string
	Icon  *icon.Icon
	// Count shows in a badge on the icon's corner, as unread messages, up to "99+". Zero shows none. CountLabel is
	// what a screen reader says for it, such as "4 unread", and the number alone when empty.
	Count      int
	CountLabel string
	// Dot shows a dot on the icon's corner, for news with no number, where Count is zero.
	Dot bool
	// Page is what the bar shows while the place is current. A place given to SetItems with no Page keeps the page
	// of the place with its Key.
	Page gunim.Node
}

// BottomBar shows one page at a time with a bar of places to move between them, 3 to 5 for a phone held upright.
// A place is an icon over a label. A pill glides behind the current place's icon, and a count or a dot shows on an
// icon's corner. In a window at least BarRailFrom wide the bar stands as a rail down the left edge, and as the window
// narrows or widens it slides out and the other form slides in. The bar keeps clear of the system's navigation bar
// and its ground runs on under it.
//
// Each place holds its page. A tap or a click on a place goes there, and the new page fades in as the old one fades
// out. The pages keep their state while hidden, and move, come and go with their places. With the keyboard on the bar, the arrow keys move the focus ring between places, Home and End
// go to the ends, and Enter or Space goes to the place with the ring.
type BottomBar struct {
	anim.Group
	// Label names the bar for a screen reader, such as "Places".
	Label string
	// OnSelect runs on the UI goroutine when the user goes to a place; a non-nil result is sent to the application as
	// the bar's intent.
	OnSelect func(i int, u *gunim.UI) gunim.Intent

	strip *barStrip
	items []BarItem
	// selected is the current place, prev the page fading out, or nil, and fade runs from 0 to 1 as the new page
	// comes in.
	selected int
	prev     gunim.Node
	fade     *anim.Float
	// rail runs from 0, a bar along the bottom, to 1, a rail down the left edge.
	rail *anim.Float
	laid bool
	// box is the bar's size from the last layout.
	box geom.Size
}

// NewBottomBar returns a bar of items, the first current. Pages, where given, are the places' pages in order, for
// places with no Page of their own.
func NewBottomBar(items []BarItem, pages ...gunim.Node) *BottomBar {
	b := &BottomBar{fade: anim.NewFloat(1), rail: anim.NewFloat(0)}
	b.strip = newBarStrip(b)
	b.Add(b.fade, b.rail)
	items = slices.Clone(items)
	for i := range items {
		if items[i].Page == nil && i < len(pages) {
			items[i].Page = pages[i]
		}
	}
	b.SetItems(items, nil)
	return b
}

// Children implements [gunim.Composite]: the places' pages, then the bar over them.
func (b *BottomBar) Children() []gunim.Node { return append(b.pages(), b.strip) }

// pages returns the places' pages, each once.
func (b *BottomBar) pages() []gunim.Node {
	var out []gunim.Node
	for _, it := range b.items {
		if it.Page != nil && !slices.Contains(out, it.Page) {
			out = append(out, it.Page)
		}
	}
	return out
}

// Items returns a copy of the places.
func (b *BottomBar) Items() []BarItem { return slices.Clone(b.items) }

// SetItems makes items the places, and sends no intent. The current place and the focus ring stay on their places,
// found by Key; where their place goes they keep its spot, kept inside the bar. A place with no Page keeps the page of
// the place with its Key. A page whose place goes leaves the tree, and the page of a new place joins it. A count that
// comes or goes grows or shrinks its badge.
func (b *BottomBar) SetItems(items []BarItem, u *gunim.UI) {
	s := b.strip
	items = slices.Clone(items)
	from := oldPlaces(b.items, items)
	for i, was := range from {
		if items[i].Page == nil && was >= 0 {
			items[i].Page = b.items[was].Page
		}
	}
	shown := b.page(b.selected)
	keep := func(i int) int {
		if at := slices.Index(from, i); at >= 0 {
			return at
		}
		return min(max(i, 0), max(len(items)-1, 0))
	}
	b.selected, s.cursor = keep(b.selected), keep(s.cursor)
	b.items = items
	if now := b.page(b.selected); now != shown && b.laid && u != nil {
		// The current place's page changed under it: the new one fades in.
		b.prev = shown
		b.fade.Jump(0)
		b.fade.Animate(1, Settle.Get(u.Theme()))
	}
	s.placesChanged(from, u)
	u.Invalidate()
}

// oldPlaces returns, for each of items, the place it was among old: the place with its Key, or for a place with no
// Key the place in its spot, or -1 for none.
func oldPlaces(old, items []BarItem) []int {
	byKey := map[Key]int{}
	for i, it := range old {
		if _, seen := byKey[it.Key]; it.Key != "" && !seen {
			byKey[it.Key] = i
		}
	}
	from := make([]int, len(items))
	for i, it := range items {
		from[i] = -1
		switch was, ok := byKey[it.Key]; {
		case it.Key != "" && ok:
			from[i] = was
			delete(byKey, it.Key)
		case it.Key == "" && i < len(old) && old[i].Key == "":
			from[i] = i
		}
	}
	return from
}

// Selected returns the current place, and 0 for a bar with none.
func (b *BottomBar) Selected() int { return b.selected }

// SetSelected goes to place i and sends no intent. Once laid out, the pill glides there and the page fades in;
// before that, or with a nil u, it shows at once.
func (b *BottomBar) SetSelected(i int, u *gunim.UI) {
	if i < 0 || i >= len(b.items) || i == b.selected {
		return
	}
	s := b.strip
	if s.cursor == b.selected {
		s.cursor = i
	}
	if !b.laid || u == nil {
		b.selected, b.prev = i, nil
		b.fade.Jump(1)
		s.jump()
		return
	}
	old := b.page(b.selected)
	s.glide(b.selected, i, u.Theme())
	b.prev, b.selected = old, i
	b.fade.Jump(0)
	b.fade.Animate(1, Settle.Get(u.Theme()))
	// The keyboard leaves a page as it hides, for the bar.
	if old != nil && u.HasFocus(old) {
		u.Focus(s)
	}
	u.Invalidate()
}

// Cursor returns the place the focus ring is on.
func (b *BottomBar) Cursor() int { return b.strip.cursor }

// SetCursor puts the focus ring on place i, and sends no intent. Once laid out it glides there; before, or with a
// nil u, it jumps.
func (b *BottomBar) SetCursor(i int, u *gunim.UI) {
	if i < 0 || i >= len(b.items) {
		return
	}
	b.strip.moveCursor(i, u)
}

// Rail reports whether the bar stands as a rail down the left edge, or is on its way there.
func (b *BottomBar) Rail() bool { return b.rail.Target() > 0.5 }

// choose goes to place i for the user.
func (b *BottomBar) choose(i int, u *gunim.UI) {
	if i < 0 || i >= len(b.items) {
		return
	}
	b.strip.moveCursor(i, u)
	if i == b.selected {
		return
	}
	b.SetSelected(i, u)
	act(u, b, gunim.CueSelect, b.OnSelect, i)
}

// page returns place i's page, or nil.
func (b *BottomBar) page(i int) gunim.Node {
	if i < 0 || i >= len(b.items) {
		return nil
	}
	return b.items[i].Page
}

// shown is how far the bar along the bottom shows, and how far the rail does, each from 0 to 1: the bar slides out
// before the rail slides in.
func (b *BottomBar) shown() (bar, rail float32) {
	t := min(max(b.rail.Value(), 0), 1)
	return 1 - min(2*t, 1), max(2*t-1, 0)
}

// Layout implements [gunim.Node]: the page in what the bar leaves, and the bar along the bottom or down the left.
func (b *BottomBar) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	size := fillOr(c, geom.Sz(390, 700))
	b.box = size
	wantRail := anim.OnOff(size.W >= BarRailFrom.Get(th))
	if !b.laid {
		b.laid = true
		b.rail.Jump(wantRail)
		b.strip.jump()
	} else if b.rail.Target() != wantRail {
		b.rail.Animate(wantRail, Settle.Get(th))
	}
	s := b.strip
	strip, byNode := b.sort(kids, f.UI())
	barT, railT := b.shown()
	if len(b.items) == 0 {
		barT, railT = 0, 0
	}
	safe, _ := s.place.under(f)
	barH := s.barHeight(th)
	railW := min(BarRailWidth.Get(th), size.W)
	if b.rail.Value() < 0.5 {
		s.rail = false
		full := barH + safe
		strip.Layout(gunim.Tight(geom.Sz(size.W, full)))
		strip.Place(geom.Pt(0, size.H-full*barT))
	} else {
		s.rail = true
		strip.Layout(gunim.Tight(geom.Sz(railW, size.H)))
		strip.Place(geom.Pt(-railW*(1-railT), 0))
	}
	left := railW * railT
	bottom := (barH + safe) * barT
	room := gunim.Tight(geom.Sz(max(size.W-left, 0), max(size.H-bottom, 0)))
	for _, n := range []gunim.Node{b.prev, b.page(b.selected)} {
		if kid, ok := byNode[n]; ok && n != nil {
			kid.Layout(room)
			kid.Place(geom.Pt(left, 0))
		}
	}
	return size
}

// sort finds the strip and the pages among kids, adds the pages of new places to the tree, and drops the pages
// whose places have gone.
func (b *BottomBar) sort(kids gunim.Children, u *gunim.UI) (strip gunim.Child, byNode map[gunim.Node]gunim.Child) {
	byNode = map[gunim.Node]gunim.Child{}
	pages := b.pages()
	for kid := range kids.All {
		n := kid.Node()
		switch {
		case n == gunim.Node(b.strip):
			strip = kid
		case slices.Contains(pages, n), n == b.prev:
			byNode[n] = kid
		case u.Presence(n) != gunim.Exiting:
			kids.Drop(n)
		}
	}
	for _, n := range pages {
		if _, ok := byNode[n]; !ok {
			byNode[n] = kids.Build(n)
		}
	}
	return strip, byNode
}

// Paint implements [gunim.Node]: the page, the old one fading out under the new one coming in, and the bar over
// them.
func (b *BottomBar) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	t := min(max(b.fade.Value(), 0), 1)
	cur := b.page(b.selected)
	var prev, now, strip gunim.Child
	var hasPrev, hasNow bool
	for kid := range kids.All {
		switch n := kid.Node(); {
		case n == gunim.Node(b.strip):
			strip = kid
		case cur != nil && n == cur:
			now, hasNow = kid, true
		case b.prev != nil && n == b.prev:
			prev, hasPrev = kid, true
		}
	}
	if t >= 1 {
		b.prev, hasPrev = nil, false
	}
	whole := geom.Rect{Max: box.Point()}
	if hasPrev {
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: whole, Opacity: 1 - t})()
			prev.Paint(p)
		}()
	}
	if hasNow {
		func() {
			if t < 1 {
				defer p.Layer(paint.LayerOpts{Bounds: whole, Opacity: t})()
			}
			now.Paint(p)
		}()
	}
	if len(b.items) > 0 {
		strip.Paint(p)
	}
}

// barStrip is a [BottomBar]'s bar, or rail: the places, which take the pointer and the keyboard.
type barStrip struct {
	Control
	b *BottomBar
	// rail says it stands as a rail, from the last layout.
	rail bool
	// cursor is the place the focus ring is on, and ring where it is drawn, gliding between places, as a place's
	// number. pill is where the pill is, as a place's number, gliding to the current place.
	cursor     int
	ring, pill *anim.Float
	// hot is the place under the pointer, or -1, lit by hotV; was is the place left, fading out with wasV.
	hot, was   int
	hotV, wasV *anim.Float
	// badges grow each place's badge in and out, by Key or by spot.
	badges map[int]*anim.Float
	click  Clicker
	tip    PartTip
	shaped []barText
	line   shapedText
	size   geom.Size
	// th is the window's live theme, kept from Layout for Access, which has no frame.
	th *theme.Live
	// place is where the bar stood in the window as it was last painted.
	place windowPlace
}

// barText is a place's shaped words.
type barText struct {
	label, ell, count shapedText
	cut               bool
}

func newBarStrip(b *BottomBar) *barStrip {
	s := &barStrip{Control: newControl(), b: b, hot: -1, was: -1, ring: anim.NewFloat(0), pill: anim.NewFloat(0),
		hotV: anim.NewFloat(0), wasV: anim.NewFloat(0), badges: map[int]*anim.Float{}}
	s.Add(s.ring, s.pill, s.hotV, s.wasV)
	return s
}

// placesChanged follows the places after SetItems, from says where each place was: the badges stay with their
// places, and grow or shrink where a count came or went.
func (s *barStrip) placesChanged(from []int, u *gunim.UI) {
	old := s.badges
	s.badges = map[int]*anim.Float{}
	for i, was := range from {
		on := anim.OnOff(s.b.items[i].Count > 0 || s.b.items[i].Dot)
		v := old[was]
		if was < 0 || v == nil {
			v = anim.NewFloat(on)
			s.Add(v)
			if was < 0 && s.laid && u != nil {
				v.Jump(0)
			}
		}
		s.badges[i] = v
		if v.Target() != on {
			if s.laid && u != nil {
				v.Animate(on, Quick.Get(u.Theme()))
			} else {
				v.Jump(on)
			}
		}
	}
	for i, v := range old {
		if !slices.Contains(from, i) {
			s.Remove(v)
		}
	}
	s.shaped = make([]barText, len(s.b.items))
	if !s.laid || u == nil {
		s.jump()
		return
	}
	s.ring.Animate(float32(s.cursor), Quick.Get(u.Theme()))
	s.pill.Animate(float32(s.b.selected), Settle.Get(u.Theme()))
}

// jump puts the pill and the ring on their places at once.
func (s *barStrip) jump() {
	s.pill.Jump(float32(s.b.selected))
	s.ring.Jump(float32(s.cursor))
}

// glide sends the pill from place from to place to.
func (s *barStrip) glide(from, to int, th *theme.Live) {
	if !s.pill.Active() {
		s.pill.Jump(float32(from))
	}
	s.pill.Animate(float32(to), Settle.Get(th))
	if s.cursor == to {
		s.ring.Animate(float32(to), Quick.Get(th))
	}
}

// moveCursor puts the focus ring on place i, gliding once laid out.
func (s *barStrip) moveCursor(i int, u *gunim.UI) {
	if len(s.b.items) == 0 {
		return
	}
	s.cursor = min(max(i, 0), len(s.b.items)-1)
	if !s.laid || u == nil {
		s.ring.Jump(float32(s.cursor))
		return
	}
	s.ring.Animate(float32(s.cursor), Quick.Get(u.Theme()))
	u.Invalidate()
}

// barHeight is the bar's height along the bottom, less the room it keeps under it, and also a place's height in
// the rail.
func (s *barStrip) barHeight(th *theme.Live) float32 {
	line := s.line.shape(faceIn(Font, th), "Ag", BarLabelSize.Get(th)).Height()
	pad := BarPadding.Get(th)
	return max(BarHeight.Get(th), 2*pad+BarPillHeight.Get(th)+BarLabelGap.Get(th)+line)
}

// slot returns place i's box, in the strip's space.
func (s *barStrip) slot(i float32, th *theme.Live) geom.Rect {
	n := max(len(s.b.items), 1)
	if s.rail {
		h := s.barHeight(th)
		return geom.Rc(0, BarPadding.Get(th)+i*h, s.size.W, h)
	}
	w := s.size.W / float32(n)
	return geom.Rc(i*w, 0, w, s.barHeight(th))
}

// at returns the place at p, or -1.
func (s *barStrip) at(p geom.Point, th *theme.Live) int {
	for i := range s.b.items {
		if s.slot(float32(i), th).Contains(p) {
			return i
		}
	}
	return -1
}

// pillRect is the pill in slot r.
func pillRect(r geom.Rect, th *theme.Live) geom.Rect {
	w, h := min(BarPillWidth.Get(th), r.Size().W-4), BarPillHeight.Get(th)
	return geom.Rc(r.Min.X+(r.Size().W-w)/2, r.Min.Y+BarPadding.Get(th), w, h)
}

// Layout implements [gunim.Node].
func (s *barStrip) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	if s.Disabled {
		s.click.Cancel()
	}
	s.follow(f.Theme)
	s.size, s.th = c.Max, f.Theme
	return s.size
}

// Handle implements [gunim.Handler].
func (s *barStrip) Handle(e input.Event, u *gunim.UI) bool {
	s.showTip(e, u, s)
	th := u.Theme()
	if s.Disabled {
		s.setHot(-1, th)
		return s.handleDisabled(e, u, nil)
	}
	n := len(s.b.items)
	switch e := e.(type) {
	case input.PointerEnter:
		s.setHot(s.at(e.Pos, th), th)
		s.tip.Handle(e, u, s, s.tipText(s.hot))
		return false
	case input.PointerMove:
		s.setHot(s.at(e.Pos, th), th)
		s.tip.Handle(e, u, s, s.tipText(s.hot))
		return false
	case input.PointerLeave:
		s.setHot(-1, th)
		s.tip.Handle(e, u, s, "")
		return false
	case input.PointerDown:
		s.tip.Handle(e, u, s, "")
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := s.at(e.Pos, th)
		s.click.Press(e, i)
		if e.Touch && i >= 0 {
			// A finger's press lights the place on the frame it lands.
			s.setHot(i, th)
			s.hotV.Jump(1)
		}
	case input.PointerUp:
		i := s.at(e.Pos, th)
		if e.Touch {
			s.setHot(-1, th)
		}
		if s.click.Release(e, i) && i >= 0 {
			s.b.choose(i, u)
		}
	case input.KeyPress:
		if e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) || e.Mods.Has(input.ModSuper) || n == 0 {
			return false
		}
		back, on := input.KeyLeft, input.KeyRight
		if s.rail {
			back, on = input.KeyUp, input.KeyDown
		}
		switch e.Key {
		case back:
			s.moveCursor(s.cursor-1, u)
		case on:
			s.moveCursor(s.cursor+1, u)
		case input.KeyHome:
			s.moveCursor(0, u)
		case input.KeyEnd:
			s.moveCursor(n-1, u)
		case input.KeyEnter, input.KeyKPEnter, input.KeySpace:
			s.b.choose(s.cursor, u)
		default:
			return false
		}
	case input.FocusGained:
		s.ring.Jump(float32(s.cursor))
	default:
		if !s.ringFollows(e, th) {
			return false
		}
	}
	u.Invalidate()
	return true
}

// setHot lights place i, or none for -1: it lights quickly, and the place left fades out more slowly.
func (s *barStrip) setHot(i int, th *theme.Live) {
	if i == s.hot {
		return
	}
	if s.hot >= 0 {
		s.was = s.hot
		s.wasV.Jump(s.hotV.Value())
		s.wasV.Animate(0, Settle.Get(th))
	}
	s.hot = i
	s.hotV.Jump(0)
	if i >= 0 {
		s.hotV.Animate(1, Quick.Get(th))
	}
}

// tipText is place i's whole label where it was cut short.
func (s *barStrip) tipText(i int) string {
	if i >= 0 && i < len(s.shaped) && s.shaped[i].cut && i < len(s.b.items) {
		return s.b.items[i].Label
	}
	return ""
}

// Step implements [anim.Stepper].
func (s *barStrip) Step(dt time.Duration) bool { return s.Group.Step(dt) }

// Paint implements [gunim.Node]: the ground and its edge, the pill, then each place.
func (s *barStrip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	s.place.note(f, s)
	defer s.faint(p, box)()
	ground := geom.Rect{Max: box.Point()}
	if !s.rail {
		// The ground runs on under the system's navigation bar where the bar stands on it.
		ground.Max.Y += s.place.runsUnder(f)
	}
	p.RRect(ground, 0, paint.Solid(MenubarFill.Get(th)))
	edge := MenuBorder.Get(th)
	if s.rail {
		p.RRect(geom.Rc(box.W-1, 0, 1, box.H), 0, paint.Solid(edge))
	} else {
		p.RRect(geom.Rc(0, 0, box.W, 1), 0, paint.Solid(edge))
	}
	if len(s.b.items) == 0 {
		return
	}
	if len(s.shaped) != len(s.b.items) {
		s.shaped = make([]barText, len(s.b.items))
	}
	pill := pillRect(s.slot(s.pill.Value(), th), th)
	p.RRect(pill, pill.Size().H/2, paint.Solid(NavSelected.Get(th)))
	for i := range s.b.items {
		s.paintPlace(p, th, i)
	}
	ring := pillRect(s.slot(s.ring.Value(), th), th)
	s.paintRing(p, ring, ring.Size().H/2, th)
}

// paintPlace paints place i: its hover, its icon with a badge on its corner, and its label.
func (s *barStrip) paintPlace(p *paint.Painter, th *theme.Live, i int) {
	it := s.b.items[i]
	slot := s.slot(float32(i), th)
	pill := pillRect(slot, th)
	lit := float32(0)
	switch i {
	case s.hot:
		lit = s.hotV.Value()
	case s.was:
		lit = s.wasV.Value()
	}
	// How much of the pill lies on this place, as it glides between places.
	on := max(0, 1-abs32(s.pill.Value()-float32(i)))
	if lit > 0.01 && on < 0.99 {
		p.RRect(pill, pill.Size().H/2, paint.Solid(paint.Faded(NavHover.Get(th), min(lit, 1)*(1-on))))
	}
	ink := anim.Mix(anim.ColorCodec, CaptionInk.Get(th), Ink.Get(th), on)
	mark := Accent.Get(th)
	if th.Sets(NavSelectedInk.Key()) {
		mark = NavSelectedInk.Get(th)
	}
	iconInk := anim.Mix(anim.ColorCodec, CaptionInk.Get(th), mark, on)
	size := min(BarIconSize.Get(th), pill.Size().H)
	ic := geom.Rc(pill.Min.X+(pill.Size().W-size)/2, pill.Min.Y+(pill.Size().H-size)/2, size, size)
	if it.Icon != nil {
		PaintIcon(p, th, it.Icon, ic, iconInk)
	}
	s.paintBadge(p, th, i, ic)
	t := &s.shaped[i]
	face := faceIn(Font, th)
	if i == s.b.selected {
		face = faceIn(BoldFont, th)
	}
	run := t.label.shape(face, it.Label, BarLabelSize.Get(th))
	room := slot.Size().W - 2*BarLabelGap.Get(th)
	t.cut = run.Advance > room
	if room <= 0 {
		return
	}
	run = fitRun(run, &t.ell, room)
	y := pill.Max.Y + BarLabelGap.Get(th)
	run.Paint(p, geom.Pt(slot.Min.X+(slot.Size().W-run.Advance)/2, y), ink)
}

// paintBadge paints place i's count or dot on the corner of its icon, ic, as far as it has grown in.
func (s *barStrip) paintBadge(p *paint.Painter, th *theme.Live, i int, ic geom.Rect) {
	it := s.b.items[i]
	v := s.badges[i]
	if v == nil {
		return
	}
	t := min(max(v.Value(), 0), 1)
	if t < 0.01 {
		return
	}
	fill, ink := ButtonDangerFill.Get(th), ButtonStrongInk.Get(th)
	corner := geom.Pt(ic.Max.X, ic.Min.Y)
	if it.Count <= 0 {
		d := BarDot.Get(th) * t
		p.RRect(geom.Rc(corner.X-d/2, corner.Y, d, d), d/2, paint.Solid(fill))
		return
	}
	words := strconv.Itoa(it.Count)
	if it.Count > 99 {
		words = "99+"
	}
	run := s.shaped[i].count.shape(faceIn(Font, th), words, BadgeSize.Get(th))
	h := BadgeHeight.Get(th)
	w := max(run.Advance+h/2, h)
	r := geom.Rc(corner.X-h/2, corner.Y-h/3, w, h)
	defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: t})()
	p.RRect(r, h/2, paint.Solid(fill))
	run.Paint(p, geom.Pt(r.Min.X+(w-run.Advance)/2, r.Min.Y+(h-run.Height())/2), ink)
}

// Access implements [gunim.Accessible]: a tab list with a tab for each place, the current one selected.
func (s *barStrip) Access() access.Info {
	info := access.Info{Role: access.RoleTabList, Name: s.accessName(s.b.Label), State: s.accessState()}
	if s.cursor < len(s.b.items) {
		info.Active = s.cursor + 1
	}
	for i, it := range s.b.items {
		part := access.Info{Role: access.RoleTab, Name: it.Label, State: s.accessState(), Key: partKey(it.Key, i),
			Actions: []string{access.ActionPress}, Bounds: s.slotBounds(i)}
		switch {
		case it.Count > 0 && it.CountLabel != "":
			part.Description = it.CountLabel
		case it.Count > 0:
			part.Description = strconv.Itoa(it.Count)
		}
		if i == s.b.selected {
			part.State |= access.StateSelected
		}
		info.Parts = append(info.Parts, part)
	}
	return s.accessHeld(info)
}

// slotBounds is place i's box as laid out last, for a screen reader.
func (s *barStrip) slotBounds(i int) geom.Rect {
	if s.th == nil {
		return geom.Rect{}
	}
	return s.slot(float32(i), s.th)
}

// AccessAct implements [gunim.AccessActor]: pressing a tab goes to its place.
func (s *barStrip) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= len(s.b.items) || s.Disabled {
		return false
	}
	s.b.choose(r.Part, u)
	return true
}
