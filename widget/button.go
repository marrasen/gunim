// Package widget holds the widgets built on gunim's core.
//
// They are as much worked examples as they are a widget set: each one
// shows how the optional interfaces fit together. A widget animates by
// owning [anim] values and embedding [anim.Group]; it takes input by
// implementing [gunim.Handler]; it animates in and out by implementing
// [gunim.Transitioner].
package widget

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Button is a labelled, clickable rectangle.
//
// Every visible reaction it has is a spring. Hovering warms the fill,
// pressing squashes it slightly, focusing grows a ring around it. All
// three can be part way through at once and stay independent, because
// each is its own value with its own motion. Each animates a state from
// 0 to 1, and the look comes from the theme every frame, so a theme
// switch lands in the middle of a hover without a jolt.
type Button struct {
	// Group makes the button an Animator, so the engine steps its
	// values and knows to keep drawing while any of them is moving.
	anim.Group

	Label string
	// Icon shows before the label, in the label's colour. A button with an icon and no label is square.
	Icon *icon.Icon
	// IconSize, when set, is the icon's size in place of [IconSize]; a button with an icon and no label then fits
	// the icon with [IconPadding] around it.
	IconSize theme.Token[float32]
	// Ink, when set, colours the label and the icon in place of the kind's ink.
	Ink theme.Token[color.NRGBA]
	// Active shows the ink in [Accent], as a toggle that is on does, fading between the two.
	Active bool
	// Ghost leaves the fill clear until the pointer is over the button.
	Ghost bool
	// Disabled fades the button faint; it then takes no clicks, keys or focus.
	Disabled bool
	// KeepFocus leaves the keyboard where it is when the button is clicked, as a toolbar's buttons do; Tab still
	// reaches it.
	KeepFocus bool
	// Kind says how much the button stands out: plain, primary for the
	// action a dialog expects, or danger for one that destroys, such as
	// Delete.
	Kind ButtonKind
	// On is the intent sent to the application when the button is
	// activated. It travels as data, so the application can be a
	// goroutine or a process on another machine, and either way it
	// stays off the goroutine that draws the window.
	//
	// Leave it nil for a button whose whole job is local.
	On gunim.Intent

	// activate is local behaviour, set by [Button.OnActivate].
	activate func(*gunim.UI)

	hover *anim.Float
	press *anim.Float
	// size is the button's size at its last layout, for telling a
	// release over it from one outside.
	size geom.Size
	ring *anim.Float
	// lit runs from 0 to 1 as Active turns on.
	lit *anim.Float
	// tone fades the colours from kind was to kind is, so a primary or
	// danger button comes up from the plain button's colours.
	tone    *anim.Float
	was, is ButtonKind
	held    bool
	text    shapedText
	// dim runs from 0 to 1 as Disabled turns on.
	dim *anim.Float
	// over says the pointer is over the button, and laid that it has been laid out.
	over, laid bool
	// self is the node the button sends from, when it is part of a larger one.
	self gunim.Node
}

// NewButton returns a button showing label.
func NewButton(label string) *Button {
	b := &Button{
		Label: label,
		hover: anim.NewFloat(0),
		press: anim.NewFloat(0),
		ring:  anim.NewFloat(0),
		lit:   anim.NewFloat(0),
		tone:  anim.NewFloat(1),
		dim:   anim.NewFloat(0),
	}
	b.Add(b.hover, b.press, b.ring, b.lit, b.tone, b.dim)
	return b
}

// OnActivate wires behaviour that runs inside the window.
//
// It is for widget authors composing a larger widget: [Dialog] uses it
// to close itself when OK is clicked. Application code reaches a window
// through [gunim.Client], where everything is data, so the closure here
// stays on the UI side of the boundary by construction.
func (b *Button) OnActivate(fn func(*gunim.UI)) { b.activate = fn }

// SetLabel changes the label. Call it from a view's update function.
func (b *Button) SetLabel(label string) { b.Label = label }

// Handle implements [gunim.Handler].
func (b *Button) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	if b.Disabled {
		return b.handleDisabled(e, th)
	}
	switch e := e.(type) {
	case input.PointerEnter:
		b.over = true
		b.hover.Animate(1, Quick.Get(th))
		// Coming back while still held presses it again.
		if b.held {
			b.press.Animate(1, Quick.Get(th))
		}
	case input.PointerLeave:
		b.over = false
		b.hover.Animate(0, Settle.Get(th))
		// The button keeps the pointer while it is held. Leaving eases
		// the press off, and releasing out here cancels it.
		b.press.Animate(0, Bounce.Get(th))
	case input.PointerDown:
		b.held = true
		b.press.Animate(1, Quick.Get(th))
	case input.PointerUp:
		if !b.held {
			return false
		}
		b.held = false
		b.press.Animate(0, Bounce.Get(th))
		if (geom.Rect{Max: b.size.Point()}).Contains(e.Pos) {
			b.fire(u)
		}
	case input.KeyPress:
		if e.Key != input.KeySpace && e.Key != input.KeyEnter && e.Key != input.KeyKPEnter {
			return false
		}
		// Keyboard activation runs the same squash, so the button
		// looks pressed however it was reached.
		b.press.Retarget(1, Quick.Get(th))
		b.press.Animate(0, Bounce.Get(th))
		b.fire(u)
	case input.FocusRing:
		b.ring.Animate(ringTo(e), Quick.Get(th))
	case input.FocusLost:
		b.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	return true
}

// handleDisabled takes the pointer's presses and passes keys by, while the button is disabled.
func (b *Button) handleDisabled(e input.Event, th *theme.Live) bool {
	switch e.(type) {
	case input.PointerEnter:
		b.over = true
	case input.PointerLeave:
		b.over = false
	case input.PointerDown, input.PointerUp:
	case input.FocusLost:
		b.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	return true
}

// fire runs the local behaviour first and reports to the application
// second.
//
// That order is the point of the whole arrangement: a button that
// closes a dialog closes it on this frame, and the application hears
// about it whenever it gets round to reading. The interface stays fluid
// while the work queues up behind it.
func (b *Button) fire(u *gunim.UI) {
	if b.activate != nil {
		b.activate(u)
	}
	if b.On != nil {
		var from gunim.Node = b
		if b.self != nil {
			from = b.self
		}
		u.Send(from, b.On)
	}
}

// Layout implements [gunim.Node].
func (b *Button) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	h := ButtonHeight.Get(th)
	w := h
	switch {
	case b.Label != "" || b.Icon == nil:
		w = b.content(th) + 2*ButtonPadding.Get(th)
	case b.IconSize.Key() != "":
		w = b.iconSize(th) + 2*IconPadding.Get(th)
		h = w
	}
	if on := float32(0); b.Active != (b.lit.Target() == 1) {
		if b.Active {
			on = 1
		}
		b.lit.Animate(on, Quick.Get(th))
	}
	if off := value(b.Disabled); !b.laid {
		b.dim.Jump(off)
	} else if b.dim.Target() != off {
		b.dim.Animate(off, Quick.Get(th))
		b.held = false
		b.press.Animate(0, Settle.Get(th))
		b.hover.Animate(value(b.over && !b.Disabled), Quick.Get(th))
	}
	b.laid = true
	b.size = c.Constrain(geom.Sz(w, h))
	if b.Kind != b.is {
		b.was, b.is = b.is, b.Kind
		b.tone.Jump(0)
		b.tone.Animate(1, anim.Tween{Duration: toneTime})
	}
	return b.size
}

// Focusable implements [gunim.Focusable].
func (b *Button) Focusable() bool { return !b.Disabled }

// FocusOnPress implements [gunim.PressFocuser].
func (b *Button) FocusOnPress() bool { return !b.KeepFocus }

// Paint implements [gunim.Node].
func (b *Button) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := ButtonRadius.Get(th)

	// Squash toward the centre while held. Scaling about the middle is
	// what makes it read as a press.
	if s := 1 - ButtonSquash.Get(th)*b.press.Value(); s != 1 {
		defer p.Push(paint.Scale(s, r.Center()))()
	}

	// The focus ring grows outward from the button's edge.
	if t := b.ring.Value(); t > 0 {
		ring := Accent.Get(th)
		ring.A = uint8(float32(ring.A) * 0.56 * min(t, 1))
		grow := 3 * t
		p.RRectStroke(
			geom.Rect{Min: geom.Pt(r.Min.X-grow, r.Min.Y-grow), Max: geom.Pt(r.Max.X+grow, r.Max.Y+grow)},
			radius+grow, paint.Fill{}, paint.Stroke{Width: 2, Color: ring},
		)
	}

	fromRest, fromHover, fromInk := kindColours(b.was, th)
	rest, hover, ink := kindColours(b.is, th)
	t := min(max(b.tone.Value(), 0), 1)
	mix := func(from, to theme.Token[color.NRGBA]) color.NRGBA {
		return anim.Mix(anim.ColorCodec, from.Get(th), to.Get(th), t)
	}
	restFill, hoverFill := mix(fromRest, rest), mix(fromHover, hover)
	if b.Ghost {
		restFill = hoverFill
		restFill.A = 0
	}
	fill := anim.Mix(anim.ColorCodec, restFill, hoverFill, b.hover.Value())
	faint := 1 - 0.6*min(max(b.dim.Value(), 0), 1)
	fill.A = uint8(float32(fill.A) * faint)
	if shadow := ButtonShadow.Get(th); shadow.A > 0 && !b.Ghost {
		// Cast down and to the right, and pressed into it while held.
		off := 4 * (1 - min(max(b.press.Value(), 0), 1))
		shadow.A = uint8(float32(shadow.A) * faint)
		p.RRect(geom.Rect{Min: geom.Pt(r.Min.X+off, r.Min.Y+off), Max: geom.Pt(r.Max.X+off, r.Max.Y+off)}, min(radius, box.H/2), paint.Solid(shadow))
	}
	p.RRect(r, min(radius, box.H/2), paint.Solid(fill))
	inked := mix(fromInk, ink)
	if b.Ink.Key() != "" {
		inked = b.Ink.Get(th)
	}
	if lit := b.lit.Value(); lit > 0 {
		inked = anim.Mix(anim.ColorCodec, inked, Accent.Get(th), min(lit, 1))
	}
	inked.A = uint8(float32(inked.A) * faint)
	// Squeezed, the icon keeps to the button's left edge.
	x := max((box.W-b.content(th))/2, 0)
	if b.Icon != nil {
		s := b.iconSize(th)
		paintIcon(p, th, b.Icon, geom.Rc(x, (box.H-s)/2, s, s), inked, 1)
		x += s + IconGap.Get(th)
	}
	if b.Label != "" {
		run := b.text.shape(faceIn(Font, th), b.Label, TextSize.Get(th))
		run.Paint(p, geom.Pt(x, (box.H-run.Height())/2), inked)
	}
}

// iconSize is the size the button draws its icon at.
func (b *Button) iconSize(th *theme.Live) float32 {
	if b.IconSize.Key() != "" {
		return b.IconSize.Get(th)
	}
	return IconSize.Get(th)
}

// content is the width of the button's icon and label, side by side.
func (b *Button) content(th *theme.Live) float32 {
	w := float32(0)
	if b.Label != "" || b.Icon == nil {
		w = b.text.shape(faceIn(Font, th), b.Label, TextSize.Get(th)).Advance
	}
	if b.Icon != nil {
		w += b.iconSize(th)
		if b.Label != "" {
			w += IconGap.Get(th)
		}
	}
	return w
}

// toneTime is how long a button takes to fade into a new kind's colours.
const toneTime = 350 * time.Millisecond

// kindColours returns the rest fill, hover fill and ink of a kind.
func kindColours(k ButtonKind, th *theme.Live) (rest, hover, ink theme.Token[color.NRGBA]) {
	switch k {
	case ButtonPrimary:
		// Its own ink where the theme sets one, and the strong ink a
		// theme may have set for both before there was one.
		if th.Sets(ButtonPrimaryInk.Key()) {
			return ButtonPrimaryFill, ButtonPrimaryHover, ButtonPrimaryInk
		}
		return ButtonPrimaryFill, ButtonPrimaryHover, ButtonStrongInk
	case ButtonDanger:
		return ButtonDangerFill, ButtonDangerHover, ButtonStrongInk
	case ButtonPlain:
	}
	return ButtonFill, ButtonHover, Ink
}

// ButtonKind says how much a [Button] stands out.
type ButtonKind uint8

// The kinds of button.
const (
	// ButtonPlain is the usual button.
	ButtonPlain ButtonKind = iota
	// ButtonPrimary is the action a dialog expects, in the accent colour.
	ButtonPrimary
	// ButtonDanger is an action that destroys, such as Delete, in red.
	ButtonDanger
)
