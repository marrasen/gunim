package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// Menu is a list of items to pick from, shown in a popup: the list a
// [Dropdown] opens and the menu a [ContextMenu] opens.
//
// A highlight glides to the item under the pointer, or to the one the
// arrow keys reach. The menu fades and unfolds from its top as it
// opens, and fades as it closes. Where the display server cannot blend
// windows, the menu fills its window and appears at once.
type Menu struct {
	anim.Group

	Items []string
	// Pick runs on the UI goroutine with the index of the item picked.
	Pick func(i int, u *gunim.UI)
	// MinWidth is the narrowest the menu gets, so a drop-down's list is
	// at least as wide as the drop-down.
	MinWidth float32

	hot   int
	glide bool
	hotY  *anim.Float
	hotOn *anim.Float
	in    *anim.Float

	rows []shapedText
	// card is where the menu is drawn in its box, row and pad the row
	// height and the space around the rows, all from the last layout.
	card        geom.Rect
	margin      float32
	row, pad    float32
	transparent bool
}

// NewMenu returns a menu of items, with nothing highlighted.
func NewMenu(items ...string) *Menu {
	m := &Menu{
		Items: items,
		hot:   -1,
		hotY:  anim.NewFloat(0),
		hotOn: anim.NewFloat(0),
		in:    anim.NewFloat(0),
	}
	m.Add(m.hotY, m.hotOn, m.in)
	return m
}

// Highlight moves the highlight to item i, gliding from where it was.
// A negative i takes it away.
func (m *Menu) Highlight(i int) {
	if i >= len(m.Items) {
		i = len(m.Items) - 1
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
	m.hotY.Animate(m.rowY(i), anim.Snappy)
}

// Highlighted returns the highlighted item, or -1.
func (m *Menu) Highlighted() int { return m.hot }

// Key moves the highlight with the arrow keys, Home and End, and picks
// the highlighted item with Enter or Space. It is for the node that
// opened the menu, which keeps the keyboard, to pass keys on. It
// reports whether it used the key.
func (m *Menu) Key(k input.KeyPress, u *gunim.UI) bool {
	switch k.Key {
	case input.KeyDown:
		m.Highlight(min(m.hot+1, len(m.Items)-1))
	case input.KeyUp:
		m.Highlight(max(m.hot-1, 0))
	case input.KeyHome:
		m.Highlight(0)
	case input.KeyEnd:
		m.Highlight(len(m.Items) - 1)
	case input.KeyEnter, input.KeySpace:
		if m.hot >= 0 && m.Pick != nil {
			m.Pick(m.hot, u)
		}
	default:
		return false
	}
	return true
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
	switch e := e.(type) {
	case input.PointerEnter:
		m.Highlight(m.rowAt(e.Pos))
	case input.PointerMove:
		if i := m.rowAt(e.Pos); i >= 0 {
			m.Highlight(i)
		}
	case input.PointerDown:
	case input.PointerUp:
		if i := m.rowAt(e.Pos); i >= 0 && m.Pick != nil {
			m.Pick(i, u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// rowAt returns the item at p, in the menu's space, or -1.
func (m *Menu) rowAt(p geom.Point) int {
	if !m.card.Contains(p) || m.row <= 0 {
		return -1
	}
	i := int((p.Y - m.card.Min.Y - m.pad) / m.row)
	if i < 0 || i >= len(m.Items) {
		return -1
	}
	return i
}

// rowY returns the top of row i in the menu's space.
func (m *Menu) rowY(i int) float32 { return m.card.Min.Y + m.pad + float32(i)*m.row }

// Layout implements [gunim.Node].
func (m *Menu) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	m.transparent = f.Transparent
	m.margin = 0
	if f.Transparent {
		m.margin = MenuMargin.Get(th)
	}
	m.row, m.pad = MenuRowHeight.Get(th), MenuPadding.Get(th)
	if len(m.rows) != len(m.Items) {
		m.rows = make([]shapedText, len(m.Items))
	}
	size := TextSize.Get(th)
	w := m.MinWidth
	for i, s := range m.Items {
		w = max(w, m.rows[i].shape(s, size).Advance+2*MenuRowPadding.Get(th))
	}
	h := float32(len(m.Items))*m.row + 2*m.pad
	m.card = geom.Rc(m.margin, m.margin, w, h)
	if m.hot >= 0 && !m.glide {
		m.hotY.Jump(m.rowY(m.hot))
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
	if m.transparent {
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
	} else {
		radius = 0
		p.RRect(card, 0, paint.Solid(MenuFill.Get(th)))
	}
	p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})

	if on := m.hotOn.Value(); on > 0.01 {
		c := MenuHot.Get(th)
		c.A = uint8(float32(c.A) * min(on, 1))
		inset := m.pad
		p.RRect(geom.Rc(card.Min.X+inset, m.hotY.Value(), card.Size().W-2*inset, m.row), max(0, radius-inset), paint.Solid(c))
	}
	ink := Ink.Get(th)
	for i := range m.Items {
		run := m.rows[i].run
		y := m.rowY(i) + (m.row-run.Height())/2
		run.Paint(p, geom.Pt(card.Min.X+MenuRowPadding.Get(th), y), ink)
	}
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
	// OnChange turns a new choice into an intent for the application.
	OnChange func(i int) gunim.Intent

	hover *anim.Float
	ring  *anim.Float
	turn  *anim.Float

	popup *gunim.Popup
	menu  *Menu
	size  geom.Size
	label shapedText
}

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
func (d *Dropdown) Focusable() bool { return true }

// IsOpen reports whether the list is open.
func (d *Dropdown) IsOpen() bool { return d.popup != nil && d.popup.Open() }

// Handle implements [gunim.Handler].
func (d *Dropdown) Handle(e input.Event, u *gunim.UI) bool {
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
	case input.FocusGained:
		d.ring.Animate(1, Quick.Get(th))
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
		case input.KeySpace, input.KeyEnter, input.KeyDown, input.KeyUp:
			d.open(u)
			return true
		default:
			return false
		}
	}
	switch k.Key {
	case input.KeyEscape:
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
	m.MinWidth = d.size.W
	m.Highlight(d.Selected)
	m.Pick = func(i int, u *gunim.UI) {
		d.close(u)
		if i != d.Selected {
			d.Selected = i
			if d.OnChange != nil {
				u.Send(d, d.OnChange(i))
			}
		}
	}
	d.menu = m
	d.popup = u.OpenPopup(d, m, gunim.PopupOptions{
		Anchor:  geom.Rect{Max: d.size.Point()},
		Max:     geom.Sz(600, 480),
		Dismiss: d.close,
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
		w = max(w, probe.shape(s, TextSize.Get(th)).Advance)
	}
	pad := FieldPadding.Get(th)
	d.size = c.Constrain(geom.Sz(w+2*pad+chevron+pad, FieldHeight.Get(th)))
	return d.size
}

// chevron is the width of the drop-down's arrow.
const chevron = 10

// Paint implements [gunim.Node].
func (d *Dropdown) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
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
		run := d.label.shape(d.Items[d.Selected], TextSize.Get(th))
		run.Paint(p, geom.Pt(pad, (box.H-run.Height())/2), Ink.Get(th))
	}

	// The chevron turns over as the list opens.
	c := geom.Pt(box.W-pad-chevron/2, box.H/2)
	defer p.Push(paint.Rotate(math.Pi*d.turn.Value(), c))()
	drawChevron(p, c, Ink.Get(th))
}

// drawChevron draws a small downward arrow centred on c, as two bars
// turned 45 degrees each way.
func drawChevron(p *paint.Painter, c geom.Point, ink color.NRGBA) {
	const half, thick = 4, 1.6
	arm := float32(half * math.Sqrt2)
	for _, s := range []float32{-1, 1} {
		mid := geom.Pt(c.X+s*half/2, c.Y)
		closeArm := p.Push(paint.Rotate(-s*math.Pi/4, mid))
		bar := geom.Rect{Min: geom.Pt(mid.X-arm/2-thick/2, mid.Y-thick/2), Max: geom.Pt(mid.X+arm/2+thick/2, mid.Y+thick/2)}
		p.RRect(bar, thick/2, paint.Solid(ink))
		closeArm()
	}
}

// ContextMenu opens a menu at the pointer when its child is clicked
// with the secondary button.
//
// While the menu is open, the context menu holds the keyboard and
// passes keys to the menu, and hands focus back once it closes.
type ContextMenu struct {
	Items []string
	// OnPick turns a picked item into an intent for the application.
	OnPick func(i int) gunim.Intent

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
		c.open(e.Pos, u)
	case input.KeyPress:
		if !c.Focusable() {
			return false
		}
		switch e.Key {
		case input.KeyEscape, input.KeyTab:
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

func (c *ContextMenu) open(at geom.Point, u *gunim.UI) {
	c.close(u)
	m := NewMenu(c.Items...)
	m.Pick = func(i int, u *gunim.UI) {
		c.close(u)
		if c.OnPick != nil {
			u.Send(c, c.OnPick(i))
		}
	}
	c.menu = m
	c.popup = u.OpenPopup(c, m, gunim.PopupOptions{
		Anchor:  geom.Rect{Min: at, Max: at},
		Max:     geom.Sz(600, 480),
		Dismiss: c.close,
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
// pointer leaves.
type Tooltip struct {
	Text  string
	Delay time.Duration

	child gunim.Node
	at    geom.Point
	stop  func()
	popup *gunim.Popup
}

// NewTooltip returns child with a tooltip of s.
func NewTooltip(child gunim.Node, s string) *Tooltip {
	return &Tooltip{Text: s, Delay: 600 * time.Millisecond, child: child}
}

// Children implements [gunim.Composite].
func (t *Tooltip) Children() []gunim.Node { return []gunim.Node{t.child} }

// Handle implements [gunim.Handler].
func (t *Tooltip) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		t.at = e.Pos
		t.wait(u)
	case input.PointerMove:
		t.at = e.Pos
		if t.popup == nil {
			t.wait(u) // the delay starts again while the pointer moves
		}
	case input.PointerLeave, input.PointerDown, input.Scroll:
		t.hide(u)
	}
	return false
}

func (t *Tooltip) wait(u *gunim.UI) {
	if t.stop != nil {
		t.stop()
	}
	t.stop = u.After(t.Delay, t.show)
}

func (t *Tooltip) show(u *gunim.UI) {
	t.stop = nil
	if t.popup != nil {
		return
	}
	// Below the pointer, clear of it, or above it where the screen runs
	// out.
	t.popup = u.OpenPopup(t, &tip{text: t.Text, in: anim.NewFloat(0)}, gunim.PopupOptions{
		Anchor: geom.Rect{Min: t.at.Sub(geom.Pt(0, 4)), Max: t.at.Add(geom.Pt(0, 22))},
	})
}

func (t *Tooltip) hide(*gunim.UI) {
	if t.stop != nil {
		t.stop()
		t.stop = nil
	}
	if t.popup != nil {
		t.popup.Close()
		t.popup = nil
	}
}

// Layout implements [gunim.Node].
func (t *Tooltip) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	s := k.Layout(cs)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (t *Tooltip) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// tip is a tooltip's popup: a small card of text that fades in.
type tip struct {
	text        string
	in          *anim.Float
	run         shapedText
	margin      float32
	transparent bool
}

// Step implements [gunim.Animator].
func (t *tip) Step(dt time.Duration) bool { return t.in.Step(dt) }

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
	run := t.run.shape(t.text, TooltipSize.Get(th))
	pad := TooltipPadding.Get(th)
	return c.Constrain(geom.Sz(run.Advance+pad.Left+pad.Right+2*t.margin, run.Height()+pad.Top+pad.Bottom+2*t.margin))
}

// Paint implements [gunim.Node].
func (t *tip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	card := geom.Rect{Min: geom.Pt(t.margin, t.margin), Max: geom.Pt(box.W-t.margin, box.H-t.margin)}
	if t.transparent {
		radius := TooltipRadius.Get(th)
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(1, max(0, t.in.Value()))})()
		p.ShadowRRect(card, radius, paint.Solid(TooltipFill.Get(th)), paint.Shadow{
			Offset: geom.Pt(0, 2), Blur: t.margin * 0.5, Color: MenuShadow.Get(th),
		})
	} else {
		p.RRect(card, 0, paint.Solid(TooltipFill.Get(th)))
	}
	pad := TooltipPadding.Get(th)
	run := t.run.run
	run.Paint(p, card.Min.Add(geom.Pt(pad.Left, pad.Top)), TooltipInk.Get(th))
}
