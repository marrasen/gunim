package gunim

// A Modal is a node that holds the keyboard while it is in the tree, such as a dialog. It takes the focus as it
// arrives. While it stays, the focus and Tab keep to it and to the popups opened from inside it, and when it leaves,
// the focus goes back to where it was before.
type Modal interface {
	Node
	Modal() bool
}

// modalHold is a modal in the tree, and the node that had the focus before it came.
type modalHold struct {
	s, back *state
}

// arrived gives a modal that just came into the tree the focus.
func (u *UI) arrived(s *state) {
	m, ok := s.node.(Modal)
	if !ok || !m.Modal() {
		return
	}
	u.modals = append(u.modals, modalHold{s: s, back: u.focus})
	u.Focus(s.node)
}

// modal returns the modal holding the keyboard: the last to arrive that is still in the tree and not leaving, or
// nil.
func (u *UI) modal() *state {
	for i := len(u.modals) - 1; i >= 0; i-- {
		if s := u.modals[i].s; u.index[s.node] == s && !s.leaving() {
			return s
		}
	}
	return nil
}

// left forgets the modals that have left or are leaving, and gives the focus back to what had it before the last
// of them came, when it can still take it.
func (u *UI) left() {
	var back *state
	kept := u.modals[:0]
	for _, h := range u.modals {
		if u.index[h.s.node] == h.s && !h.s.leaving() {
			kept = append(kept, h)
			continue
		}
		if back == nil {
			back = h.back
		}
	}
	u.modals = kept
	if back == nil || u.index[back.node] != back || back.leaving() {
		return
	}
	if u.focus == nil || u.focus.leaving() {
		u.Focus(back.node)
	}
}

// inside reports whether s is m or lies within it, counting a popup's tree as within the node that opened it.
func inside(s, m *state) bool {
	for a := s; a != nil; {
		if a == m {
			return true
		}
		if a.parent == nil {
			a = a.opener
			continue
		}
		a = a.parent
	}
	return false
}
