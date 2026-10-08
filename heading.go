package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// A HeadingWatcher is a node that wants to know which way the device faces, as a compass rose turned to north does,
// or a game that asks the player to turn toward the east. While it is shown and WatchesHeading reports true, the
// engine runs the device's compass and hands the node each reading as [input.Heading]. Once no node shown watches,
// the engine stops the compass, which spares the battery, and it stops it while the window is hidden too, as while
// its application is in the background. WatchesHeading is asked after each frame, so a node that starts or stops
// watching between frames asks for one with [UI.Invalidate].
//
// [Client.HasCompass] and [UI.HasCompass] say whether there is a compass to run. A desktop has none, nor has every
// phone, and a watcher there hears nothing, so an application asks first and offers another way where there is
// none.
type HeadingWatcher interface {
	Handler
	WatchesHeading() bool
}

// HasCompass reports whether the device has a compass gunim can run for a [HeadingWatcher]: a phone with the
// sensors does, and a desktop does not. It is safe from any goroutine.
func (c Client) HasCompass() bool { return hasCompass(c.w.dw) }

// HasCompass reports whether the device has a compass gunim can run for a [HeadingWatcher], as
// [Client.HasCompass] does. It is false on a nil UI.
func (u *UI) HasCompass() bool { return u != nil && hasCompass(u.w.dw) }

// hasCompass reports whether dw can tell which way the device faces.
func hasCompass(dw driver.Window) bool {
	c, ok := dw.(driver.Compass)
	return ok && c.HasCompass()
}

// watchHeading runs the device's compass while a node shown watches the heading, and stops it once none does or
// the window is hidden. It runs after each frame, which settles what is shown, and as the window hides.
func (u *UI) watchHeading() {
	c, ok := u.w.dw.(driver.Compass)
	if !ok || !c.HasCompass() {
		return
	}
	want := !u.w.hidden && !u.goingAway && len(u.headingWatchers()) > 0
	if want != u.compassOn {
		u.compassOn = want
		c.WatchHeading(want)
	}
}

// stopHeading stops the device's compass, as the window closes.
func (u *UI) stopHeading() {
	if !u.compassOn {
		return
	}
	u.compassOn = false
	if c, ok := u.w.dw.(driver.Compass); ok {
		c.WatchHeading(false)
	}
}

// noteHeadingNode notes s while its node can watch the heading, so a frame walks the tree for watchers only while
// it holds one. [UI.forget] lets it go.
func (u *UI) noteHeadingNode(s *state) {
	if _, ok := s.node.(HeadingWatcher); !ok {
		return
	}
	if u.headingNodes == nil {
		u.headingNodes = map[*state]struct{}{}
	}
	u.headingNodes[s] = struct{}{}
}

// headingEvent hands a reading of the compass to every node shown that watches it.
func (u *UI) headingEvent(e input.Heading) {
	for _, s := range u.headingWatchers() {
		if s.leaving() {
			// An earlier watcher took this one out of the tree.
			continue
		}
		u.deliver(s, e)
	}
}

// headingWatchers returns the nodes the last frame drew, in the window and its popups, that watch the heading now.
func (u *UI) headingWatchers() []*state {
	if len(u.headingNodes) == 0 {
		return nil
	}
	var ws []*state
	for _, r := range u.roots() {
		ws = u.gatherWatchers(r, ws)
	}
	return ws
}

// gatherWatchers adds s and the nodes beneath it that watch the heading to ws, skipping what the last frame left
// undrawn and what is leaving.
func (u *UI) gatherWatchers(s *state, ws []*state) []*state {
	if s.presence == Exiting || s.drawn != u.seq {
		return ws
	}
	if h, ok := s.node.(HeadingWatcher); ok && h.WatchesHeading() {
		ws = append(ws, s)
	}
	for _, k := range s.kids {
		ws = u.gatherWatchers(k, ws)
	}
	return ws
}
