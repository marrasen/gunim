package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// windowPlace is where a node stood in its window as it was last painted: the window in the node's own space, and
// how far the node's bottom edge stood from the window's. A node notes it in Paint, where the engine knows its place,
// and reads it in the next Layout, where it does not yet.
type windowPlace struct {
	win geom.Rect
	gap float32
	ok  bool
}

// note notes where n stands in f's window, from Paint.
func (w *windowPlace) note(f gunim.Frame, n gunim.Node) {
	w.ok = false
	u := f.UI()
	if u == nil {
		return
	}
	at, okAt := u.Bounds(n)
	win, okWin := u.Bounds(u.Root())
	if !okAt || !okWin {
		return
	}
	w.win = win.Add(geom.Pt(-at.Min.X, -at.Min.Y))
	w.gap, w.ok = max(win.Max.Y-at.Max.Y, 0), true
}

// window returns the window in the node's space, or box where the node has not been painted.
func (w *windowPlace) window(box geom.Size) geom.Rect {
	if !w.ok {
		return geom.Rect{Max: box.Point()}
	}
	return w.win
}

// under returns how far up into the node's box from its bottom edge the system's bars and the keyboard reach. A
// node that a [Surface] keeps clear of the bars stands Safe.Bottom up from the window's edge, so before it has been
// painted it counts from there.
func (w *windowPlace) under(f gunim.Frame) (safe, keyboard float32) {
	gap := f.Safe.Bottom
	if w.ok {
		gap = w.gap
	}
	return max(f.Safe.Bottom-gap, 0), max(f.Keyboard-gap, 0)
}

// runsUnder is how far the node's ground may run on below its box, under the system's bars: as far as they reach
// where the node stands right on them, as a [Surface] keeps it, and 0 elsewhere.
func (w *windowPlace) runsUnder(f gunim.Frame) float32 {
	if !w.ok || f.Safe.Bottom <= 0 || abs32(w.gap-f.Safe.Bottom) > 0.5 {
		return 0
	}
	return f.Safe.Bottom
}
