package plugui

import (
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// greyFaint is how far a greyed control of this package fades, as a share of [widget.Faint]'s way.
const greyFaint = 0.6

// greying is what a control of this package keeps as it greys: how far it has faded, and where the pointer is.
type greying struct {
	// dim runs from 0 to 1 as the control greys, laid says it has been laid out, and over says the pointer is on it.
	dim        *anim.Float
	laid, over bool
}

// newGreying returns a greying at rest, ungreyed.
func newGreying() greying { return greying{dim: anim.NewFloat(0)} }

// follow glides dim to grey from Layout, jumping there on the first layout, and reports whether grey changed since
// the layout before.
func (g *greying) follow(grey bool, th *theme.Live) bool {
	to := anim.OnOff(grey)
	if !g.laid {
		g.laid = true
		g.dim.Jump(to)
		return false
	}
	if g.dim.Target() == to {
		return false
	}
	g.dim.Animate(to, widget.Quick.Get(th))
	return true
}

// track follows the pointer coming over the control and leaving it.
func (g *greying) track(e input.Event) {
	switch e.(type) {
	case input.PointerEnter:
		g.over = true
	case input.PointerLeave:
		g.over = false
	}
}

// value is how far the control has greyed, from 0 to 1.
func (g *greying) value() float32 { return min(max(g.dim.Value(), 0), 1) }

// faint draws what follows as faint as the control has greyed, and returns what ends it.
func (g *greying) faint(p *paint.Painter, box geom.Size) func() {
	return widget.Faint(p, box, g.value()*greyFaint)
}

// ring is how far a focus ring at ring shows on the control, fading out as it greys.
func (g *greying) ring(ring float32) float32 { return ring * (1 - g.value()) }

// handleGreyed is what a greyed control does with e, as a disabled [widget.Control] does: it takes the primary
// button's presses, so a click on it reaches nothing round it, and passes every other event by.
func handleGreyed(e input.Event) bool {
	switch e := e.(type) {
	case input.PointerDown:
		return e.Button == input.ButtonPrimary
	case input.PointerUp:
		return e.Button == input.ButtonPrimary
	}
	return false
}
