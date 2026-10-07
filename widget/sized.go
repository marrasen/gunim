package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Sized gives its child a fixed width, a fixed height, or both, such as a
// column of labels that must line up. A zero Width or Height leaves that
// side to the child. The parent's limits come first: a Width wider than
// the parent allows shrinks to fit it.
type Sized struct {
	Width, Height float32
	child         gunim.Node
}

// NewSized returns child held to width and height.
func NewSized(child gunim.Node, width, height float32) *Sized {
	return &Sized{Width: width, Height: height, child: child}
}

// Children implements [gunim.Composite].
func (s *Sized) Children() []gunim.Node { return []gunim.Node{s.child} }

// Layout implements [gunim.Node].
func (s *Sized) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	want := c.Constrain(geom.Sz(s.Width, s.Height))
	cs := c
	if s.Width > 0 {
		cs.Min.W, cs.Max.W = want.W, want.W
	}
	if s.Height > 0 {
		cs.Min.H, cs.Max.H = want.H, want.H
	}
	kid := kids.At(0)
	size := kid.Layout(cs)
	kid.Place(geom.Point{})
	if s.Width > 0 {
		size.W = want.W
	}
	if s.Height > 0 {
		size.H = want.H
	}
	return c.Constrain(size)
}

// Paint implements [gunim.Node].
func (s *Sized) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}
