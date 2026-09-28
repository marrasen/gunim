package gunim

import (
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// handlePlatform turns one raw platform event from the window into
// tree traffic.
func (u *UI) handlePlatform(ev any) { u.handleOn(u.root, ev) }

// handleOn turns one raw platform event into tree traffic, for the
// window whose tree starts at root: the main window's, or a popup's.
//
// Pointer events are routed by position, keyboard events by focus. The
// engine works out hover changes itself, on every widget's behalf. A
// press in a popup leaves focus where it is, so the node that opened a
// menu keeps the keyboard while the menu is clicked.
func (u *UI) handleOn(root *state, ev any) {
	u.invalid = true
	if u.goingAway {
		// A window on its way out takes nothing more.
		return
	}
	switch e := ev.(type) {
	case access.Request:
		u.accessRequest(e)
	case driver.DragOutEnded:
		if len(u.dragOuts) > 0 {
			id := u.dragOuts[0]
			u.dragOuts = u.dragOuts[1:]
			u.endDrop(id, e.Taken)
		}
	case driver.WindowMaximized:
		if u.chrome != nil {
			u.chrome.maximized = e.Maximized
		}
	case driver.WindowFocus:
		if root != u.root {
			return
		}
		if !e.Focused {
			u.dismissFor(nil, nil)
		}
		var ev input.Event = input.WindowFocusGained{Time: time.Now()}
		if !e.Focused {
			ev = input.WindowFocusLost{Time: time.Now()}
		}
		target := u.focus
		if target == nil {
			target = u.root
		}
		u.bubble(target, ev)
	case input.PointerMove:
		if root == u.root {
			u.pointer = e.Pos
			if u.drag != nil {
				u.drag.mods = e.Mods
				u.dragTo(e.Pos)
				return
			}
		}
		u.updateHover(root, e.Pos, e.Time)
		mk := func(local geom.Point) input.Event {
			return input.PointerMove{Pos: local, Mods: e.Mods, Time: e.Time}
		}
		if u.capture != nil {
			u.deliver(u.capture, mk(u.local(u.capture, e.Pos)))
			u.shapePointer(root, e.Pos)
			return
		}
		u.dispatchAt(root, e.Pos, mk)
		u.shapePointer(root, e.Pos)
	case input.PointerDown:
		if root == u.root {
			u.pointer = e.Pos
		}
		// A press outside a popup dismisses it; the press still goes
		// where it lands.
		u.dismissFor(u.hit(root, e.Pos), u.onAnchorOf(root, e.Pos))
		// On a chromeless window's edge or caption, the press moves or
		// sizes the window, where the system leaves that to the engine.
		if root == u.root && u.framePress(e.Pos, e) {
			return
		}
		// A press moves focus before it is delivered, so a text field
		// that is clicked is already focused when it sees the press.
		was := u.focus
		if root == u.root {
			u.focusAt(e.Pos)
		}
		focusing := u.focus != was
		// Whoever takes the press keeps the pointer until the release.
		u.capture = u.dispatchAt(root, e.Pos, func(local geom.Point) input.Event {
			return input.PointerDown{Pos: local, Button: e.Button, Mods: e.Mods, Clicks: e.Clicks, Focusing: focusing, Time: e.Time}
		})
		u.shapePointer(root, e.Pos)
	case input.PointerUp:
		if root == u.root && u.drag != nil {
			u.drag.mods = e.Mods
			u.capture = nil
			u.dragDrop(e.Pos)
			u.updateHover(root, e.Pos, e.Time)
			return
		}
		mk := func(local geom.Point) input.Event {
			return input.PointerUp{Pos: local, Button: e.Button, Mods: e.Mods, Time: e.Time}
		}
		if c := u.capture; c != nil {
			u.capture = nil
			u.deliver(c, mk(u.local(c, e.Pos)))
			u.updateHover(root, e.Pos, e.Time)
			u.shapePointer(root, e.Pos)
			return
		}
		u.dispatchAt(root, e.Pos, mk)
	case input.Scroll:
		if !u.wheelZoomerAt(root, e.Pos) && u.zoomKey(e) {
			return
		}
		u.dispatchAt(root, e.Pos, func(local geom.Point) input.Event {
			return input.Scroll{Pos: local, Delta: e.Delta, Notches: e.Notches, Mods: e.Mods, Time: e.Time}
		})
	case input.PointerLeave:
		u.updateHover(root, geom.Pt(-1, -1), e.Time)
	case input.Drop:
		u.dispatchAt(root, e.Pos, func(local geom.Point) input.Event {
			return input.Drop{Pos: local, Data: e.Data, Paths: e.Paths, Mods: e.Mods, Time: e.Time}
		})
	case input.KeyPress, input.KeyRelease:
		if u.drag != nil {
			// Keys speak to the drag while it lasts.
			u.dragKey(ev)
			return
		}
		u.keyEvent(ev)
	default:
		u.keyEvent(ev)
	}
}

// keyEvent delivers a keyboard or focus event. It goes to the focused
// node and bubbles from there, which is how a shortcut a text field
// ignores ends up at the window. With nothing focused it goes to the
// root, so a window-wide shortcut works before anything has been
// clicked.
func (u *UI) keyEvent(ev any) {
	if u.zoomKey(ev) {
		return
	}
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

// focusAt moves focus for a press at p: to the nearest [Focusable] at
// or above the node hit. A press inside the focused node, or on a
// [FocusKeeper], leaves focus where it is, which keeps a focused dialog
// focused when its panel is clicked. A press anywhere else drops it.
func (u *UI) focusAt(p geom.Point) {
	target := u.hit(u.root, p)
	for s := target; s != nil; s = s.parent {
		if f, ok := s.node.(Focusable); ok && f.Focusable() {
			u.Focus(s.node)
			return
		}
		if _, ok := s.node.(FocusKeeper); ok {
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
			if _, skip := s.node.(TabSkipper); !skip {
				order = append(order, s)
			}
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
	u.revealState(order[next])
}

// Reveal scrolls n into view through every [Revealer] around it, as
// focusing it does, and leaves the keyboard where it is: for a list
// that follows what the user works in, such as a row for the pane in
// front.
func (u *UI) Reveal(n Node) {
	if s, ok := u.index[n]; ok {
		u.revealState(s)
	}
}

// revealState asks each [Revealer] above s to bring s into view.
func (u *UI) revealState(s *state) {
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
// the node under the pointer changes. The pointer is over a node and
// every node around it, so a card lights up while the pointer is over
// a button inside it. Each node the pointer leaves hears so, innermost
// first, and each node it enters hears so, outermost first. While a
// node holds the pointer,
// only it and what is inside it can be hovered, so a drag lights up
// nothing else, and the node learns when the pointer leaves it and
// comes back.
//
// root is the tree of the window the pointer is in. The pointer leaving
// one window clears the hover only when it was in that window, so it
// keeps what it found in the popup it moved into.
func (u *UI) updateHover(root *state, p geom.Point, t time.Time) {
	next := u.hit(root, p)
	if u.capture != nil && (next == nil || !next.within(u.capture)) {
		next = nil
	}
	if next == nil && u.hover != nil && !u.hover.within(root) {
		return
	}
	if next == u.hover {
		return
	}
	prev := u.hover
	u.hover = next
	for s := prev; s != nil && s.parent != nil && (next == nil || !next.within(s)); s = s.parent {
		u.deliver(s, input.PointerLeave{Time: t})
	}
	var entered []*state
	for s := next; s != nil && s.parent != nil && (prev == nil || !prev.within(s)); s = s.parent {
		entered = append(entered, s)
	}
	for _, s := range slices.Backward(entered) {
		u.deliver(s, input.PointerEnter{Pos: u.local(s, p), Time: t})
	}
}

// dispatchAt finds the topmost node under p and offers it the event
// built by mk, with the position translated into that node's space. A
// node that declines it passes the event on to its ancestors. It
// returns the node that took the event, or nil.
func (u *UI) dispatchAt(root *state, p geom.Point, mk func(local geom.Point) input.Event) *state {
	for s := u.hit(root, p); s != nil; s = s.parent {
		if h, ok := s.node.(Handler); ok {
			var took bool
			u.on(s, func() { took = h.Handle(mk(u.local(s, p)), u) })
			if took {
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
		if h, ok := s.node.(Handler); ok {
			var took bool
			u.on(s, func() { took = h.Handle(e, u) })
			if took {
				return true
			}
		}
	}
	return false
}

// deliver hands e to exactly one node and stops there. Enter, leave and
// focus changes concern a specific node, so they skip the bubbling that
// region events get.
func (u *UI) deliver(s *state, e input.Event) {
	if h, ok := s.node.(Handler); ok {
		u.on(s, func() { h.Handle(e, u) })
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
		local := u.local(k, p)
		if !(geom.Rect{Max: k.size.Point()}).Contains(local) || !k.clip.Contains(p) {
			continue
		}
		if sh, ok := k.node.(Shaped); ok && !sh.Covers(local) {
			continue
		}
		if deep := u.hit(k, p); deep != nil {
			return deep
		}
		return k
	}
	if s.parent == nil {
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
