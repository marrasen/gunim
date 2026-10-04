package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Pad lays its child out inset by a padding token, the theme's [Margin]
// unless set otherwise.
type Pad struct {
	Padding theme.Token[geom.Insets]
	child   gunim.Node
}

// NewPad returns child inset by the theme's [Margin].
func NewPad(child gunim.Node) *Pad { return &Pad{Padding: Margin, child: child} }

// Children implements [gunim.Composite].
func (p *Pad) Children() []gunim.Node { return []gunim.Node{p.child} }

// Layout implements [gunim.Node].
func (p *Pad) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	return inset(c, p.Padding.Get(f.Theme), kids)
}

// Paint implements [gunim.Node].
func (p *Pad) Paint(pt *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(pt)
}

// inset lays the first child out within c less in, places it inside
// the insets, and returns the size around it.
func inset(c gunim.Constraints, in geom.Insets, kids gunim.Children) geom.Size {
	dw, dh := in.Left+in.Right, in.Top+in.Bottom
	shrink := func(v, by float32) float32 {
		if v <= 0 {
			return v // unbounded stays unbounded
		}
		return max(0, v-by)
	}
	inner := gunim.Constraints{
		Min: geom.Sz(max(0, c.Min.W-dw), max(0, c.Min.H-dh)),
		Max: geom.Sz(shrink(c.Max.W, dw), shrink(c.Max.H, dh)),
	}
	kid := kids.At(0)
	s := kid.Layout(inner)
	kid.Place(geom.Pt(in.Left, in.Top))
	return c.Constrain(geom.Sz(s.W+dw, s.H+dh))
}

// Card is a rounded panel around its child, from the theme's
// [CardFill], [CardRadius] and [CardPadding].
type Card struct {
	child gunim.Node
	Fill  theme.Token[color.NRGBA]
}

// NewCard returns a card around child.
func NewCard(child gunim.Node) *Card { return &Card{child: child, Fill: CardFill} }

// Children implements [gunim.Composite].
func (c *Card) Children() []gunim.Node { return []gunim.Node{c.child} }

// Layout implements [gunim.Node].
func (c *Card) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	return inset(cs, CardPadding.Get(f.Theme), kids)
}

// Paint implements [gunim.Node].
func (c *Card) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, CardRadius.Get(f.Theme), paint.Solid(c.Fill.Get(f.Theme)))
	kids.At(0).Paint(p)
}

// Surface is a window's root: it fills the window with the theme's
// [Background] and stacks the mounted views over it, like [gunim.Box].
// The background runs to the window's edges; the views keep clear of
// what the system draws over them, [gunim.Frame.Safe], as a phone's
// status and navigation bars.
type Surface struct {
	gunim.Box
}

// NewSurface returns an empty surface.
func NewSurface() *Surface { return &Surface{} }

// Layout implements [gunim.Node].
func (s *Surface) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	inner := geom.Rect{Max: size.Point()}.Inset(s.Padding).Inset(f.Safe)
	for kid := range kids.All {
		kid.Layout(gunim.Tight(inner.Size()))
		kid.Place(inner.Min)
	}
	return size
}

// Paint implements [gunim.Node].
func (s *Surface) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(Background.Get(f.Theme)))
	s.Box.Paint(p, f, box, kids)
}
