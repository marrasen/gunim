package widget

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// clicker follows one click on a widget's targets, the rule every widget that acts on a click keeps: a press of the
// primary button, then its release over the target that took the press. The press that makes a double click starts
// none, so a link or a remove cross acts once however fast the clicks come; with repeats, each press starts a click,
// as a button counts every click. A finger that goes on to scroll lets go at [input.Away], over no target, and so
// acts on nothing.
//
// A target is any number of nought or more the widget gives its parts, such as a row's index; a widget with one
// target uses [over].
type clicker struct {
	// at is the target pressed, and held says its release is still to come.
	at   int
	held bool
	// repeats has every press start a click, a double click's second press too.
	repeats bool
}

// press takes e as the start of a click on target at, or on nothing for a negative at. Only the first press of the
// primary button, on a target, starts one.
func (c *clicker) press(e input.PointerDown, at int) {
	c.at, c.held = at, e.Button == input.ButtonPrimary && (e.Clicks <= 1 || c.repeats) && at >= 0
}

// release takes e as the end of the click, and reports whether the click lands: the primary button let go over
// target at, the one pressed.
func (c *clicker) release(e input.PointerUp, at int) bool {
	if e.Button != input.ButtonPrimary {
		return false
	}
	held := c.held
	c.held = false
	return held && at >= 0 && at == c.at
}

// over is the target for a widget of size with one target: 0 for pos inside it, and -1 elsewhere.
func over(pos geom.Point, size geom.Size) int {
	if (geom.Rect{Max: size.Point()}).Contains(pos) {
		return 0
	}
	return -1
}
