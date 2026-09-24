package gunim

import (
	"slices"
	"time"

	"github.com/marrasen/gunim/geom"
)

// handlePlatform turns one raw platform event into tree traffic.
//
// Pointer events are routed by position, keyboard events by focus. The
// engine works out hover changes itself, on every widget's behalf.
func (u *UI) handlePlatform(ev any) {
	u.invalid = true
	switch e := ev.(type) {
	case PointerMove:
		u.updateHover(e.Pos, e.Time)
		u.dispatchAt(e.Pos, func(local geom.Point) Event {
			return PointerMove{Pos: local, Mods: e.Mods, Time: e.Time}
		})
	case PointerDown:
		// A press moves focus before it is delivered, so a text field
		// that is clicked is already focused when it sees the press.
		if target := u.hit(u.root, e.Pos); target != nil {
			u.Focus(target.node)
		}
		u.dispatchAt(e.Pos, func(local geom.Point) Event {
			return PointerDown{Pos: local, Button: e.Button, Mods: e.Mods, Clicks: e.Clicks, Time: e.Time}
		})
	case PointerUp:
		u.dispatchAt(e.Pos, func(local geom.Point) Event {
			return PointerUp{Pos: local, Button: e.Button, Mods: e.Mods, Time: e.Time}
		})
	case Scroll:
		u.dispatchAt(e.Pos, func(local geom.Point) Event {
			return Scroll{Pos: local, Delta: e.Delta, Mods: e.Mods, Time: e.Time}
		})
	case PointerLeave:
		u.updateHover(geom.Pt(-1, -1), e.Time)
	default:
		// Keyboard and focus events go to the focused node and bubble
		// from there, which is how a shortcut a text field ignores ends
		// up at the window.
		if ev, ok := ev.(Event); ok {
			u.bubble(u.focus, ev)
		}
	}
}

// updateHover sends PointerLeave and PointerEnter when the node under
// the pointer changes.
func (u *UI) updateHover(p geom.Point, t time.Time) {
	next := u.hit(u.root, p)
	if next == u.hover {
		return
	}
	if u.hover != nil {
		u.send(u.hover, PointerLeave{Time: t})
	}
	u.hover = next
	if next != nil {
		u.send(next, PointerEnter{Pos: u.local(next, p), Time: t})
	}
}

// dispatchAt finds the topmost node under p and offers it the event
// built by mk, with the position translated into that node's space. A
// node that declines it passes the event on to its ancestors.
func (u *UI) dispatchAt(p geom.Point, mk func(local geom.Point) Event) {
	for s := u.hit(u.root, p); s != nil; s = s.parent {
		if h, ok := s.node.(Handler); ok {
			if h.Handle(mk(u.local(s, p)), u) {
				return
			}
		}
	}
}

// bubble offers e to s and then to each of its ancestors.
func (u *UI) bubble(s *state, e Event) {
	for ; s != nil; s = s.parent {
		if h, ok := s.node.(Handler); ok && h.Handle(e, u) {
			return
		}
	}
}

// send delivers e to exactly one node and stops there. Enter, leave and
// focus changes concern a specific node, so they skip the bubbling that
// region events get.
func (u *UI) send(s *state, e Event) {
	if h, ok := s.node.(Handler); ok {
		h.Handle(e, u)
	}
}

// hit finds the topmost node containing p, where p is in s's
// coordinate space.
//
// Children are tested last-first because the last child painted is the
// one on top. Exiting nodes are skipped, so a click aimed at what lies
// behind a fading dialog reaches it.
func (u *UI) hit(s *state, p geom.Point) *state {
	for _, k := range slices.Backward(s.kids) {
		if k.presence == Exiting {
			continue
		}
		if !k.bounds().Contains(p) {
			continue
		}
		if deep := u.hit(k, p.Sub(k.origin)); deep != nil {
			return deep
		}
		return k
	}
	if s == u.root {
		return nil
	}
	return s
}

// local converts a point in window space into s's own space.
func (u *UI) local(s *state, p geom.Point) geom.Point {
	for n := s; n != nil && n != u.root; n = n.parent {
		p = p.Sub(n.origin)
	}
	return p
}
