package widget

import (
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Hero is an element that flies from one screen to the next: a
// thumbnail growing into the picture it stands for, a list row's title
// becoming a page's heading.
//
// Give two heroes the same Tag, one in each screen. When one arrives
// while the other is on screen, it flies from where the other is to
// its own place, and the other hides until it lands. When one leaves
// while the other is on screen, the other flies back from where the
// leaving one was. The flight follows the theme's [HeroMotion].
//
// In flight, the arriving hero's child is laid out at the size in
// between, every frame, so a picture that fills its box by cropping
// keeps filling it as it grows. It is drawn above everything else in
// the window, so a scroll view or card it leaves never clips it.
// Places and sizes are measured in the window, so a hero under a
// scaling transform flies to where it appears.
type Hero struct {
	anim.Group
	Tag string

	child gunim.Node
	// fly runs from 0 to 1 over a flight, from and to its ends in window
	// space, the to end followed each frame.
	fly    *anim.Float
	from   geom.Rect
	flying bool
	// hidden is set while a counterpart flies in this hero's place, and
	// partner is the counterpart this hero hid for its own flight.
	hidden  bool
	partner *Hero
	// rect is where the hero was last drawn, in window space, in frame
	// seen. leaving is set once the hero has started to leave.
	rect    geom.Rect
	seen    uint64
	arrived bool
	leaving bool
	// natural is the child's size at the hero's own place.
	natural geom.Size
}

// NewHero returns child as a hero tagged tag.
func NewHero(tag string, child gunim.Node) *Hero {
	h := &Hero{Tag: tag, child: child, fly: anim.NewFloat(1)}
	h.Add(h.fly)
	return h
}

// Children implements [gunim.Composite].
func (h *Hero) Children() []gunim.Node { return []gunim.Node{h.child} }

// heroes lists a window's heroes by tag.
type heroes map[string][]*Hero

type heroesKey struct{}

func (h *Hero) register(f gunim.Frame) heroes {
	reg := gunim.Local(f, heroesKey{}, func() heroes { return heroes{} })
	list := slices.DeleteFunc(reg[h.Tag], func(o *Hero) bool {
		// Heroes not drawn lately have left, or are out of sight.
		return o != h && o.seen+2 < f.Number()
	})
	if !slices.Contains(list, h) {
		list = append(list, h)
	}
	reg[h.Tag] = list
	return reg
}

// counterpart returns the other hero with h's tag that was on screen
// in the frame just before this one and is staying, or nil.
func (h *Hero) counterpart(f gunim.Frame) *Hero {
	for _, o := range h.register(f)[h.Tag] {
		if o != h && !o.leaving && o.seen+1 == f.Number() {
			return o
		}
	}
	return nil
}

// Transition implements [gunim.Transitioner].
func (h *Hero) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		if !h.arrived {
			h.arrived = true
			if o := h.counterpart(f); o != nil {
				h.takeOff(o.rect, f)
				o.hidden, h.partner = true, o
			}
		}
	case gunim.Exiting:
		if !h.leaving {
			h.leaving = true
			if o := h.counterpart(f); o != nil {
				// The one staying flies back from here, and this one is
				// gone at once: there is only ever one of a pair to see.
				o.takeOff(h.rect, f)
				h.hidden = true
			}
		}
	case gunim.Present:
	}
	return h.hidden || !h.flying
}

// takeOff starts a flight from r.
func (h *Hero) takeOff(r geom.Rect, f gunim.Frame) {
	if h.flying {
		// Mid-flight, start again from where it is now.
		r = lerpRect(h.from, h.rect, h.fly.Value())
	}
	h.from, h.flying, h.hidden = r, true, false
	h.fly.Jump(0)
	h.fly.Animate(1, HeroMotion.Get(f.Theme))
}

// Layout implements [gunim.Node].
func (h *Hero) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	h.register(f)
	if h.flying && !h.fly.Active() {
		h.flying = false
		if h.partner != nil {
			h.partner.hidden, h.partner = false, nil
		}
	}
	kid := kids.At(0)
	h.natural = kid.Layout(c)
	kid.Place(geom.Point{})
	if h.flying {
		// The child takes the size in between, for this frame.
		s := lerpSize(h.from.Size(), h.natural, h.fly.Value())
		kid.Layout(gunim.Tight(geom.Sz(max(s.W, 0), max(s.H, 0))))
	}
	return h.natural
}

// Paint implements [gunim.Node].
func (h *Hero) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	t := p.Transform()
	h.rect = geom.Rect{Min: t.Apply(geom.Point{}), Max: t.Apply(box.Point())}.Normalized()
	h.seen = f.Number()
	if h.hidden {
		return
	}
	if !h.flying {
		kids.At(0).Paint(p)
		return
	}
	at := lerpRect(h.from, h.rect, h.fly.Value()).Min
	p.Float(func(p *paint.Painter) {
		defer p.Push(paint.Translate(at))()
		kids.At(0).Paint(p)
	})
}

func lerpRect(a, b geom.Rect, t float32) geom.Rect {
	return geom.Rect{Min: lerpPt(a.Min, b.Min, t), Max: lerpPt(a.Max, b.Max, t)}
}

func lerpSize(a, b geom.Size, t float32) geom.Size {
	return geom.Sz(a.W+(b.W-a.W)*t, a.H+(b.H-a.H)*t)
}
