package widget

import (
	"math"
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
	// Anchor makes the hero a place a counterpart flies from and back to, such as a tile in a grid: it stays put as
	// it arrives, and leaving sends nothing flying.
	Anchor bool

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
	// natural is the child's size at the hero's own place, and scale how
	// many window pixels one of its own is there, under the transforms
	// it is drawn with, as last painted.
	natural geom.Size
	scale   float32
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
		// Heroes not drawn lately have left, or are out of sight, and some have taken another tag.
		return o != h && (o.seen+2 < f.Number() || o.Tag != h.Tag)
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
		if h.leaving {
			// Brought back before it had gone, as a view mounted again
			// mid-exit: it shows again, and flies out once more.
			h.leaving, h.hidden, h.flying, h.arrived = false, false, false, false
		}
		if !h.arrived {
			h.arrived = true
			if o := h.counterpart(f); o != nil && !h.Anchor {
				// From where the counterpart shows, on its way back if it is.
				h.takeOff(o.shown(), f)
				o.hidden, h.partner = true, o
			}
		}
	case gunim.Exiting:
		if !h.leaving {
			h.leaving = true
			if o := h.counterpart(f); o != nil && !h.Anchor {
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

// shown returns where the hero shows now, in window space: on its way,
// mid-flight.
func (h *Hero) shown() geom.Rect {
	if h.flying {
		return lerpRect(h.from, h.rect, h.fly.Value())
	}
	return h.rect
}

// takeOff starts a flight from r.
func (h *Hero) takeOff(r geom.Rect, f gunim.Frame) {
	if h.flying {
		// Mid-flight, start again from where it is now.
		r = h.shown()
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
		// The child takes the size in between, for this frame, as it will
		// show in the window: under a transform that scales the hero's
		// place, as a zoom does, it flies to the size it appears there.
		k := max(h.scale, 0.001)
		if h.scale == 0 {
			k = 1
		}
		s := lerpSize(h.from.Size(), geom.Sz(h.natural.W*k, h.natural.H*k), h.fly.Value())
		kid.Layout(gunim.Tight(geom.Sz(max(s.W/k, 0), max(s.H/k, 0))))
	}
	return h.natural
}

// Paint implements [gunim.Node].
func (h *Hero) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	t := p.Transform()
	h.rect = geom.Rect{Min: t.Apply(geom.Point{}), Max: t.Apply(box.Point())}.Normalized()
	h.scale = float32(math.Hypot(float64(t.A), float64(t.D)))
	h.seen = f.Number()
	if h.hidden {
		return
	}
	if !h.flying {
		kids.At(0).Paint(p)
		return
	}
	// The child, laid out at the size in between, scaled as the window
	// shows its place, fills the rectangle in between.
	at := lerpRect(h.from, h.rect, h.fly.Value())
	kid := kids.At(0)
	k := h.scale
	if w := kid.Size().W; w > 0 {
		k = at.Size().W / w
	}
	p.Float(func(p *paint.Painter) {
		defer p.Push(paint.Translate(at.Min))()
		defer p.Push(paint.Scale(k, geom.Point{}))()
		kid.Paint(p)
	})
}

func lerpRect(a, b geom.Rect, t float32) geom.Rect {
	return geom.Rect{Min: lerpPt(a.Min, b.Min, t), Max: lerpPt(a.Max, b.Max, t)}
}

func lerpSize(a, b geom.Size, t float32) geom.Size {
	return geom.Sz(a.W+(b.W-a.W)*t, a.H+(b.H-a.H)*t)
}
