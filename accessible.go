package gunim

import (
	"slices"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// An Accessible is a node that means something to a person using a
// screen reader, and says what: a button and its label, a checkbox and
// whether it is checked. A node that only arranges others, such as a
// row, leaves this out, and the nodes inside it belong to the nearest
// Accessible above it.
type Accessible interface {
	Node
	Access() access.Info
}

// An AccessActor is an [Accessible] that carries out a screen reader's
// requests: its actions, or a new value. It reports whether it did.
// Focus requests are the engine's, for any [Focusable].
type AccessActor interface {
	Accessible
	AccessAct(r access.Request, u *UI) bool
}

// partBits is where a part's number goes in its ID, above the node's.
const partBits = 48

// accessID returns s's ID for assistive technology, giving it one on
// first use. IDs count up from 1 and never repeat, so a screen reader
// holding one for a node that has gone finds nothing.
func (u *UI) accessID(s *state) uint64 {
	if s.aid == 0 {
		u.aids++
		s.aid = u.aids
	}
	return s.aid
}

// accessTree gathers what the window's tree says for assistive
// technology, as the last frame drew it.
func (u *UI) accessTree(root *state, title string) *access.Tree {
	if u.byAID == nil {
		u.byAID = map[*state]map[uint64]*state{}
	}
	// Trees of popups that have closed go.
	for r := range u.byAID {
		if r != u.root && !slices.ContainsFunc(u.popups, func(p *surface) bool { return p.root == r }) {
			delete(u.byAID, r)
		}
	}
	ids := map[uint64]*state{}
	u.byAID[root] = ids
	top := &access.Node{
		Info:   access.Info{Role: access.RoleWindow, Name: title},
		ID:     u.accessID(root),
		Bounds: geom.Rect{Max: root.size.Point()},
	}
	ids[top.ID] = root
	u.gather(root, top, ids)
	return access.NewTree(top)
}

// gather adds what s's children say to parent, skipping nodes that say
// nothing and the ones the last frame left undrawn or that are leaving.
// ids gathers the nodes by ID.
func (u *UI) gather(s *state, parent *access.Node, ids map[uint64]*state) {
	for _, k := range s.kids {
		if k.presence == Exiting || k.drawn != u.seq {
			continue
		}
		into := parent
		if a, ok := k.node.(Accessible); ok {
			n := u.accessNode(k, a.Access())
			ids[n.ID] = k
			parent.Children = append(parent.Children, n)
			into = n
		}
		u.gather(k, into, ids)
	}
}

// accessNode makes s's node from what it said.
func (u *UI) accessNode(s *state, info access.Info) *access.Node {
	id := u.accessID(s)
	f, focusable := s.node.(Focusable)
	n := &access.Node{
		Info:      info,
		ID:        id,
		Bounds:    windowRect(s.toWindow, geom.Rect{Max: s.size.Point()}),
		Focusable: focusable && f.Focusable(),
		Focused:   u.focus == s && info.Active == 0,
	}
	k := 0
	n.Children = u.accessParts(s, info.Parts, id, n.Focusable, info.Active, &k)
	n.Parts = nil
	return n
}

// accessParts makes the nodes of s's parts and the parts inside them,
// counting them in order with k. A part takes focus with its node, such
// as a tab with the tab list.
func (u *UI) accessParts(s *state, parts []access.Info, id uint64, focusable bool, active int, k *int) []*access.Node {
	if len(parts) == 0 {
		return nil
	}
	out := make([]*access.Node, 0, len(parts))
	for _, p := range parts {
		*k++
		part := &access.Node{
			Info:      p,
			ID:        id | uint64(*k)<<partBits,
			Bounds:    windowRect(s.toWindow, p.Bounds),
			Focusable: focusable,
			Focused:   u.focus == s && active == *k,
		}
		part.Children = u.accessParts(s, p.Parts, id, focusable, active, k)
		part.Parts = nil
		out = append(out, part)
	}
	return out
}

// windowRect maps r through t into window space, as the box around its
// corners.
func windowRect(t paint.Transform, r geom.Rect) geom.Rect {
	a, b := t.Apply(r.Min), t.Apply(r.Max)
	return geom.Rect{Min: a, Max: b}.Normalized()
}

// publishAccess hands dw the tree under root, when something there
// wants it.
func (u *UI) publishAccess(dw driver.Window, root *state, title string) {
	pub, ok := dw.(driver.AccessPublisher)
	if !ok || !pub.AccessWanted() {
		return
	}
	pub.PublishAccess(u.accessTree(root, title))
}

// focusNow tells assistive technology at once when input moved focus,
// from the tree as the last frame drew it with focus where it is now.
// A screen reader reading a key press looks for the focus straight
// away; told a frame later, it takes the news for what it already
// knows, and says nothing.
func (u *UI) focusNow() {
	if !u.focusMoved {
		return
	}
	u.focusMoved = false
	u.publishAccess(u.w.dw, u.root, u.w.title)
}

// accessRequest carries out a screen reader's request.
func (u *UI) accessRequest(r access.Request) {
	var s *state
	for _, ids := range u.byAID {
		if found, ok := ids[r.ID&(1<<partBits-1)]; ok {
			s = found
		}
	}
	if s == nil || s.leaving() {
		return
	}
	r.Part = int(r.ID>>partBits) - 1
	if r.Focus {
		u.Focus(s.node)
	}
	if a, ok := s.node.(AccessActor); ok && (r.Action != "" || r.SetValue) {
		u.on(s, func() { a.AccessAct(r, u) })
	}
	u.invalid = true
}
