package gunim

import (
	"slices"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// A BackTaker is a node that goes a page back, as one that stacks pages does. TakesBack reports whether it has a page to
// go back to now.
//
// A phone's Back, a gesture or a button of the system's, arrives as Escape, which closes what Escape closes: a menu, a
// dialog, a page that takes it. Where nothing takes that, the node last in the tree that takes Back and shows hears
// [input.HistoryStep], whichever node has the keyboard. With a [Modal] open, only nodes inside it do.
//
// In a window with [WindowOptions.LeaveOnBack] a Back that nothing takes leaves the application. The engine asks
// TakesBack after each frame and each event, as the system wants to know ahead whose the next Back is, and to show
// where it leads. A node that takes Escape alone is an [EscapeTaker]. A popup that a press outside closes, a drag and a
// modal take it themselves.
type BackTaker interface {
	Handler
	TakesBack() bool
}

// An EscapeTaker is a node that takes Escape at times, as a field that Escape clears, or a list whose selection it lets
// go of. TakesEscape reports whether it takes Escape now, with no modifiers held. A phone's Back is then for it, as Escape
// is, in a window with [WindowOptions.LeaveOnBack] too. The engine asks the nodes Escape reaches, after each frame and
// each event: the one with the keyboard and those it is in, as far as an open [Modal]. One that takes Escape always, as
// one that closes by it does, says true as long as it does.
type EscapeTaker interface {
	Handler
	TakesEscape(u *UI) bool
}

// takesEscape reports whether a node that Escape reaches takes it now.
func (u *UI) takesEscape() bool {
	s := u.focus
	if s == nil {
		s = u.appRoot()
	}
	for ; s != nil; s = s.parent {
		if e, ok := s.node.(EscapeTaker); ok && !s.leaving() && e.TakesEscape(u) {
			return true
		}
	}
	return false
}

// noteBackNode notes s while its node can take Back, so a frame walks the tree for one only while it holds some.
// [UI.forget] lets it go.
func (u *UI) noteBackNode(s *state) {
	if _, ok := s.node.(BackTaker); !ok {
		return
	}
	if u.backNodes == nil {
		u.backNodes = map[*state]struct{}{}
	}
	u.backNodes[s] = struct{}{}
}

// backTakers returns the nodes the last frame drew, in the window and its popups, that take Back now, in the order they
// stand in the tree. With a modal open it returns those inside it.
func (u *UI) backTakers() []*state {
	if len(u.backNodes) == 0 {
		return nil
	}
	var ts []*state
	m := u.modal(u.focus)
	for _, r := range u.roots() {
		ts = u.gatherBack(r, m, ts)
	}
	return ts
}

// gatherBack adds s and the nodes beneath it that take Back to ts, skipping what the last frame left undrawn and what
// is leaving, and, with a modal m, what is outside it.
func (u *UI) gatherBack(s, m *state, ts []*state) []*state {
	if s.presence == Exiting || s.drawn != u.seq {
		return ts
	}
	if b, ok := s.node.(BackTaker); ok && (m == nil || s.within(m)) && b.TakesBack() {
		ts = append(ts, s)
	}
	for _, k := range s.kids {
		ts = u.gatherBack(k, m, ts)
	}
	return ts
}

// closable reports whether a popup is open that a press outside closes.
func (u *UI) closable() bool {
	return slices.ContainsFunc(u.popups, func(p *surface) bool { return !p.closing && p.opts.Dismiss != nil })
}

// backLeaves reports whether a Back that comes now leaves the application.
func (u *UI) backLeaves() bool {
	if !u.leaveOnBack || u.drag != nil || u.closable() || u.modal(u.focus) != nil {
		return false
	}
	if len(u.keysKept) > 0 || u.layoutAwaited() {
		// A key that waits for a frame may show a page that takes it.
		return false
	}
	return !u.takesEscape() && len(u.backTakers()) == 0
}

// tellBack tells the driver whether the next Back leaves the application, where that has changed since it last heard.
func (u *UI) tellBack() {
	b, ok := u.w.dw.(driver.BackKeeper)
	if !ok {
		return
	}
	if leaves := u.backLeaves(); leaves != u.backLeft {
		u.backLeft = leaves
		b.SetBackLeaves(leaves)
	}
}

// back goes back for the user: what Escape closes, then a page, and then out of the application.
func (u *UI) back(e driver.BackAsked) {
	press := input.KeyPress{Key: input.KeyEscape, Time: e.Time}
	if u.drag != nil {
		u.dragKey(press)
		return
	}
	u.overhear(press)
	taken := u.keyEvent(press)
	u.keyEvent(input.KeyRelease{Key: input.KeyEscape, Time: e.Time})
	if taken {
		return
	}
	if u.closable() {
		u.dismissFor(nil, nil)
		return
	}
	ts := u.backTakers()
	for _, s := range slices.Backward(ts) {
		b, ok := s.node.(BackTaker)
		if !ok || s.leaving() {
			// An earlier one took this one out of the tree.
			continue
		}
		took := false
		u.on(s, func() { took = b.Handle(input.HistoryStep{Time: e.Time}, u) })
		if took {
			return
		}
	}
	// The driver heard of it too late, or a node that said it takes Back left it.
	if u.leaveOnBack && len(ts) == 0 && u.modal(u.focus) == nil {
		if b, ok := u.w.dw.(driver.BackKeeper); ok {
			b.LeaveApp()
		}
	}
}
