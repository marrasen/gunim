package widget

import (
	"image/color"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Toast tokens.
var (
	ToastWidth = theme.Length("toast.width", 340)
	ToastGap   = theme.Length("toast.gap", 10)
	// ToastInfoInk, ToastSuccessInk, ToastWarningInk and ToastErrorInk colour the icon of each kind of toast.
	ToastInfoInk    = theme.Color("toast.info", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff})
	ToastSuccessInk = theme.Color("toast.success", color.NRGBA{R: 0x4c, G: 0xc3, B: 0x8a, A: 0xff})
	ToastWarningInk = theme.Color("toast.warning", color.NRGBA{R: 0xe8, G: 0xb3, B: 0x4a, A: 0xff})
	ToastErrorInk   = theme.Color("toast.error", color.NRGBA{R: 0xff, G: 0x6b, B: 0x66, A: 0xff})
)

// toastLife is how long a toast stays when the pointer leaves it alone.
const toastLife = 5 * time.Second

// toastDrawOn is how long a toast's icon takes to draw itself on.
const toastDrawOn = 600 * time.Millisecond

// Toast is a short notice: a title, and a line or two under it.
type Toast struct {
	Title string
	Body  string
	// Action names a link on the toast, such as Undo, and On is the
	// intent a click on it sends. The toast goes once it is clicked.
	Action string
	On     gunim.Intent
	// Kind picks the icon before the title and its colour. The plain kind shows none.
	Kind ToastKind
	// Icon, when set, replaces the kind's icon. A plain toast shows it in the ink.
	Icon *icon.Icon
	// Key names the toast, so [Toasts.Close] can take it away. A toast
	// shown with the key of one showing takes its place.
	Key string
	// Buttons are buttons under the text, the first the one the toast
	// expects. A toast with buttons asks: it stays until one of them,
	// or its close button, is clicked, and a click elsewhere on it does
	// nothing. Neither it nor its buttons take the keyboard as they are
	// shown or clicked; Tab still reaches them.
	Buttons []ToastButton
	// Check, when not empty, labels a tick box above the buttons. The
	// intents of the buttons and of Dismiss hear whether it is ticked.
	Check string
	// Dismiss makes the intent the close button of a toast that asks
	// sends, told whether the tick box is ticked. Nil sends none.
	Dismiss func(checked bool) gunim.Intent
}

// ToastButton is a button of a [Toast] that asks.
type ToastButton struct {
	Label string
	// On makes the intent a click sends, told whether the toast's tick
	// box is ticked. Nil sends none. The toast goes once it is clicked.
	On func(checked bool) gunim.Intent
}

// ToastKind says what a [Toast] reports, and picks its icon and colour.
type ToastKind uint8

// The kinds of toast.
const (
	// ToastPlain shows no icon.
	ToastPlain ToastKind = iota
	// ToastInfo shows icon.Info in [ToastInfoInk].
	ToastInfo
	// ToastSuccess shows icon.CircleCheck in [ToastSuccessInk].
	ToastSuccess
	// ToastWarning shows icon.TriangleAlert in [ToastWarningInk].
	ToastWarning
	// ToastError shows icon.CircleAlert in [ToastErrorInk].
	ToastError
)

// look returns the kind's icon and colour, or nil for the plain kind.
func (k ToastKind) look() (*icon.Icon, theme.Token[color.NRGBA]) {
	switch k {
	case ToastInfo:
		return icon.Info, ToastInfoInk
	case ToastSuccess:
		return icon.CircleCheck, ToastSuccessInk
	case ToastWarning:
		return icon.TriangleAlert, ToastWarningInk
	case ToastError:
		return icon.CircleAlert, ToastErrorInk
	case ToastPlain:
	}
	return nil, Ink
}

// name is what a screen reader calls the kind's icon, so an error reads as one.
func (k ToastKind) name() string {
	switch k {
	case ToastInfo:
		return "Information"
	case ToastSuccess:
		return "Success"
	case ToastWarning:
		return "Warning"
	case ToastError:
		return "Error"
	case ToastPlain:
	}
	return ""
}

// Toasts shows short notices in a stack, the newest nearest the corner
// it sits in. Each slides in from the side and fades up, stays a few
// seconds, and slides away; the pointer resting on one keeps it, and a
// click dismisses it. As one leaves, the rest glide together.
//
// It takes the size of its stack, so the window behind it keeps its
// clicks: place it in a corner, as the last child of a window's root.
type Toasts struct {
	// Life is how long a toast stays; zero takes five seconds.
	Life time.Duration

	cards []*toastCard
}

// Show adds a toast to the stack. One with the key of a toast showing
// takes its place. A toast that asks stays until it is answered.
func (t *Toasts) Show(to Toast, u *gunim.UI) {
	if to.Key != "" {
		t.Close(to.Key, u)
	}
	c := newToastCard(t, to)
	t.cards = append(t.cards, c)
	u.Insert(t, c)
	if !c.asks() {
		t.expire(c, u)
	}
	u.Invalidate()
}

// Close takes away the toast showing with key, if one is, as though its
// time were up: it sends nothing.
func (t *Toasts) Close(key string, u *gunim.UI) {
	if key == "" {
		return
	}
	for _, c := range slices.Clone(t.cards) {
		if c.key == key {
			t.dismiss(c, u)
		}
	}
}

// Len returns how many toasts are showing.
func (t *Toasts) Len() int { return len(t.cards) }

// expire takes c away once its time is up, or later, while the pointer
// rests on it.
func (t *Toasts) expire(c *toastCard, u *gunim.UI) {
	life := t.Life
	if life <= 0 {
		life = toastLife
	}
	u.After(life, func(u *gunim.UI) {
		if c.hovered {
			t.expire(c, u)
			return
		}
		t.dismiss(c, u)
	})
}

func (t *Toasts) dismiss(c *toastCard, u *gunim.UI) {
	i := slices.Index(t.cards, c)
	if i < 0 {
		return
	}
	t.cards = slices.Delete(t.cards, i, i+1)
	u.Remove(c)
	u.Invalidate()
}

// Layout implements [gunim.Node]. The newest toast sits at the bottom,
// the rest above it; a toast on its way out keeps its place.
func (t *Toasts) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	w := min(ToastWidth.Get(th), max(c.Max.W, 1))
	gap := ToastGap.Get(th)
	sizes := map[gunim.Node]geom.Size{}
	for k := range kids.All {
		sizes[k.Node()] = k.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, 0)})
	}
	// Heights from the bottom up, for the toasts staying.
	total := float32(0)
	for i := len(t.cards) - 1; i >= 0; i-- {
		total += sizes[t.cards[i]].H
		if i > 0 {
			total += gap
		}
	}
	y := total
	motion := Settle.Get(th)
	for i := len(t.cards) - 1; i >= 0; i-- {
		card := t.cards[i]
		y -= sizes[card].H
		if !card.placed {
			card.y.Jump(y)
			card.placed = true
		}
		card.y.Animate(y, motion)
		y -= gap
	}
	height := total
	for k := range kids.All {
		if card, ok := k.Node().(*toastCard); ok {
			k.Place(geom.Pt(0, card.y.Value()))
			height = max(height, card.y.Value()+k.Size().H)
		}
	}
	return geom.Sz(w, max(height, 0))
}

// Paint implements [gunim.Node].
func (t *Toasts) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// toastCard is one toast.
type toastCard struct {
	anim.Group
	owner  *Toasts
	title  *Label
	body   *Label
	action *Link
	// mark is the kind's icon before the title, or nil.
	mark *Icon
	// key is the toast's key. A toast that asks has buttons, a close
	// button, and check, its tick box, or nil.
	key     string
	buttons *Flex
	close   *IconButton
	check   *Checkbox
	in      *anim.Float
	y       *anim.Float
	hover   *anim.Float
	hovered bool
	placed  bool
	// size is the card's size at its last layout, for telling a release
	// on it from one off it.
	size  geom.Size
	click clicker
}

func newToastCard(t *Toasts, to Toast) *toastCard {
	c := &toastCard{owner: t, title: NewLabel(to.Title), body: NewLabel(to.Body),
		in: anim.NewFloat(0), y: anim.NewFloat(0), hover: anim.NewFloat(0)}
	c.body.Color = PaletteHint
	c.Add(c.in, c.y, c.hover)
	if to.Action != "" {
		c.action = NewLink(to.Action)
		c.action.On = to.On
		c.action.OnActivate(func(u *gunim.UI) { t.dismiss(c, u) })
	}
	c.key = to.Key
	if len(to.Buttons) > 0 {
		c.ask(to)
	}
	ic, ink := to.Kind.look()
	if to.Icon != nil {
		ic = to.Icon
	}
	if ic != nil {
		c.mark = NewIcon(ic, to.Kind.name())
		c.mark.Color = ink
		c.mark.DrawOn(toastDrawOn)
	}
	return c
}

// ask gives the card the buttons, the close button and the tick box of
// a toast that asks. None of them takes the keyboard when clicked.
func (c *toastCard) ask(to Toast) {
	checked := func() bool { return c.check != nil && c.check.On }
	if to.Check != "" {
		c.check = NewCheckbox(to.Check)
		c.check.KeepFocus = true
	}
	kids := make([]gunim.Node, 0, len(to.Buttons))
	for i, tb := range to.Buttons {
		b := NewButton(tb.Label)
		b.KeepFocus = true
		if i == 0 {
			b.Kind = ButtonPrimary
		}
		on := tb.On
		b.OnActivate(func(u *gunim.UI) {
			if on != nil {
				if in := on(checked()); in != nil {
					u.Send(c, in)
				}
			}
			c.owner.dismiss(c, u)
		})
		kids = append(kids, b)
	}
	c.buttons = Row(kids...)
	c.buttons.Justify = JustifyEnd
	c.buttons.Gap = Gap
	c.close = NewIconButton(icon.X, "Close")
	c.close.KeepFocus = true
	dismiss := to.Dismiss
	c.close.OnActivate(func(u *gunim.UI) {
		if dismiss != nil {
			if in := dismiss(checked()); in != nil {
				u.Send(c, in)
			}
		}
		c.owner.dismiss(c, u)
	})
}

// asks reports whether the card is of a toast that asks.
func (c *toastCard) asks() bool { return c.buttons != nil }

// corner is the link or the button at the right of the title, or nil.
func (c *toastCard) corner() gunim.Node {
	switch {
	case c.close != nil:
		return c.close
	case c.action != nil:
		return c.action
	}
	return nil
}

// KeepsFocus implements [gunim.FocusKeeper]: a click on a toast leaves
// the keyboard where it is.
func (c *toastCard) KeepsFocus() {}

// Children implements [gunim.Composite].
func (c *toastCard) Children() []gunim.Node {
	out := []gunim.Node{c.title}
	if c.body.Text != "" {
		out = append(out, c.body)
	}
	if c.check != nil {
		out = append(out, c.check)
	}
	if c.buttons != nil {
		out = append(out, c.buttons)
	}
	if n := c.corner(); n != nil {
		out = append(out, n)
	}
	if c.mark != nil {
		out = append(out, c.mark)
	}
	return out
}

// Transition implements [gunim.Transitioner]: in from the side and up
// from clear, and back the way it came.
func (c *toastCard) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		c.in.Animate(1, Settle.Get(f.Theme))
	case gunim.Exiting:
		c.in.Animate(0, Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !c.in.Active()
}

// Layout implements [gunim.Node]. The icon sits before the title, and the text after it.
func (c *toastCard) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	pad := CardPadding.Get(th)
	w := cs.Max.W
	left := pad.Left
	if c.mark != nil {
		left += IconSize.Get(th) + IconGap.Get(th)
	}
	room := w - left - pad.Right
	// The action, or the close button, sits at the right of the title.
	var act, title geom.Size
	// line is the height of the title's first line, which the icon sits beside.
	var line float32
	y := pad.Top
	corner := c.corner()
	for k := range kids.All {
		if corner != nil && k.Node() == corner {
			act = k.Layout(gunim.Constraints{Max: geom.Sz(room, 0)})
			top := pad.Top
			if corner == gunim.Node(c.close) {
				// The close button's box is larger than its cross; it
				// keeps to the corner rather than push the title down.
				top = pad.Top / 2
			}
			k.Place(geom.Pt(w-pad.Right-act.W, top))
		}
	}
	for k := range kids.All {
		switch n := k.Node(); n {
		case corner, gunim.Node(c.mark):
		default:
			r := room
			if n == gunim.Node(c.title) && act.W > 0 {
				r = max(0, room-act.W-Gap.Get(th))
			}
			cs := gunim.Constraints{Max: geom.Sz(r, 0)}
			if c.buttons != nil && n == gunim.Node(c.buttons) {
				// The buttons sit at the right, under a little room.
				cs.Min.W = r
				y += 4
			}
			s := k.Layout(cs)
			k.Place(geom.Pt(left, y))
			if n == gunim.Node(c.title) {
				title = s
				line = c.title.paragraph(f, r).LineHeight
			}
			y += s.H + 4
		}
	}
	for k := range kids.All {
		if c.mark != nil && k.Node() == gunim.Node(c.mark) {
			s := k.Layout(gunim.Constraints{Max: geom.Sz(room, 0)})
			k.Place(geom.Pt(pad.Left, pad.Top+(min(line, title.H)-s.H)/2))
		}
	}
	c.size = geom.Sz(w, y-4+pad.Bottom)
	return c.size
}

// Paint implements [gunim.Node].
func (c *toastCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	t := min(max(c.in.Value(), 0), 1)
	if t <= 0.001 {
		return
	}
	r := geom.Rect{Max: box.Point()}
	defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(-24)), Opacity: t})()
	defer p.Push(paint.Translate(geom.Pt(48*(1-t), 0)))()
	shadow := DialogShadow.Get(th)
	fill := anim.Mix(anim.ColorCodec, DialogFill.Get(th), MenuFill.Get(th), min(max(c.hover.Value(), 0), 1))
	p.ShadowRRect(r, CardRadius.Get(th), paint.Solid(fill), paint.Shadow{Offset: geom.Pt(0, 4), Blur: 16, Color: shadow})
	p.RRectStroke(r, CardRadius.Get(th), paint.Fill{}, paint.Stroke{Width: 1, Color: DialogBorder.Get(th)})
	for k := range kids.All {
		k.Paint(p)
	}
}

// Handle implements [gunim.Handler]: the pointer resting keeps the
// toast, and a click dismisses it, unless it asks.
func (c *toastCard) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		c.hovered = true
		c.hover.Animate(1, Quick.Get(u.Theme()))
	case input.PointerLeave:
		c.hovered = false
		c.hover.Animate(0, Settle.Get(u.Theme()))
	case input.PointerDown:
		c.click.press(e, 0)
	case input.PointerUp:
		if c.click.release(e, over(e.Pos, c.size)) && !c.asks() {
			c.owner.dismiss(c, u)
		}
	default:
		return false
	}
	return true
}
