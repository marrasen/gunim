package gunim

import (
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// handlePlatform turns one raw platform event into tree traffic.
//
// Pointer events are routed by position, keyboard events by focus. The
// engine works out hover changes itself, on every widget's behalf.
func (u *UI) handlePlatform(ev any) {
	u.invalid = true
	switch e := ev.(type) {
	case input.PointerMove:
		u.updateHover(e.Pos, e.Time)
		mk := func(local geom.Point) input.Event {
			return input.PointerMove{Pos: local, Mods: e.Mods, Time: e.Time}
		}
		if u.capture != nil {
			u.deliver(u.capture, mk(u.local(u.capture, e.Pos)))
			return
		}
		u.dispatchAt(e.Pos, mk)
	case input.PointerDown:
		// A press moves focus before it is delivered, so a text field
		// that is clicked is already focused when it sees the press.
		u.focusAt(e.Pos)
		// Whoever takes the press keeps the pointer until the release.
		u.capture = u.dispatchAt(e.Pos, func(local geom.Point) input.Event {
			return input.PointerDown{Pos: local, Button: e.Button, Mods: e.Mods, Clicks: e.Clicks, Time: e.Time}
		})
	case input.PointerUp:
		mk := func(local geom.Point) input.Event {
			return input.PointerUp{Pos: local, Button: e.Button, Mods: e.Mods, Time: e.Time}
		}
		if c := u.capture; c != nil {
			u.capture = nil
			u.deliver(c, mk(u.local(c, e.Pos)))
			u.updateHover(e.Pos, e.Time)
			return
		}
		u.dispatchAt(e.Pos, mk)
	case input.Scroll:
		u.dispatchAt(e.Pos, func(local geom.Point) input.Event {
			return input.Scroll{Pos: local, Delta: e.Delta, Mods: e.Mods, Time: e.Time}
		})
	case input.PointerLeave:
		u.updateHover(geom.Pt(-1, -1), e.Time)
	default:
		// Keyboard and focus events go to the focused node and bubble
		// from there, which is how a shortcut a text field ignores ends
		// up at the window. With nothing focused they go to the root, so
		// a window-wide shortcut works before anything has been clicked.
		if ev, ok := ev.(input.Event); ok {
			target := u.focus
			if target == nil {
				target = u.root
			}
			if !u.bubble(target, ev) {
				u.tab(ev)
			}
		}
	}
}

// focusAt moves focus for a press at p: to the nearest [Focusable] at
// or above the node hit. A press inside the focused node leaves focus
// where it is, which keeps a focused dialog focused when its panel is
// clicked. A press anywhere else drops it.
func (u *UI) focusAt(p geom.Point) {
	target := u.hit(u.root, p)
	for s := target; s != nil; s = s.parent {
		if f, ok := s.node.(Focusable); ok && f.Focusable() {
			u.Focus(s.node)
			return
		}
	}
	if target != nil && u.focus != nil && target.within(u.focus) {
		return
	}
	u.Focus(nil)
}

// tab moves focus when Tab went unhandled: forward, or back with Shift.
func (u *UI) tab(ev input.Event) {
	k, ok := ev.(input.KeyPress)
	if !ok || k.Key != input.KeyTab || k.Mods.Has(input.ModControl) || k.Mods.Has(input.ModAlt) {
		return
	}
	u.FocusNext(!k.Mods.Has(input.ModShift))
}

// FocusNext moves keyboard focus to the next [Focusable] node the last
// frame drew, in the order they were painted, or to the previous one
// when forward is false. It wraps around at either end, and scrolls the
// node into view through every [Revealer] around it.
func (u *UI) FocusNext(forward bool) {
	var order []*state
	var walk func(s *state)
	walk = func(s *state) {
		if s.presence == Exiting || (s != u.root && s.drawn != u.seq) {
			return
		}
		if f, ok := s.node.(Focusable); ok && f.Focusable() {
			order = append(order, s)
		}
		for _, k := range s.kids {
			walk(k)
		}
	}
	walk(u.root)
	if len(order) == 0 {
		return
	}
	at := -1
	for i, s := range order {
		if s == u.focus {
			at = i
		}
	}
	var next int
	switch {
	case at < 0 && forward:
		next = 0
	case at < 0:
		next = len(order) - 1
	case forward:
		next = (at + 1) % len(order)
	default:
		next = (at - 1 + len(order)) % len(order)
	}
	u.Focus(order[next].node)
	u.reveal(order[next])
}

// reveal asks each [Revealer] above s to bring s into view.
func (u *UI) reveal(s *state) {
	for a := s.parent; a != nil; a = a.parent {
		if r, ok := a.node.(Revealer); ok {
			r.Reveal(u.rectIn(s, a), u)
		}
	}
}

// rectIn returns s's box, as the last frame drew it, in a's space.
func (u *UI) rectIn(s, a *state) geom.Rect {
	inv, ok := a.toWindow.Invert()
	if !ok {
		return geom.Rect{}
	}
	out := geom.Rect{Min: geom.Pt(float32(math.Inf(1)), float32(math.Inf(1))), Max: geom.Pt(float32(math.Inf(-1)), float32(math.Inf(-1)))}
	for _, c := range []geom.Point{{}, {X: s.size.W}, {Y: s.size.H}, s.size.Point()} {
		p := inv.Apply(s.toWindow.Apply(c))
		out.Min = geom.Pt(min(out.Min.X, p.X), min(out.Min.Y, p.Y))
		out.Max = geom.Pt(max(out.Max.X, p.X), max(out.Max.Y, p.Y))
	}
	return out
}

// updateHover sends [input.PointerLeave] and [input.PointerEnter] when
// the node under the pointer changes. While a node holds the pointer,
// only it and what is inside it can be hovered, so a drag lights up
// nothing else, and the node learns when the pointer leaves it and
// comes back.
func (u *UI) updateHover(p geom.Point, t time.Time) {
	next := u.hit(u.root, p)
	if u.capture != nil && (next == nil || !next.within(u.capture)) {
		next = nil
	}
	if next == u.hover {
		return
	}
	if u.hover != nil {
		u.deliver(u.hover, input.PointerLeave{Time: t})
	}
	u.hover = next
	if next != nil {
		u.deliver(next, input.PointerEnter{Pos: u.local(next, p), Time: t})
	}
}

// dispatchAt finds the topmost node under p and offers it the event
// built by mk, with the position translated into that node's space. A
// node that declines it passes the event on to its ancestors. It
// returns the node that took the event, or nil.
func (u *UI) dispatchAt(p geom.Point, mk func(local geom.Point) input.Event) *state {
	for s := u.hit(u.root, p); s != nil; s = s.parent {
		if h, ok := s.node.(Handler); ok {
			if h.Handle(mk(u.local(s, p)), u) {
				return s
			}
		}
	}
	return nil
}

// bubble offers e to s and then to each of its ancestors, and reports
// whether one took it.
func (u *UI) bubble(s *state, e input.Event) bool {
	for ; s != nil; s = s.parent {
		if h, ok := s.node.(Handler); ok && h.Handle(e, u) {
			return true
		}
	}
	return false
}

// deliver hands e to exactly one node and stops there. Enter, leave and
// focus changes concern a specific node, so they skip the bubbling that
// region events get.
func (u *UI) deliver(s *state, e input.Event) {
	if h, ok := s.node.(Handler); ok {
		h.Handle(e, u)
	}
}

// hit finds the topmost node under p, a point in window space.
//
// Each node is tested where the last frame drew it, through the
// transform it was painted under, so a scaled dialog or a panel sliding
// in takes clicks where it appears. The part of a node a clipping layer
// hides takes none, and neither does a node that frame left unpainted.
//
// Children are tested last-first because the last child painted is the
// one on top. Exiting nodes are skipped, so a click aimed at what lies
// behind a fading dialog reaches it.
func (u *UI) hit(s *state, p geom.Point) *state {
	for _, k := range slices.Backward(s.kids) {
		if k.presence == Exiting || k.drawn != u.seq {
			continue
		}
		if !(geom.Rect{Max: k.size.Point()}).Contains(u.local(k, p)) || !k.clip.Contains(p) {
			continue
		}
		if deep := u.hit(k, p); deep != nil {
			return deep
		}
		return k
	}
	if s == u.root {
		return nil
	}
	return s
}

// local converts a point in window space into s's own space, through
// the transform s was last painted under. A node drawn with no area,
// scaled to zero, maps every point far outside itself.
func (u *UI) local(s *state, p geom.Point) geom.Point {
	inv, ok := s.toWindow.Invert()
	if !ok {
		inf := float32(math.Inf(1))
		return geom.Pt(inf, inf)
	}
	return inv.Apply(p)
}
