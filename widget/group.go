package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// GroupRadius is the corner radius of the ring round a [Group] that has the keyboard.
var GroupRadius = theme.Length("group.radius", 8)

// Group makes the controls it holds one stop for Tab: Tab lands on the one that last had the keyboard, and the arrow
// keys along Axis move between them, once the control with the keyboard gives them up at its end. While the keyboard
// is in the group, a ring shows round all of it.
type Group struct {
	anim.Group
	// Axis is the way the arrow keys move: Up and Down for Vertical, Left and Right for Horizontal.
	Axis  Axis
	child gunim.Node
	ring  *anim.Float
}

// NewGroup returns a group round child, whose arrow keys move along axis.
func NewGroup(axis Axis, child gunim.Node) *Group {
	g := &Group{Axis: axis, child: child, ring: anim.NewFloat(0)}
	g.Add(g.ring)
	return g
}

// TabGroup implements [gunim.TabGroup].
func (g *Group) TabGroup() {}

// Children implements [gunim.Composite].
func (g *Group) Children() []gunim.Node { return []gunim.Node{g.child} }

// Layout implements [gunim.Node]: the child fills the group.
func (g *Group) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	size := k.Layout(c)
	k.Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node]: the child, and the ring over it.
func (g *Group) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	GroupRing(p, geom.Rect{Max: box.Point()}, GroupRadius.Get(f.Theme), g.ring.Value(), f.Theme)
}

// Handle implements [gunim.Handler]: the ring follows the keyboard, and the arrow keys a control gave up move on to
// the next control.
func (g *Group) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.FocusRing:
		g.ring.Animate(ringTo(e), RingFade.Get(u.Theme()))
		u.Invalidate()
		return true
	case input.KeyPress:
		if e.Mods != 0 {
			return false
		}
		back, on := input.KeyLeft, input.KeyRight
		if g.Axis == Vertical {
			back, on = input.KeyUp, input.KeyDown
		}
		switch e.Key {
		case back, on:
			return u.FocusWithin(g, e.Key == on)
		default:
			return false
		}
	default:
		return false
	}
}
