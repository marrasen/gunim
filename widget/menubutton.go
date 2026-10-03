package widget

import (
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// MenuButton is a button that opens a menu below it. With StayOpen the
// menu stays open after a pick, and a pick ticks or unticks the item, for
// a set of choices where several can be on, such as the files a filter
// lets through.
type MenuButton struct {
	anim.Group

	Title string
	// Icon shows before the title.
	Icon *icon.Icon
	// Items, Hints, Checked, Breaks, Captions and Icons make the menu; see
	// [Menu].
	Items    []string
	Hints    []string
	Checked  []bool
	Breaks   []int
	Captions []int
	Icons    []*icon.Icon
	// StayOpen keeps the menu open after a pick, and flips the item's tick.
	StayOpen bool
	// Active draws the button's border in the accent colour, as when its
	// choices narrow something down.
	Active bool
	// OnPick turns the item picked into an intent.
	OnPick func(i int) gunim.Intent
	// Picked runs on the UI goroutine as an item is picked, for a pick
	// that does its work in the window, such as opening a dialog.
	Picked func(i int, u *gunim.UI)

	hover *anim.Float
	ring  *anim.Float
	turn  *anim.Float

	popup *gunim.Popup
	menu  *Menu
	size  geom.Size
	shown shapedText
}

// NewMenuButton returns a button titled title that opens a menu of items.
func NewMenuButton(title string, items ...string) *MenuButton {
	b := &MenuButton{Title: title, Items: items, hover: anim.NewFloat(0), ring: anim.NewFloat(0), turn: anim.NewFloat(0)}
	b.Add(b.hover, b.ring, b.turn)
	return b
}

// Focusable implements [gunim.Focusable].
func (b *MenuButton) Focusable() bool { return true }

// IsOpen reports whether the menu is open.
func (b *MenuButton) IsOpen() bool { return b.popup != nil && b.popup.Open() }

// Handle implements [gunim.Handler].
func (b *MenuButton) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerEnter:
		b.hover.Animate(1, Quick.Get(th))
	case input.PointerLeave:
		b.hover.Animate(0, Settle.Get(th))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		b.toggle(u)
	case input.PointerUp:
	case input.FocusRing:
		b.ring.Animate(ringTo(e), Quick.Get(th))
	case input.FocusLost:
		b.ring.Animate(0, Settle.Get(th))
		b.close(u)
	case input.KeyPress:
		return b.key(e, u)
	default:
		return false
	}
	return true
}

func (b *MenuButton) toggle(u *gunim.UI) {
	if b.IsOpen() {
		b.close(u)
	} else {
		b.open(u)
	}
}

func (b *MenuButton) key(k input.KeyPress, u *gunim.UI) bool {
	if !b.IsOpen() {
		switch k.Key {
		case input.KeySpace, input.KeyEnter, input.KeyKPEnter, input.KeyDown:
			b.open(u)
			return true
		default:
			return false
		}
	}
	switch k.Key {
	case input.KeyEscape:
		u.Cue(gunim.CueClose, b)
		b.close(u)
		return true
	case input.KeyTab:
		b.close(u)
		return false
	default:
		return b.menu.Key(k, u)
	}
}

func (b *MenuButton) open(u *gunim.UI) {
	m := NewMenu()
	b.sync(m)
	m.MinWidth = b.size.W
	m.Pick = func(i int, u *gunim.UI) {
		if b.StayOpen {
			for len(b.Checked) <= i {
				b.Checked = append(b.Checked, false)
			}
			b.Checked[i] = !b.Checked[i]
			b.sync(m)
		} else {
			b.close(u)
		}
		if b.OnPick != nil {
			if v := b.OnPick(i); v != nil {
				u.Send(b, v)
			}
		}
		if b.Picked != nil {
			b.Picked(i, u)
		}
		u.Invalidate()
	}
	b.menu = m
	u.Cue(gunim.CueOpen, b)
	b.popup = u.OpenPopup(b, m, gunim.PopupOptions{
		Anchor:  geom.Rect{Max: b.size.Point()},
		Max:     geom.Sz(600, 560),
		Dismiss: dismissed(b, b.close),
	})
	b.turn.Animate(1, Quick.Get(u.Theme()))
}

// sync gives the open menu the button's items as they are now.
func (b *MenuButton) sync(m *Menu) {
	m.Items, m.Hints, m.Checked, m.Breaks, m.Captions, m.Icons = b.Items, b.Hints, b.Checked, b.Breaks, b.Captions, b.Icons
}

func (b *MenuButton) close(u *gunim.UI) {
	if b.popup != nil {
		b.popup.Close()
		b.popup = nil
	}
	b.turn.Animate(0, Quick.Get(u.Theme()))
}

// Layout implements [gunim.Node]. The button is as wide as its title.
func (b *MenuButton) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	if b.menu != nil && b.IsOpen() {
		b.sync(b.menu)
	}
	run := b.shown.shape(faceIn(Font, th), b.Title, TextSize.Get(th))
	pad := FieldPadding.Get(th)
	b.size = c.Constrain(geom.Sz(b.iconRoom(th)+run.Advance+2*pad+chevron+pad, FieldHeight.Get(th)))
	return b.size
}

// Paint implements [gunim.Node].
func (b *MenuButton) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := FieldRadius.Get(th)
	if t := b.ring.Value(); t > 0 {
		ring := Accent.Get(th)
		ring.A = uint8(float32(ring.A) * 0.56 * min(t, 1))
		grow := 3 * t
		p.RRectStroke(geom.Rect{Min: geom.Pt(-grow, -grow), Max: geom.Pt(box.W+grow, box.H+grow)},
			radius+grow, paint.Fill{}, paint.Stroke{Width: 2, Color: ring})
	}
	fill := anim.Mix(anim.ColorCodec, ButtonFill.Get(th), ButtonHover.Get(th), b.hover.Value())
	border := FieldBorder.Get(th)
	if b.Active {
		border = Accent.Get(th)
	}
	p.RRectStroke(r, radius, paint.Solid(fill), paint.Stroke{Width: 1, Color: border})
	pad := FieldPadding.Get(th)
	if b.Icon != nil {
		s := IconSize.Get(th)
		paintIcon(p, th, b.Icon, geom.Rc(pad, (box.H-s)/2, s, s), Ink.Get(th), 1)
	}
	run := b.shown.run
	run.Paint(p, geom.Pt(pad+b.iconRoom(th), (box.H-run.Height())/2), Ink.Get(th))
	c := geom.Pt(box.W-pad-chevron/2, box.H/2)
	defer p.Push(paint.Rotate(math.Pi*b.turn.Value(), c))()
	paintChevron(p, th, c, Ink.Get(th))
}

// iconRoom is the room the icon takes before the title, with its gap.
func (b *MenuButton) iconRoom(th *theme.Live) float32 {
	if b.Icon == nil {
		return 0
	}
	return IconSize.Get(th) + IconGap.Get(th)
}

// Access implements [gunim.Accessible].
func (b *MenuButton) Access() access.Info {
	info := access.Info{
		Role:    access.RoleButton,
		Name:    b.Title,
		State:   access.StateExpandable | access.StateHasPopup,
		Actions: []string{access.ActionPress},
	}
	if b.IsOpen() {
		info.State |= access.StateExpanded
	}
	if info.Name == "" {
		info.Name = iconName(b.Icon)
	}
	return info
}

// AccessAct implements [gunim.AccessActor].
func (b *MenuButton) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	b.toggle(u)
	return true
}
