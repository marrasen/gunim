package widget

import (
	"github.com/marrasen/gunim"
)

// edgeBelow is how far a node stood from the window's bottom edge in the last frame drawn, and false where that
// frame did not draw it.
func edgeBelow(u *gunim.UI, n gunim.Node) (float32, bool) {
	if u == nil {
		return 0, false
	}
	at, ok := u.Bounds(n)
	if !ok {
		return 0, false
	}
	win, ok := u.Bounds(u.Root())
	if !ok {
		return 0, false
	}
	return max(win.Max.Y-at.Max.Y, 0), true
}

// under returns how far up into a node's box from its bottom edge the system's bars and the keyboard reach. A node
// that a [Surface] keeps clear of the bars stands Safe.Bottom up from the window's edge, so before it has been drawn
// it counts from there.
func under(f gunim.Frame, n gunim.Node) (safe, keyboard float32) {
	gap, ok := edgeBelow(f.UI(), n)
	if !ok {
		gap = f.Safe.Bottom
	}
	return max(f.Safe.Bottom-gap, 0), max(f.Keyboard-gap, 0)
}

// runsUnder reports whether a node stands right on the system's bars at the window's bottom, so its ground may run
// on under them, as [Surface] keeps it clear of them.
func runsUnder(f gunim.Frame, n gunim.Node) float32 {
	gap, ok := edgeBelow(f.UI(), n)
	if !ok || f.Safe.Bottom <= 0 || abs32(gap-f.Safe.Bottom) > 0.5 {
		return 0
	}
	return f.Safe.Bottom
}
