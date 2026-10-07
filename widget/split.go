package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Split tokens.
var (
	// SplitGap is the room between a Split's panes, which the pointer
	// grabs to move the divider.
	SplitGap = theme.Length("split.gap", 6)
	// SplitLine and SplitHot are the divider's colour at rest, and
	// while the pointer is on it or drags it.
	SplitLine = theme.Color("split.line", color.NRGBA{R: 0x2c, G: 0x31, B: 0x3d, A: 0xff})
	SplitHot  = theme.Color("split.hot", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff})
)

// splitMin is the smallest a pane gets while the divider is dragged.
const splitMin = 48

// Split shows two panes side by side, or one above the other, with a
// divider between them that the pointer drags. Splits nest, for any
// arrangement of panes.
//
// The first pane's share of the space glides to where SetShare aims
// it, so a pane opened with a Split whose share starts at 1 slides in
// as the divider springs into place. At a share of 0 or 1 the divider
// narrows away and one pane has all the space. A double click on the
// divider evens the panes out.
type Split struct {
	anim.Group
	// Axis lays the panes side by side, which is the zero value, or one
	// above the other with [Vertical].
	Axis Axis
	// Fixed keeps the first pane's length, in logical pixels, as the
	// space changes, as a sidebar does. Share and SetShare then count
	// that length, where they otherwise count a share from 0 to 1.
	Fixed bool
	// Glide, when set, is the motion SetShare glides with, in place of
	// [Settle].
	Glide theme.Token[anim.Spring]
	// OnMove, when set, makes the intent sent once the pointer lets the
	// divider go, with the first pane's new share.
	OnMove func(share float32) gunim.Intent

	// laid is set by the first layout; a share set before it shows at once.
	laid          bool
	first, second gunim.Node
	share         *anim.Float
	hot           *anim.Float
	// held is set while the pointer drags the divider, grab being how
	// far into the gap it took hold.
	held bool
	grab float32
	over bool
	// length is the space along the split the last layout had, gap the
	// room the divider took, and own the split's size.
	length, gap float32
	own         geom.Size
}

// NewSplit returns first and second split evenly.
func NewSplit(first, second gunim.Node) *Split {
	s := &Split{first: first, second: second, share: anim.NewFloat(0.5), hot: anim.NewFloat(0)}
	s.Add(s.share, s.hot)
	return s
}

// Children implements [gunim.Composite].
func (s *Split) Children() []gunim.Node { return []gunim.Node{s.first, s.second} }

// Panes returns the two panes.
func (s *Split) Panes() (first, second gunim.Node) { return s.first, s.second }

// SetPane puts n in place of pane i, 0 for the first and 1 for the
// second. The pane it replaces leaves the way it animates out.
func (s *Split) SetPane(i int, n gunim.Node, u *gunim.UI) {
	old := &s.first
	if i == 1 {
		old = &s.second
	}
	if *old == n {
		return
	}
	u.Remove(*old)
	*old = n
	u.InsertAt(s, i, n)
}

// Held reports whether the pointer is dragging the divider.
func (s *Split) Held() bool { return s.held }

// Share returns the first pane's share of the space that the split is
// heading for, from 0 to 1.
func (s *Split) Share() float32 { return s.share.Target() }

// SetShare aims the first pane's share of the space at v, from 0 to 1,
// or its length with Fixed, and sends no intent. Once the split is laid
// out the divider glides there with Glide; before that, or with a nil u,
// it jumps.
func (s *Split) SetShare(v float32, u *gunim.UI) {
	if !s.Fixed {
		v = min(max(v, 0), 1)
	}
	v = max(v, 0)
	if !s.laid || u == nil {
		s.share.Jump(v)
		return
	}
	glide := Settle
	if s.Glide.Key() != "" {
		glide = s.Glide
	}
	s.share.Animate(v, glide.Get(u.Theme()))
	u.Invalidate()
}

func (s *Split) along(p geom.Point) float32 {
	if s.Axis == Vertical {
		return p.Y
	}
	return p.X
}

// fraction returns the first pane's share of the space, from 0 to 1.
func (s *Split) fraction() float32 {
	v := s.share.Value()
	if s.Fixed {
		if s.length <= 0 {
			return 0
		}
		v /= s.length
	}
	return min(max(v, 0), 1)
}

// room returns the gap the divider takes: all of it, or less as one
// pane nears the whole space.
func (s *Split) room(gap float32) float32 {
	v := s.fraction()
	return gap * min(max(min(v, 1-v)*20, 0), 1)
}

// firstLength returns the first pane's length along the split.
func (s *Split) firstLength() float32 {
	space := s.length - s.gap
	if s.Fixed {
		return float32(int(min(max(s.share.Value(), 0), space) + 0.5))
	}
	return float32(int(space*s.fraction() + 0.5))
}

// Layout implements [gunim.Node]. The split fills the space it is
// given.
func (s *Split) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	s.laid = true
	own := c.Max
	s.own = own
	s.length = own.W
	if s.Axis == Vertical {
		s.length = own.H
	}
	s.gap = s.room(SplitGap.Get(f.Theme))
	a := s.firstLength()
	b := max(0, s.length-s.gap-a)
	for kid := range kids.All {
		switch kid.Node() {
		case s.first:
			s.place(kid, 0, a, own)
		case s.second:
			s.place(kid, a+s.gap, b, own)
		default:
			// A pane on its way out keeps the first pane's place.
			s.place(kid, 0, a, own)
		}
	}
	return own
}

func (s *Split) place(kid gunim.Child, at, length float32, own geom.Size) {
	size := geom.Sz(length, own.H)
	pos := geom.Pt(at, 0)
	if s.Axis == Vertical {
		size, pos = geom.Sz(own.W, length), geom.Pt(0, at)
	}
	kid.Layout(gunim.Tight(size))
	kid.Place(pos)
}

// Paint implements [gunim.Node]. A pane with no room draws nothing.
func (s *Split) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		if sz := kid.Size(); sz.W >= 1 && sz.H >= 1 {
			kid.Paint(p)
		}
	}
	if s.gap < 0.5 {
		return
	}
	line := SplitLine.Get(f.Theme)
	if t := min(max(s.hot.Value(), 0), 1); t > 0 {
		line = mixColor(line, SplitHot.Get(f.Theme), t)
	}
	at := s.firstLength() + s.gap/2
	width := 1 + s.hot.Value()
	r := geom.Rc(at-width/2, 0, width, box.H)
	if s.Axis == Vertical {
		r = geom.Rc(0, at-width/2, box.W, width)
	}
	p.RRect(r, 0, paint.Solid(line))
}

// inGap reports whether p, in the split's space, is on the divider.
func (s *Split) inGap(p geom.Point) bool {
	a := s.firstLength()
	at := s.along(p)
	return s.gap >= 0.5 && at >= a && at < a+s.gap
}

// Cursor implements [gunim.CursorShaper]: resize arrows over the
// divider.
func (s *Split) Cursor(p geom.Point) input.Cursor {
	if !s.held && !s.inGap(p) {
		return input.CursorArrow
	}
	if s.Axis == Vertical {
		return input.CursorResizeV
	}
	return input.CursorResizeH
}

// DragsTouch implements [gunim.TouchDragger]: a finger on the divider
// drags it.
func (s *Split) DragsTouch() bool { return s.held }

// Handle implements [gunim.Handler]: the pointer drags the divider.
func (s *Split) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || !s.inGap(e.Pos) {
			return false
		}
		if e.Clicks == 2 && !s.Fixed {
			s.share.Animate(0.5, Settle.Get(th))
			s.moved(u)
			return true
		}
		s.held = true
		s.grab = s.along(e.Pos) - s.firstLength()
		s.light(true, u)
		return true
	case input.PointerMove:
		if s.held {
			space := s.length - s.gap
			if space > 0 {
				a := min(max(s.along(e.Pos)-s.grab, min(splitMin, space/2)), max(space-splitMin, space/2))
				if s.Fixed {
					s.share.Jump(a)
				} else {
					s.share.Jump(a / space)
				}
			}
			u.Invalidate()
			return true
		}
		s.light(s.inGap(e.Pos), u)
		return false
	case input.PointerUp:
		if !s.held {
			return false
		}
		s.held = false
		s.light(s.inGap(e.Pos), u)
		s.moved(u)
		return true
	case input.PointerLeave:
		if !s.held {
			s.light(false, u)
		}
	}
	return false
}

// light brightens the divider while the pointer is on it.
func (s *Split) light(on bool, u *gunim.UI) {
	if on == s.over {
		return
	}
	s.over = on
	to := float32(0)
	if on {
		to = 1
	}
	s.hot.Animate(to, Quick.Get(u.Theme()))
	u.Invalidate()
}

func (s *Split) moved(u *gunim.UI) {
	if s.OnMove != nil {
		u.Send(s, s.OnMove(s.share.Target()))
	}
}

// mixColor blends from a toward b by t, from 0 to 1.
func mixColor(a, b color.NRGBA, t float32) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}
