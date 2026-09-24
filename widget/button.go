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

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Button is a labelled, clickable rectangle.
//
// Every visible reaction it has is a spring. Hovering warms the fill,
// pressing squashes it slightly, focusing grows a ring around it. All
// three can be part way through at once and stay independent, because
// each is its own value with its own motion.
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

	fill  *anim.Color
	press *anim.Float
	ring  *anim.Float
	held  bool
}

// Colours. A real theme belongs elsewhere; these keep the example
// readable.
var (
	buttonIdle  = color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff}
	buttonHover = color.NRGBA{R: 0x3d, G: 0x45, B: 0x58, A: 0xff}
	buttonText  = color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff}
	accent      = color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff}
)

// NewButton returns a button showing label.
func NewButton(label string) *Button {
	b := &Button{
		Label: label,
		fill:  anim.NewColor(buttonIdle),
		press: anim.NewFloat(0),
		ring:  anim.NewFloat(0),
	}
	b.Add(b.fill, b.press, b.ring)
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
func (b *Button) Handle(e gunim.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case gunim.PointerEnter:
		b.fill.Animate(buttonHover, anim.Snappy)
	case gunim.PointerLeave:
		b.fill.Animate(buttonIdle, anim.Gentle)
		// Releasing outside the button cancels the press, and the
		// squash springs back on its own.
		b.held = false
		b.press.Animate(0, anim.Bouncy)
	case gunim.PointerDown:
		b.held = true
		b.press.Animate(1, anim.Snappy)
	case gunim.PointerUp:
		if !b.held {
			return false
		}
		b.held = false
		b.press.Animate(0, anim.Bouncy)
		b.fire(u)
	case gunim.KeyPress:
		if e.Key != gunim.KeySpace && e.Key != gunim.KeyEnter {
			return false
		}
		// Keyboard activation runs the same squash, so the button
		// looks pressed however it was reached.
		b.press.Retarget(1, anim.Snappy)
		b.press.Animate(0, anim.Bouncy)
		b.fire(u)
	case gunim.FocusGained:
		b.ring.Animate(1, anim.Snappy)
	case gunim.FocusLost:
		b.ring.Animate(0, anim.Gentle)
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
func (b *Button) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	// Placeholder metrics. Real measurement means shaping the label with
	// the theme's face, which is the text package's job.
	const perRune, padding, height = 8.5, 32, 36
	return c.Constrain(geom.Sz(float32(len(b.Label))*perRune+padding, height))
}

// Paint implements [gunim.Node].
func (b *Button) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rect{Max: box.Point()}

	// Squash toward the centre while held. Scaling about the middle is
	// what makes it read as a press.
	if s := 1 - 0.035*b.press.Value(); s != 1 {
		defer p.Push(paint.Scale(s, r.Center()))()
	}

	// The focus ring grows outward from the button's edge.
	if t := b.ring.Value(); t > 0 {
		ring := accent
		ring.A = uint8(0x90 * t)
		grow := 3 * t
		p.RRectStroke(
			geom.Rect{Min: geom.Pt(r.Min.X-grow, r.Min.Y-grow), Max: geom.Pt(r.Max.X+grow, r.Max.Y+grow)},
			8+grow, paint.Fill{}, paint.Stroke{Width: 2, Color: ring},
		)
	}

	p.RRect(r, 8, paint.Solid(b.fill.Value()))
	p.Text(nil, buttonText, r) // glyphs come from the text package
}
