package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// popCard is the frame of a popup beside something in the calendar: a card width wide, with a shadow and a border
// as a menu has, that grows out of the side facing what it is beside as it opens, and shrinks back as it closes.
type popCard struct {
	anim.Group
	child gunim.Node
	width float32
	// left says the card opens to the left of what it is beside, so it grows from its right edge.
	left bool
	in   *anim.Float
	// margin is room round the card for its shadow, where the window can show one.
	margin float32
}

func newPopCard(child gunim.Node, width float32, left bool) popCard {
	c := popCard{child: child, width: width, left: left, in: anim.NewFloat(0)}
	c.Add(c.in)
	return c
}

// Children implements [gunim.Composite].
func (c *popCard) Children() []gunim.Node { return []gunim.Node{c.child} }

// PopupPadding implements [gunim.PopupPadder]: room for the shadow.
func (c *popCard) PopupPadding() geom.Insets { return geom.Uniform(c.margin) }

// Layout implements [gunim.Node].
func (c *popCard) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	c.margin = 0
	if f.Transparent {
		c.margin = widget.MenuMargin.Get(f.Theme)
	}
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(c.width, 0), Max: geom.Sz(c.width, max(cs.Max.H-2*c.margin, 0))})
	kid.Place(geom.Pt(c.margin, c.margin))
	return geom.Sz(s.W+2*c.margin, s.H+2*c.margin)
}

// Paint implements [gunim.Node].
func (c *popCard) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	t := min(max(c.in.Value(), 0), 1)
	card := geom.Rect{Min: geom.Pt(c.margin, c.margin), Max: geom.Pt(box.W-c.margin, box.H-c.margin)}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: t})()
	pivot := geom.Pt(card.Min.X, card.Min.Y+24)
	if c.left {
		pivot.X = card.Max.X
	}
	defer p.Push(paint.Scale(0.92+0.08*t, pivot))()
	radius := widget.MenuRadius.Get(th)
	if c.margin > 0 {
		p.ShadowRRect(card, radius, paint.Solid(widget.MenuFill.Get(th)), paint.Shadow{Offset: geom.Pt(0, 4),
			Blur: c.margin * 0.7, Color: widget.MenuShadow.Get(th)})
	} else {
		p.RRect(card, radius, paint.Solid(widget.MenuFill.Get(th)))
	}
	p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: widget.MenuBorder.Get(th)})
	kids.At(0).Paint(p)
}

// Transition implements [gunim.Transitioner].
func (c *popCard) Transition(pr gunim.Presence, f gunim.Frame) bool {
	switch pr {
	case gunim.Entering:
		c.in.Animate(1, widget.Bounce.Get(f.Theme))
	case gunim.Exiting:
		c.in.Animate(0, widget.Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !c.in.Active()
}
