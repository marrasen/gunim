// Package widget holds the widgets built on gunim's core.
//
// They are as much worked examples as they are a widget set: each one
// shows how the optional interfaces fit together. A widget animates by
// owning [anim] values and embedding [anim.Group]; it takes input by
// implementing [gunim.Handler]; it animates in and out by implementing
// [gunim.Transitioner].
package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
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
	held bool
	text shapedText
}

// NewButton returns a button showing label.
func NewButton(label string) *Button {
	b := &Button{
		Label: label,
		hover: anim.NewFloat(0),
		press: anim.NewFloat(0),
		ring:  anim.NewFloat(0),
	}
	b.Add(b.hover, b.press, b.ring)
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
	switch e := e.(type) {
	case input.PointerEnter:
		b.hover.Animate(1, Quick.Get(th))
		// Coming back while still held presses it again.
		if b.held {
			b.press.Animate(1, Quick.Get(th))
		}
	case input.PointerLeave:
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
		if e.Key != input.KeySpace && e.Key != input.KeyEnter {
			return false
		}
		// Keyboard activation runs the same squash, so the button
		// looks pressed however it was reached.
		b.press.Retarget(1, Quick.Get(th))
		b.press.Animate(0, Bounce.Get(th))
		b.fire(u)
	case input.FocusGained:
		b.ring.Animate(1, Quick.Get(th))
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
		u.Send(b, b.On)
	}
}

// Layout implements [gunim.Node].
func (b *Button) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	run := b.text.shape(b.Label, TextSize.Get(f.Theme))
	b.size = c.Constrain(geom.Sz(run.Advance+2*ButtonPadding.Get(f.Theme), ButtonHeight.Get(f.Theme)))
	return b.size
}

// Focusable implements [gunim.Focusable].
func (b *Button) Focusable() bool { return true }

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

	fill := anim.Mix(anim.ColorCodec, ButtonFill.Get(th), ButtonHover.Get(th), b.hover.Value())
	p.RRect(r, radius, paint.Solid(fill))
	run := b.text.shape(b.Label, TextSize.Get(th))
	run.Paint(p, geom.Pt((box.W-run.Advance)/2, (box.H-run.Height())/2), Ink.Get(th))
}
