package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// Convert turns p, in from's own space, into to's own space, as the
// frame just drawn placed them. The two may sit in different windows, as
// a popup's content and the node that opened it do: a handle dragged in
// a popup over the text it selects. Convert reports false when either
// node is out of the tree, or one of their windows cannot say where it
// is on the screen.
func (u *UI) Convert(p geom.Point, from, to Node) (geom.Point, bool) {
	fs, ok := u.index[from]
	if !ok {
		return p, false
	}
	ts, ok := u.index[to]
	if !ok {
		return p, false
	}
	at := fs.screenAt(p)
	if fw, tw := u.windowOf(fs), u.windowOf(ts); fw != tw {
		fsc, ok := fw.(driver.Screener)
		if !ok {
			return p, false
		}
		tsc, ok := tw.(driver.Screener)
		if !ok {
			return p, false
		}
		at = tsc.FromScreen(fsc.ToScreen(at))
	}
	return u.local(ts, at), true
}
