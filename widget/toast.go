package widget

import (
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Toast tokens.
var (
	ToastWidth = theme.Length("toast.width", 340)
	ToastGap   = theme.Length("toast.gap", 10)
)

// toastLife is how long a toast stays when the pointer leaves it alone.
const toastLife = 5 * time.Second

// Toast is a short notice: a title, and a line or two under it.
type Toast struct {
	Title string
	Body  string
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

// Show adds a toast to the stack.
func (t *Toasts) Show(to Toast, u *gunim.UI) {
	c := newToastCard(t, to)
	t.cards = append(t.cards, c)
	u.Insert(t, c)
	t.expire(c, u)
	u.Invalidate()
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
	owner   *Toasts
	title   *Label
	body    *Label
	in      *anim.Float
	y       *anim.Float
	hover   *anim.Float
	hovered bool
	placed  bool
}

func newToastCard(t *Toasts, to Toast) *toastCard {
	c := &toastCard{owner: t, title: NewLabel(to.Title), body: NewLabel(to.Body),
		in: anim.NewFloat(0), y: anim.NewFloat(0), hover: anim.NewFloat(0)}
	c.body.Color = PaletteHint
	c.Add(c.in, c.y, c.hover)
	return c
}

// Children implements [gunim.Composite].
func (c *toastCard) Children() []gunim.Node {
	if c.body.Text == "" {
		return []gunim.Node{c.title}
	}
	return []gunim.Node{c.title, c.body}
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

// Layout implements [gunim.Node].
func (c *toastCard) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	pad := CardPadding.Get(f.Theme)
	w := cs.Max.W
	y := pad.Top
	for k := range kids.All {
		s := k.Layout(gunim.Constraints{Max: geom.Sz(w-pad.Left-pad.Right, 0)})
		k.Place(geom.Pt(pad.Left, y))
		y += s.H + 4
	}
	return geom.Sz(w, y-4+pad.Bottom)
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
// toast, and a click dismisses it.
func (c *toastCard) Handle(e input.Event, u *gunim.UI) bool {
	switch e.(type) {
	case input.PointerEnter:
		c.hovered = true
		c.hover.Animate(1, Quick.Get(u.Theme()))
	case input.PointerLeave:
		c.hovered = false
		c.hover.Animate(0, Settle.Get(u.Theme()))
	case input.PointerDown:
		c.owner.dismiss(c, u)
	case input.PointerUp:
	default:
		return false
	}
	return true
}
