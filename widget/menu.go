package widget

import (
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Menu is a list of items to pick from, shown in a popup: the list a
// [Dropdown] opens and the menu a [ContextMenu] opens.
//
// A highlight glides to the item under the pointer, or to the one the
// arrow keys reach. The menu fades and unfolds from its top as it
// opens, and fades as it closes. Where the display server cannot blend
// windows, the menu fills its window and appears at once.
//
// A menu taller than the room it is given, or than the screen leaves
// it, scrolls: by the wheel, a finger, its bar, and the keys, which
// keep the highlight in view.
type Menu struct {
	anim.Group

	Items []string
	// Hints, Checked and Disabled say more about the items, in the same
	// order, and may be shorter than Items. A hint shows at the right,
	// such as the item's shortcut. A checked item has a tick before it.
	// A disabled item is dimmed, and the pointer and the keys pass it
	// by.
	Hints    []string
	Checked  []bool
	Disabled []bool
	// Icons shows an icon before each item's text, in the same order, and may be shorter than Items.
	Icons []*icon.Icon
	// Swatches shows a dot of each colour before its item's text, in place of an icon, as for a choice of calendars
	// or labels by their colours. It may be shorter than Items, and a colour with no alpha shows none.
	Swatches []color.NRGBA
	// Breaks lists the items a line goes above, grouping the menu.
	Breaks []int
	// Captions lists the items that are captions over the group below
	// them: drawn small and dim, and passed by like a disabled item.
	Captions []int
	// Pick runs on the UI goroutine with the index of the item picked.
	Pick func(i int, u *gunim.UI)
	// OnHighlight runs on the UI goroutine when the pointer or a key
	// moves the highlight, with the item it moved to, or -1.
	OnHighlight func(i int, u *gunim.UI)
	// MinWidth is the narrowest the menu gets, so a drop-down's list is
	// at least as wide as the drop-down.
	MinWidth float32
	// AccessKeys has the items mark their access keys with a & before the letter, as "&Open", which is not drawn.
	// Without a mark an item's key is its first letter. Typing a key picks its item; see [Menu.Key].
	AccessKeys bool

	// cues underlines the access keys.
	cues bool

	hot int
	// picks counts the lines picked, so a highlight moved on the way to
	// a pick is not told of after it: the pick has closed the menu.
	picks int
	glide bool
	hotY  *anim.Float
	hotOn *anim.Float
	in    *anim.Float

	rows     []shapedText
	hintRuns []shapedText
	// tops holds each row's top, from the first row's.
	tops []float32
	// card is where the menu is drawn in its box, row and pad the row
	// height and the space around the rows, all from the last layout.
	card        geom.Rect
	margin      float32
	row, pad    float32
	transparent bool
	// bare draws only the highlight and the rows, fading, for a card drawn round them by something else: the menu
	// beside a compact menubar's list.
	bare bool
	// pointer and wasPointer are where the pointer last moved on the menu, and where it was the move before.
	pointer, wasPointer geom.Point
	// scroll scrolls the rows when they are taller than the card, in the space whose top is the first row's.
	scroll scrolling
	// room is how tall the screen lets the menu's window be, from FitPopup, or 0 where nothing says.
	room float32
}

// NewMenu returns a menu of items, with nothing highlighted.
func NewMenu(items ...string) *Menu {
	m := &Menu{
		Items: items,
		hot:   -1,
		hotY:  anim.NewFloat(0),
		hotOn: anim.NewFloat(0),
		in:    anim.NewFloat(0),

		scroll: newScrolling(),
	}
	m.Add(m.hotY, m.hotOn, m.in, &m.scroll)
	return m
}

func flag(list []bool, i int) bool { return i >= 0 && i < len(list) && list[i] }

func (m *Menu) enabled(i int) bool {
	return i >= 0 && i < len(m.Items) && !flag(m.Disabled, i) && !slices.Contains(m.Captions, i)
}

// step returns the enabled item from i on, going by dir, or from
// staying where it is when there is none.
func (m *Menu) step(from, i, dir int) int {
	for ; i >= 0 && i < len(m.Items); i += dir {
		if m.enabled(i) {
			return i
		}
	}
	return from
}

// around returns the next enabled item from the highlight, going dir, and round from one end to the other, as menus
// do on Windows. With nothing highlighted, Down starts at the first item and Up at the last.
func (m *Menu) around(dir int) int {
	n := len(m.Items)
	at := m.hot
	if at < 0 && dir < 0 {
		at = n
	}
	for k := 1; k <= n; k++ {
		i := ((at+dir*k)%n + n) % n
		if m.enabled(i) {
			return i
		}
	}
	return m.hot
}

// Highlight moves the highlight to item i, gliding from where it was.
// A negative i, or a disabled item, takes it away.
func (m *Menu) Highlight(i int) {
	if i >= len(m.Items) {
		i = len(m.Items) - 1
	}
	if !m.enabled(i) {
		i = -1
	}
	m.hot = i
	if i < 0 {
		m.hotOn.Animate(0, anim.Snappy)
		return
	}
	m.hotOn.Animate(1, anim.Snappy)
	if !m.glide {
		// Before the first layout there is nowhere to glide from.
		return
	}
	m.hotY.Animate(m.rowTop(i), anim.Snappy)
	m.reveal(i)
}

// reveal scrolls just far enough to bring item i fully into view.
func (m *Menu) reveal(i int) {
	if !m.scroll.scrollable() || i < 0 || i >= len(m.tops) {
		return
	}
	top, at := m.tops[i], m.scroll.base()
	switch {
	case top < at:
		m.scroll.ScrollTo(top, Quick.Get(m.scroll.th))
	case top+m.row > at+m.scroll.viewport:
		m.scroll.ScrollTo(top+m.row-m.scroll.viewport, Quick.Get(m.scroll.th))
	}
}

// FitPopup implements [gunim.PopupFitter]: the menu keeps to the taller of the room below its anchor and above it,
// and scrolls what does not fit.
func (m *Menu) FitPopup(r driver.Room) {
	m.room = 0
	if room := max(r.Below, r.Above); !math.IsInf(float64(room), 1) {
		m.room = room
	}
}

// DragsTouch implements [gunim.TouchDragger]: a finger on the bar's thumb drags it, and anywhere else scrolls the
// rows.
func (m *Menu) DragsTouch() bool { return m.scroll.DragsTouch() }

// scrolled returns e with its position moved into the scroll's space, whose top is the first row's.
func (m *Menu) scrolled(e input.Event) input.Event {
	d := geom.Pt(0, m.card.Min.Y+m.pad)
	switch e := e.(type) {
	case input.PointerEnter:
		e.Pos = e.Pos.Sub(d)
		return e
	case input.PointerMove:
		e.Pos = e.Pos.Sub(d)
		return e
	case input.PointerDown:
		e.Pos = e.Pos.Sub(d)
		return e
	case input.PointerUp:
		e.Pos = e.Pos.Sub(d)
		return e
	case input.Scroll:
		e.Pos = e.Pos.Sub(d)
		return e
	}
	return e
}

// Highlighted returns the highlighted item, or -1.
func (m *Menu) Highlighted() int { return m.hot }

// Key moves the highlight with the arrow keys, Home and End, and picks
// the highlighted item with Enter or Space. With AccessKeys, a letter
// picks the item whose key it is, or moves the highlight among several.
// It is for the node that opened the menu, which keeps the keyboard, to
// pass keys on. It reports whether it used the key.
func (m *Menu) Key(k input.KeyPress, u *gunim.UI) bool {
	defer m.toldUnlessPicked(m.hot, m.picks, u)
	switch k.Key {
	case input.KeyDown:
		m.Highlight(m.around(1))
	case input.KeyUp:
		m.Highlight(m.around(-1))
	case input.KeyHome:
		m.Highlight(m.step(m.hot, 0, 1))
	case input.KeyEnd:
		m.Highlight(m.step(m.hot, len(m.Items)-1, -1))
	case input.KeyEnter, input.KeyKPEnter, input.KeySpace:
		if m.enabled(m.hot) && m.Pick != nil {
			m.pick(m.hot, u)
		}
	default:
		return m.AccessKeys && m.pickByKey(k, u)
	}
	return true
}

// pickByKey picks the item whose access key k types. With several such items it moves the highlight to the next.
func (m *Menu) pickByKey(k input.KeyPress, u *gunim.UI) bool {
	r := keyRune(k)
	if r == 0 || k.Mods.Has(input.ModControl) || k.Mods.Has(input.ModAlt) {
		return false
	}
	var hits []int
	for i, s := range m.Items {
		if _, key, _ := accessKey(s); key == r && m.enabled(i) {
			hits = append(hits, i)
		}
	}
	if len(hits) == 0 {
		return false
	}
	next := hits[0]
	for _, i := range hits {
		if i > m.hot {
			next = i
			break
		}
	}
	m.Highlight(next)
	if len(hits) == 1 && m.Pick != nil {
		m.pick(next, u)
	}
	return true
}

// label returns item i as it is drawn: without its access key's mark, with AccessKeys.
func (m *Menu) label(i int) string {
	if !m.AccessKeys {
		return m.Items[i]
	}
	shown, _, _ := accessKey(m.Items[i])
	return shown
}

// PopupPadding implements [gunim.PopupPadder]: the room around the
// menu for its shadow.
func (m *Menu) PopupPadding() geom.Insets { return geom.Uniform(m.margin) }

// Transition implements [gunim.Transitioner].
func (m *Menu) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		m.in.Animate(1, Quick.Get(f.Theme))
	case gunim.Exiting:
		m.in.Animate(0, Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !m.in.Active()
}

// Handle implements [gunim.Handler].
func (m *Menu) Handle(e input.Event, u *gunim.UI) bool {
	defer m.toldUnlessPicked(m.hot, m.picks, u)
	if m.scroll.scrollable() {
		in := m.scrolled(e)
		if m.scroll.barEvent(in, u) {
			return true
		}
		switch e := e.(type) {
		case input.Scroll:
			m.scroll.handle(in, u)
			return true
		case input.PointerUp:
			if m.scroll.ClaimsPointer(e.Pos) {
				// The end of a press on the bar's track, which paged.
				return true
			}
		}
	}
	switch e := e.(type) {
	case input.PointerEnter:
		// The pointer arriving without moving, as when the menu opens
		// under it, leaves the highlight where the keys put it. A move
		// follows any real arrival.
	case input.PointerMove:
		m.wasPointer, m.pointer = m.pointer, e.Pos
		if i := m.rowAt(e.Pos); i >= 0 {
			m.Highlight(i)
		}
	case input.PointerDown:
	case input.PointerUp:
		if i := m.rowAt(e.Pos); m.enabled(i) && m.Pick != nil {
			m.pick(i, u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// dismissed returns shut for a popup's Dismiss, with [gunim.CueClose]
// played first: the user sent the popup away without choosing in it.
func dismissed(n gunim.Node, shut func(*gunim.UI)) func(*gunim.UI) {
	return func(u *gunim.UI) {
		u.Cue(gunim.CueClose, n)
		shut(u)
	}
}

// pick picks item i, counting it.
func (m *Menu) pick(i int, u *gunim.UI) {
	m.picks++
	u.Cue(gunim.CuePress, m)
	m.Pick(i, u)
}

// toldUnlessPicked runs OnHighlight when the highlight has moved from
// was, unless a line was picked since picks was counted: the owner is
// told of the pick, and whatever the pick closed is not asked about
// the highlight on the way to it.
func (m *Menu) toldUnlessPicked(was, picks int, u *gunim.UI) {
	if m.picks == picks {
		m.told(was, u)
	}
}

// told runs OnHighlight when the highlight has moved from was.
func (m *Menu) told(was int, u *gunim.UI) {
	if m.hot != was && m.OnHighlight != nil {
		m.OnHighlight(m.hot, u)
	}
}

// rowAt returns the item at p, in the menu's space, or -1.
func (m *Menu) rowAt(p geom.Point) int {
	if !m.card.Contains(p) || m.row <= 0 {
		return -1
	}
	for i := range m.Items {
		if y := m.rowY(i); p.Y >= y && p.Y < y+m.row {
			return i
		}
	}
	return -1
}

// RowRect returns where item i is in the menu's space, from the last
// layout, as for a menu opening beside it.
func (m *Menu) RowRect(i int) geom.Rect {
	return geom.Rc(m.card.Min.X, m.rowY(i), m.card.Size().W, m.row)
}

// rowY returns the top of row i in the menu's space, where it is scrolled to now.
func (m *Menu) rowY(i int) float32 { return m.rowTop(i) - m.scroll.Offset() }

// rowTop returns the top of row i in the menu's space, as it is with the menu scrolled to its top.
func (m *Menu) rowTop(i int) float32 {
	top := float32(i) * m.row
	if i >= 0 && i < len(m.tops) {
		top = m.tops[i]
	}
	return m.card.Min.Y + m.pad + top
}

// menuBreak is the room a line between groups takes.
const menuBreak = 9

// menuTick is the room a tick takes before the items, when any has one.
const menuTick = 18

// Layout implements [gunim.Node].
func (m *Menu) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	m.transparent = f.Transparent
	m.margin = 0
	if f.Transparent && !m.bare {
		m.margin = MenuMargin.Get(th)
	}
	m.row, m.pad = MenuRowHeight.Get(th), MenuPadding.Get(th)
	if len(m.rows) != len(m.Items) {
		m.rows = make([]shapedText, len(m.Items))
		m.hintRuns = make([]shapedText, len(m.Items))
	}
	size := TextSize.Get(th)
	gutter := m.gutter() + m.iconRoom(th)
	w := m.MinWidth
	m.tops = m.tops[:0]
	y := float32(0)
	for i := range m.Items {
		if slices.Contains(m.Breaks, i) && i > 0 {
			y += menuBreak
		}
		m.tops = append(m.tops, y)
		y += m.row
		line := gutter + m.rows[i].shape(faceIn(Font, th), m.label(i), size).Advance + 2*MenuRowPadding.Get(th)
		if i < len(m.Hints) && m.Hints[i] != "" {
			line += 32 + m.hintRuns[i].shape(faceIn(Font, th), m.Hints[i], size*0.9).Advance
		}
		w = max(w, line)
	}
	h := y + 2*m.pad
	limit := c.Max.H
	if m.room > 0 {
		limit = min(limit, m.room)
	}
	if limit -= 2 * m.margin; limit > 0 && h > limit {
		// Too tall: the rows scroll, and the menu leaves room at the right for the bar.
		h = max(limit, m.row+2*m.pad)
		w += ScrollbarGrabWidth.Get(th)
	}
	m.card = geom.Rc(m.margin, m.margin, w, h)
	m.scroll.fit(y, geom.Sz(m.card.Max.X, h-2*m.pad), th)
	if m.hot >= 0 && !m.glide {
		m.hotY.Jump(m.rowTop(m.hot))
		// The menu opens with the highlighted row in its middle.
		m.scroll.jumpTo(m.scroll.clamp(m.tops[m.hot] - (m.scroll.viewport-m.row)/2))
	}
	m.glide = true
	return c.Constrain(geom.Sz(w+2*m.margin, h+2*m.margin))
}

// Paint implements [gunim.Node].
func (m *Menu) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	th := f.Theme
	card := m.card
	radius := MenuRadius.Get(th)
	t := m.in.Value()
	switch {
	case m.bare:
		defer p.Layer(paint.LayerOpts{Bounds: card, Opacity: min(1, max(0, t))})()
		if !m.transparent {
			radius = 0
		}
	case m.transparent:
		// Fade in and unfold from the top edge, which is the edge that
		// meets the anchor when the menu opens below it.
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: card.Max.Add(geom.Pt(m.margin, m.margin))}, Opacity: min(1, max(0, t))})()
		top := geom.Pt(card.Center().X, card.Min.Y)
		defer p.Push(paint.Scale(0.96+0.04*t, top))()
		shadow := MenuShadow.Get(th)
		p.ShadowRRect(card, radius, paint.Solid(MenuFill.Get(th)), paint.Shadow{
			Offset: geom.Pt(0, 3),
			Blur:   m.margin * 0.6,
			Color:  shadow,
		})
		p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	default:
		radius = 0
		p.RRect(card, 0, paint.Solid(MenuFill.Get(th)))
		p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	}

	if m.scroll.scrollable() {
		// The rows fade where more of them lie past the card's edge.
		defer m.paintBar(p, f)
		defer m.scroll.layer(p, card, th)()
	}
	if on := m.hotOn.Value(); on > 0.01 {
		c := MenuHot.Get(th)
		c.A = uint8(float32(c.A) * min(on, 1))
		inset := m.pad
		y := m.hotY.Value() - m.scroll.Offset()
		p.RRect(geom.Rc(card.Min.X+inset, y, card.Size().W-2*inset, m.row), max(0, radius-inset), paint.Solid(c))
	}
	ink := Ink.Get(th)
	dim := ink
	dim.A /= 3
	hint := MenuHint.Get(th)
	pad := MenuRowPadding.Get(th)
	gutter := m.gutter()
	for _, i := range m.Breaks {
		if i > 0 && i < len(m.Items) {
			y := m.rowY(i) - menuBreak/2
			p.RRect(geom.Rc(card.Min.X+pad, y, card.Size().W-2*pad, 1), 0, paint.Solid(MenuBorder.Get(th)))
		}
	}
	for i := range m.Items {
		col := ink
		if !m.enabled(i) {
			col = dim
		}
		if slices.Contains(m.Captions, i) {
			caption := m.rows[i].shape(faceIn(Font, th), m.label(i), TextSize.Get(th)*0.85)
			caption.Paint(p, geom.Pt(card.Min.X+pad, m.rowY(i)+(m.row-caption.Height())/2), hint)
			continue
		}
		run := m.rows[i].run
		y := m.rowY(i) + (m.row-run.Height())/2
		if flag(m.Checked, i) {
			s := IconSize.Get(th)
			paintIcon(p, th, icon.Check, geom.Rc(card.Min.X+pad+gutter/2-1-s/2, m.rowY(i)+(m.row-s)/2, s, s), col, 1)
		}
		x := card.Min.X + pad + gutter
		if i < len(m.Icons) && m.Icons[i] != nil {
			s := IconSize.Get(th)
			paintIcon(p, th, m.Icons[i], geom.Rc(x, m.rowY(i)+(m.row-s)/2, s, s), col, 1)
		} else if i < len(m.Swatches) && m.Swatches[i].A > 0 {
			paintSwatch(p, th, m.Swatches[i], geom.Pt(x, m.rowY(i)+m.row/2))
		}
		run.Paint(p, geom.Pt(x+m.iconRoom(th), y), col)
		if m.cues && m.AccessKeys {
			_, _, at := accessKey(m.Items[i])
			underline(p, run, at, geom.Pt(x+m.iconRoom(th), y), col)
		}
		if i < len(m.Hints) && m.Hints[i] != "" {
			h := m.hintRuns[i].run
			if !m.enabled(i) {
				hint.A /= 3
			}
			h.Paint(p, geom.Pt(card.Max.X-pad-h.Advance, m.rowY(i)+(m.row-h.Height())/2), hint)
			hint = MenuHint.Get(th)
		}
	}
}

// paintBar draws the scroll bar down the card's right edge.
func (m *Menu) paintBar(p *paint.Painter, f gunim.Frame) {
	defer p.Push(paint.Translate(geom.Pt(0, m.card.Min.Y+m.pad)))()
	m.scroll.paintBar(p, f)
}

// iconRoom is the room icons take before the items' text, when any item has one or a swatch.
func (m *Menu) iconRoom(th *theme.Live) float32 {
	for _, ic := range m.Icons {
		if ic != nil {
			return IconSize.Get(th) + IconGap.Get(th)
		}
	}
	for _, c := range m.Swatches {
		if c.A > 0 {
			return IconSize.Get(th) + IconGap.Get(th)
		}
	}
	return 0
}

// paintSwatch draws a dot of c in the room of an icon whose left edge's middle is at.
func paintSwatch(p *paint.Painter, th *theme.Live, c color.NRGBA, at geom.Point) {
	s := IconSize.Get(th)
	d := s * 0.62
	p.RRect(geom.Rc(at.X+(s-d)/2, at.Y-d/2, d, d), d/2, paint.Solid(c))
}

// gutter is the room before the items' titles: a tick's, when any item has one.
func (m *Menu) gutter() float32 {
	if slices.Contains(m.Checked, true) {
		return menuTick
	}
	return 0
}

// Dropdown shows one item of a list, and opens the list in a popup to
// pick another. The list opens below the drop-down, or above it where
// the screen runs out, with the chosen item highlighted.
//
// With focus, Space, Enter and the arrow keys open the list; the arrow
// keys, Home and End move through it; Enter or Space picks; Escape and
// Tab close it.
type Dropdown struct {
	anim.Group

	Items    []string
	Selected int
	// Icons shows an icon before each item, as in a [Menu], and before the chosen one on the drop-down itself, and
	// Swatches a dot of a colour.
	Icons    []*icon.Icon
	Swatches []color.NRGBA
	// Label names the drop-down for a screen reader, as the label
	// beside it does on screen.
	Label string
	// Disabled shows the drop-down faint, and it takes no clicks, keys
	// or focus, for a choice that does not apply now.
	Disabled bool
	// MaxWidth caps the drop-down's width, cutting a longer item short with
	// an ellipsis. Zero leaves it as wide as its longest item.
	MaxWidth float32
	// OnChange turns a new choice into an intent for the application.
	OnChange func(i int) gunim.Intent
	// picked is local behaviour, set by OnPick.
	picked func(i int, u *gunim.UI)

	hover *anim.Float
	ring  *anim.Float
	turn  *anim.Float

	popup *gunim.Popup
	menu  *Menu
	size  geom.Size
	shown shapedText
	ell   shapedText
}

// OnPick wires behaviour that runs inside the window when the user
// picks an item, such as filling in a form from a saved entry.
func (d *Dropdown) OnPick(fn func(i int, u *gunim.UI)) { d.picked = fn }

// NewDropdown returns a drop-down of items with the first chosen.
func NewDropdown(items ...string) *Dropdown {
	d := &Dropdown{
		Items: items,
		hover: anim.NewFloat(0),
		ring:  anim.NewFloat(0),
		turn:  anim.NewFloat(0),
	}
	d.Add(d.hover, d.ring, d.turn)
	return d
}

// Focusable implements [gunim.Focusable].
func (d *Dropdown) Focusable() bool { return !d.Disabled }

// IsOpen reports whether the list is open.
func (d *Dropdown) IsOpen() bool { return d.popup != nil && d.popup.Open() }

// Handle implements [gunim.Handler].
func (d *Dropdown) Handle(e input.Event, u *gunim.UI) bool {
	if d.Disabled {
		return false
	}
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerEnter:
		d.hover.Animate(1, Quick.Get(th))
	case input.PointerLeave:
		d.hover.Animate(0, Settle.Get(th))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if d.IsOpen() {
			d.close(u)
		} else {
			d.open(u)
		}
	case input.PointerUp:
	case input.FocusRing:
		d.ring.Animate(ringTo(e), Quick.Get(th))
	case input.FocusLost:
		d.ring.Animate(0, Settle.Get(th))
		d.close(u)
	case input.KeyPress:
		return d.key(e, u)
	default:
		return false
	}
	return true
}

func (d *Dropdown) key(k input.KeyPress, u *gunim.UI) bool {
	if !d.IsOpen() {
		switch k.Key {
		case input.KeySpace, input.KeyEnter, input.KeyKPEnter, input.KeyDown, input.KeyUp:
			d.open(u)
			return true
		default:
			return false
		}
	}
	switch k.Key {
	case input.KeyEscape:
		u.Cue(gunim.CueClose, d)
		d.close(u)
		return true
	case input.KeyTab:
		d.close(u)
		return false // focus moves on
	default:
		return d.menu.Key(k, u)
	}
}

func (d *Dropdown) open(u *gunim.UI) {
	m := NewMenu(d.Items...)
	m.Icons, m.Swatches = d.Icons, d.Swatches
	m.MinWidth = d.size.W
	m.Highlight(d.Selected)
	m.Pick = func(i int, u *gunim.UI) {
		d.close(u)
		if i != d.Selected {
			d.Selected = i
			if d.picked != nil {
				d.picked(i, u)
			}
			if d.OnChange != nil {
				u.Send(d, d.OnChange(i))
			}
		}
	}
	d.menu = m
	u.Cue(gunim.CueOpen, d)
	d.popup = u.OpenPopup(d, m, gunim.PopupOptions{
		Anchor:  geom.Rect{Max: d.size.Point()},
		Max:     geom.Sz(600, 480),
		Dismiss: dismissed(d, d.close),
	})
	d.turn.Animate(1, Quick.Get(u.Theme()))
}

func (d *Dropdown) close(u *gunim.UI) {
	if d.popup != nil {
		d.popup.Close()
		d.popup = nil
	}
	d.turn.Animate(0, Quick.Get(u.Theme()))
}

// Layout implements [gunim.Node]. The drop-down is as wide as its
// longest item.
func (d *Dropdown) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	w := float32(0)
	var probe shapedText
	for _, s := range d.Items {
		w = max(w, probe.shape(faceIn(Font, th), s, TextSize.Get(th)).Advance)
	}
	pad := FieldPadding.Get(th)
	w += 2*pad + chevron + pad + d.iconRoom(th)
	if d.MaxWidth > 0 {
		w = min(w, d.MaxWidth)
	}
	d.size = c.Constrain(geom.Sz(w, FieldHeight.Get(th)))
	return d.size
}

// icon returns item i's icon, or nil.
func (d *Dropdown) icon(i int) *icon.Icon {
	if i < 0 || i >= len(d.Icons) {
		return nil
	}
	return d.Icons[i]
}

// iconRoom is the room before the drop-down's text for the icons, when any item has one, so the text keeps its
// place from choice to choice.
func (d *Dropdown) iconRoom(th *theme.Live) float32 {
	for _, ic := range d.Icons {
		if ic != nil {
			return IconSize.Get(th) + IconGap.Get(th)
		}
	}
	for _, c := range d.Swatches {
		if c.A > 0 {
			return IconSize.Get(th) + IconGap.Get(th)
		}
	}
	return 0
}

// chevron is the width of the drop-down's arrow.
const chevron = 10

// Paint implements [gunim.Node].
func (d *Dropdown) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer faintIf(p, box, d.Disabled)()
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := FieldRadius.Get(th)
	if t := d.ring.Value(); t > 0 {
		ring := Accent.Get(th)
		ring.A = uint8(float32(ring.A) * 0.56 * min(t, 1))
		grow := 3 * t
		p.RRectStroke(geom.Rect{Min: geom.Pt(-grow, -grow), Max: geom.Pt(box.W+grow, box.H+grow)},
			radius+grow, paint.Fill{}, paint.Stroke{Width: 2, Color: ring})
	}
	fill := anim.Mix(anim.ColorCodec, ButtonFill.Get(th), ButtonHover.Get(th), d.hover.Value())
	p.RRectStroke(r, radius, paint.Solid(fill), paint.Stroke{Width: 1, Color: FieldBorder.Get(th)})

	pad := FieldPadding.Get(th)
	if d.Selected >= 0 && d.Selected < len(d.Items) {
		x := pad
		if ic := d.icon(d.Selected); ic != nil {
			s := IconSize.Get(th)
			paintIcon(p, th, ic, geom.Rc(x, (box.H-s)/2, s, s), Ink.Get(th), 1)
		} else if d.Selected < len(d.Swatches) && d.Swatches[d.Selected].A > 0 {
			paintSwatch(p, th, d.Swatches[d.Selected], geom.Pt(x, box.H/2))
		}
		x += d.iconRoom(th)
		run := d.shown.shape(faceIn(Font, th), d.Items[d.Selected], TextSize.Get(th))
		if room := box.W - x - pad - chevron - pad; run.Advance > room {
			run = cutRun(run, d.ell.shape(faceIn(Font, th), "…", TextSize.Get(th)), room)
		}
		run.Paint(p, geom.Pt(x, (box.H-run.Height())/2), Ink.Get(th))
	}

	// The chevron turns over as the list opens.
	c := geom.Pt(box.W-pad-chevron/2, box.H/2)
	defer p.Push(paint.Rotate(math.Pi*d.turn.Value(), c))()
	paintChevron(p, th, c, Ink.Get(th))
}

// paintChevron draws icon.ChevronDown centred on c.
func paintChevron(p *paint.Painter, th *theme.Live, c geom.Point, ink color.NRGBA) {
	s := IconSize.Get(th)
	paintIcon(p, th, icon.ChevronDown, geom.Rc(c.X-s/2, c.Y-s/2, s, s), ink, 1)
}

// ContextMenu opens a menu at the pointer when its child is clicked
// with the secondary button.
//
// While the menu is open, the context menu holds the keyboard and
// passes keys to the menu, and hands focus back once it closes.
type ContextMenu struct {
	Items []string
	// Hints, Checked, Disabled and Breaks say more about the items, as
	// they do in a [Menu]: a shortcut at the right, a tick, a dimmed item
	// the pointer passes by, and the items a line goes above.
	Hints    []string
	Checked  []bool
	Disabled []bool
	Breaks   []int
	// Icons shows an icon before each item, and Captions lists the
	// items that are captions over the group below them, as in a
	// [Menu].
	Icons    []*icon.Icon
	Captions []int
	// Prepare, when set, runs as the secondary button goes down at at, in
	// the context menu's space, before the menu opens. It may set the items
	// for the place pressed, and returning false opens no menu.
	Prepare func(at geom.Point, u *gunim.UI) bool
	// OnPick turns a picked item into an intent for the application.
	OnPick func(i int) gunim.Intent
	// Picked, when set, runs on the UI goroutine with the item picked, for
	// work inside the window, such as copying to the clipboard.
	Picked func(i int, u *gunim.UI)

	child gunim.Node
	popup *gunim.Popup
	menu  *Menu
	// back is what had focus before the menu opened.
	back gunim.Node
}

// NewContextMenu returns child with a context menu of items.
func NewContextMenu(child gunim.Node, items ...string) *ContextMenu {
	return &ContextMenu{Items: items, child: child}
}

// Children implements [gunim.Composite].
func (c *ContextMenu) Children() []gunim.Node { return []gunim.Node{c.child} }

// Focusable implements [gunim.Focusable]. The context menu takes focus
// only while its menu is open.
func (c *ContextMenu) Focusable() bool { return c.popup != nil && c.popup.Open() }

// Handle implements [gunim.Handler].
func (c *ContextMenu) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonSecondary {
			return false
		}
		if c.Prepare != nil && !c.Prepare(e.Pos, u) {
			return false
		}
		c.show(e.Pos, u)
	case input.KeyPress:
		if !c.Focusable() {
			return false
		}
		switch e.Key {
		case input.KeyEscape, input.KeyTab:
			u.Cue(gunim.CueClose, c)
			c.close(u)
		default:
			c.menu.Key(e, u)
		}
	case input.FocusLost:
		c.close(u)
	default:
		return false
	}
	return true
}

// Open opens the menu at at, in the context menu's space, as a press of the
// secondary button there does, for a key such as the Menu key.
func (c *ContextMenu) Open(at geom.Point, u *gunim.UI) {
	if c.Prepare != nil && !c.Prepare(at, u) {
		return
	}
	c.show(at, u)
}

func (c *ContextMenu) show(at geom.Point, u *gunim.UI) {
	c.close(u)
	m := NewMenu(c.Items...)
	m.Hints, m.Checked, m.Disabled, m.Breaks, m.Icons = c.Hints, c.Checked, c.Disabled, c.Breaks, c.Icons
	m.Captions = c.Captions
	m.Pick = func(i int, u *gunim.UI) {
		c.close(u)
		if c.Picked != nil {
			c.Picked(i, u)
		}
		if c.OnPick != nil {
			if v := c.OnPick(i); v != nil {
				u.Send(c, v)
			}
		}
	}
	c.menu = m
	u.Cue(gunim.CueOpen, c)
	c.popup = u.OpenPopup(c, m, gunim.PopupOptions{
		Anchor:  geom.Rect{Min: at, Max: at},
		Max:     geom.Sz(600, 480),
		Dismiss: dismissed(c, c.close),
	})
	if f := u.Focused(); f != c {
		c.back = f
	}
	u.Focus(c)
}

func (c *ContextMenu) close(u *gunim.UI) {
	if c.popup == nil {
		return
	}
	c.popup.Close()
	c.popup = nil
	if u.Focused() == c {
		back := c.back
		c.back = nil
		u.Focus(back)
	}
}

// Layout implements [gunim.Node].
func (c *ContextMenu) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	s := k.Layout(cs)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (c *ContextMenu) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// Tooltip shows a line of text in a popup below the pointer once it
// has rested on the tooltip's child for Delay, and hides it when the
// pointer leaves. Text changed while it shows turns over in place: the
// words before slide up and away as the new come up under them, and the
// popup glides to their width.
type Tooltip struct {
	Text  string
	Delay time.Duration

	child gunim.Node
	tip   tipper
}

// NewTooltip returns child with a tooltip of s.
func NewTooltip(child gunim.Node, s string) *Tooltip {
	return &Tooltip{Text: s, Delay: tipDelay, child: child}
}

// tipDelay is how long the pointer rests on a node before its tooltip shows.
const tipDelay = 600 * time.Millisecond

// Children implements [gunim.Composite].
func (t *Tooltip) Children() []gunim.Node { return []gunim.Node{t.child} }

// Handle implements [gunim.Handler].
func (t *Tooltip) Handle(e input.Event, u *gunim.UI) bool {
	t.tip.handle(e, u, t, t.Text, t.Delay)
	return false
}

// PartTip shows a tooltip about the part of a node under the pointer, such as an event in a calendar, once the
// pointer rests on it. The node passes each pointer event to Handle with the text for the part under the pointer,
// or an empty text where there is none. Moving to another part hides the tooltip and waits again.
type PartTip struct {
	tip tipper
}

// Handle follows the pointer event e over owner, with text the tooltip of the part under it.
func (p *PartTip) Handle(e input.Event, u *gunim.UI, owner gunim.Node, text string) {
	if text != p.tip.text {
		p.tip.hide(u)
		p.tip.text = text
	}
	if text == "" {
		if _, left := e.(input.PointerLeave); left {
			p.tip.quiet = false
		}
		return
	}
	if _, moved := e.(input.PointerMove); moved && p.tip.popup == nil && p.tip.stop == nil && !p.tip.quiet {
		p.tip.owner, p.tip.delay, p.tip.at = owner, tipDelay, e.(input.PointerMove).Pos
		p.tip.wait(u)
		return
	}
	p.tip.handle(e, u, owner, text, tipDelay)
}

// Hide hides the tooltip, such as while the part under the pointer is dragged.
func (p *PartTip) Hide(u *gunim.UI) { p.tip.hide(u) }

// tipper shows a tooltip for the node it serves.
type tipper struct {
	text  string
	delay time.Duration
	at    geom.Point
	stop  func()
	popup *gunim.Popup
	// owner is the node the popup opens from.
	owner gunim.Node
	// quiet keeps the tooltip down after a press or a scroll, until the pointer leaves.
	quiet bool
	// card is the popup's card while it shows, whose words turn over as the text changes.
	card *tip
}

// retext turns the words of the tooltip showing over to text, where they differ.
func (t *tipper) retext(text string) {
	t.text = text
	if t.card != nil && t.card.text != text && text != "" {
		t.card.turnTo(text)
	}
}

// handle shows the tooltip text for owner once the pointer rests on it for delay, and hides it when the pointer
// leaves, presses or scrolls. After a press or a scroll it stays down until the pointer leaves and comes back.
func (t *tipper) handle(e input.Event, u *gunim.UI, owner gunim.Node, text string, delay time.Duration) {
	t.owner, t.delay = owner, delay
	t.retext(text)
	switch e := e.(type) {
	case input.PointerEnter:
		t.at, t.quiet = e.Pos, false
		t.wait(u)
	case input.PointerMove:
		t.at = e.Pos
		if t.popup == nil && !t.quiet {
			t.wait(u) // the delay starts again while the pointer moves
		}
	case input.PointerLeave:
		t.quiet = false
		t.hide(u)
	case input.PointerDown, input.Scroll:
		t.quiet = true
		t.hide(u)
	}
}

func (t *tipper) wait(u *gunim.UI) {
	if t.stop != nil {
		t.stop()
	}
	t.stop = u.After(t.delay, t.show)
}

func (t *tipper) show(u *gunim.UI) {
	t.stop = nil
	if t.popup != nil || t.text == "" {
		return
	}
	// Below the pointer, clear of it, or above it where the screen runs
	// out.
	t.card = newTip(t.text)
	t.popup = u.OpenPopup(t.owner, t.card, gunim.PopupOptions{
		Anchor: geom.Rect{Min: t.at.Sub(geom.Pt(0, 4)), Max: t.at.Add(geom.Pt(0, 22))},
	})
}

func (t *tipper) hide(*gunim.UI) {
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
	if t.popup != nil {
		t.popup.Close()
		t.popup, t.card = nil, nil
	}
}

// Layout implements [gunim.Node]. The text set since the last layout
// turns over in the tooltip showing.
func (t *Tooltip) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	t.tip.retext(t.Text)
	k := kids.At(0)
	s := k.Layout(cs)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (t *Tooltip) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// tip is a tooltip's popup: a small card of text that fades in, and
// whose words turn over as they change: was, the words before, slide up
// and away as turn runs to 1, and text comes up under them.
type tip struct {
	text, was   string
	in, turn    *anim.Float
	run, wasRun shapedText
	margin      float32
	transparent bool
}

func newTip(text string) *tip {
	return &tip{text: text, in: anim.NewFloat(0), turn: anim.NewFloat(1)}
}

// turnTo turns the words over to s.
func (t *tip) turnTo(s string) {
	t.was, t.text = t.text, s
	t.turn.Jump(0)
	t.turn.Animate(1, anim.Spring{Response: 0.3, Damping: 0.85})
}

// Step implements [gunim.Animator].
func (t *tip) Step(dt time.Duration) bool {
	in := t.in.Step(dt)
	return t.turn.Step(dt) || in
}

// Transition implements [gunim.Transitioner].
func (t *tip) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		t.in.Animate(1, Quick.Get(f.Theme))
	case gunim.Exiting:
		t.in.Animate(0, Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !t.in.Active()
}

// PopupPadding implements [gunim.PopupPadder].
func (t *tip) PopupPadding() geom.Insets { return geom.Uniform(t.margin) }

// Layout implements [gunim.Node].
func (t *tip) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	t.transparent = f.Transparent
	t.margin = 0
	if f.Transparent {
		t.margin = MenuMargin.Get(th)
	}
	run := t.run.shape(faceIn(Font, th), t.text, TooltipSize.Get(th))
	pad := TooltipPadding.Get(th)
	// As wide as the wider of the words, while they turn over.
	w := run.Advance
	if t.turn.Value() < 1 {
		w = max(w, t.wasRun.shape(faceIn(Font, th), t.was, TooltipSize.Get(th)).Advance)
	}
	return c.Constrain(geom.Sz(w+pad.Left+pad.Right+2*t.margin, run.Height()+pad.Top+pad.Bottom+2*t.margin))
}

// Paint implements [gunim.Node].
func (t *tip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	pad := TooltipPadding.Get(th)
	card := geom.Rect{Min: geom.Pt(t.margin, t.margin), Max: geom.Pt(box.W-t.margin, box.H-t.margin)}
	// Turning over, the card glides from the width of the words before
	// to the width of the new.
	turn := min(max(t.turn.Value(), 0), 1)
	if turn < 1 {
		from, to := t.wasRun.run.Advance, t.run.run.Advance
		card.Max.X = card.Min.X + from + (to-from)*turn + pad.Left + pad.Right
	}
	if t.transparent {
		radius := TooltipRadius.Get(th)
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(1, max(0, t.in.Value()))})()
		p.ShadowRRect(card, radius, paint.Solid(TooltipFill.Get(th)), paint.Shadow{
			Offset: geom.Pt(0, 2), Blur: t.margin * 0.5, Color: MenuShadow.Get(th),
		})
	} else {
		p.RRect(card, 0, paint.Solid(TooltipFill.Get(th)))
	}
	at, ink := card.Min.Add(geom.Pt(pad.Left, pad.Top)), TooltipInk.Get(th)
	if turn >= 1 {
		t.run.run.Paint(p, at, ink)
		return
	}
	// The words turning over, cut to the card.
	defer p.Layer(paint.LayerOpts{Bounds: card, Opacity: 1, Clip: true})()
	h := t.run.run.Height()
	t.wasRun.run.Paint(p, at.Sub(geom.Pt(0, h*turn)), fadedBy(ink, 1-turn))
	t.run.run.Paint(p, at.Add(geom.Pt(0, h*(1-turn))), fadedBy(ink, turn))
}
