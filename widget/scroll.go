package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// barLinger is how long the scrollbar stays after the content stops
// moving, before it fades.
const barLinger = 800 * time.Millisecond

// Scroll shows one child through a window it can scroll, vertically.
//
// The wheel sets a target and a spring carries the content there, so a
// fast flick of the wheel glides rather than jumps, and a new flick
// mid-glide carries on from the current speed. A thin bar fades in while
// the content moves and out once it rests. At either end the wheel
// passes on to whatever scrolls outside, so scroll views nest.
//
// The child is laid out with unbounded height and the width the Scroll
// is given.
type Scroll struct {
	child  gunim.Node
	offset *anim.Float
	target float32
	bar    *anim.Float
	idle   time.Duration
	// th is the window's live theme, kept from Layout for Step, which
	// has no frame. It is a handle to the live values, never a copy.
	th *theme.Live

	content, viewport float32
}

// NewScroll returns a Scroll showing child.
func NewScroll(child gunim.Node) *Scroll {
	return &Scroll{child: child, offset: anim.NewFloat(0), bar: anim.NewFloat(0)}
}

// Children implements [gunim.Composite].
func (s *Scroll) Children() []gunim.Node { return []gunim.Node{s.child} }

// Offset returns how far the content is scrolled right now.
func (s *Scroll) Offset() float32 { return s.offset.Value() }

// ScrollTo sets the target offset, clamped to the content, and carries
// the content there with motion.
func (s *Scroll) ScrollTo(y float32, motion anim.Motion) {
	s.target = s.clamp(y)
	s.offset.Animate(s.target, motion)
	s.bar.Animate(1, Quick.Get(s.th))
	s.idle = 0
}

func (s *Scroll) clamp(y float32) float32 {
	return max(0, min(y, s.content-s.viewport))
}

// Step implements [gunim.Animator]. Once the content has rested for a
// moment, the bar fades.
func (s *Scroll) Step(dt time.Duration) bool {
	moving := s.offset.Step(dt)
	if moving {
		s.idle = 0
	} else if s.bar.Target() > 0 {
		s.idle += dt
		if s.idle >= barLinger {
			s.bar.Animate(0, Settle.Get(s.th))
		}
	}
	barMoving := s.bar.Step(dt)
	return moving || barMoving || s.bar.Target() > 0
}

// Handle implements [gunim.Handler].
func (s *Scroll) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	var to float32
	switch e := e.(type) {
	case input.Scroll:
		to = s.target - e.Delta.Y
	case input.KeyPress:
		switch e.Key {
		case input.KeyUp:
			to = s.target - ScrollLine.Get(th)
		case input.KeyDown:
			to = s.target + ScrollLine.Get(th)
		case input.KeyPageUp:
			to = s.target - s.viewport*0.9
		case input.KeyPageDown:
			to = s.target + s.viewport*0.9
		case input.KeyHome:
			to = 0
		case input.KeyEnd:
			to = s.content
		default:
			return false
		}
	default:
		return false
	}
	// At an end, the event is for whatever scrolls outside.
	if s.clamp(to) == s.target {
		return false
	}
	s.ScrollTo(to, Quick.Get(th))
	return true
}

// Reveal implements [gunim.Revealer]: it scrolls just far enough to
// bring r, in the Scroll's own space, fully into view, with a little
// room around it.
func (s *Scroll) Reveal(r geom.Rect, u *gunim.UI) {
	const room = 8
	offset := s.offset.Value()
	switch {
	case r.Min.Y < 0:
		s.ScrollTo(offset+r.Min.Y-room, Quick.Get(u.Theme()))
	case r.Max.Y > s.viewport:
		s.ScrollTo(offset+r.Max.Y-s.viewport+room, Quick.Get(u.Theme()))
	}
}

// Layout implements [gunim.Node].
func (s *Scroll) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	s.th = f.Theme
	own := c.Max
	kid := kids.At(0)
	size := kid.Layout(gunim.Constraints{Min: geom.Sz(own.W, 0), Max: geom.Sz(own.W, 0)})
	s.content, s.viewport = size.H, own.H

	// The content may have shrunk under the offset; bring it back.
	if t := s.clamp(s.target); t != s.target {
		s.target = t
		s.offset.Animate(t, Settle.Get(f.Theme))
	}
	kid.Place(geom.Pt(0, -s.offset.Value()))
	return own
}

// Paint implements [gunim.Node].
func (s *Scroll) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1, Clip: true})()
		kids.At(0).Paint(p)
	}()

	t := s.bar.Value()
	if t <= 0 || s.content <= s.viewport {
		return
	}
	w := ScrollbarWidth.Get(f.Theme)
	length := max(box.H*s.viewport/s.content, 2*w)
	top := (box.H - length) * s.offset.Value() / (s.content - s.viewport)
	col := ScrollbarColor.Get(f.Theme)
	col.A = uint8(float32(col.A) * min(t, 1))
	p.RRect(geom.Rc(box.W-w-2, top, w, length), w/2, paint.Solid(col))
}
