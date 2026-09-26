//go:build linux || darwin

package desktop

import (
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/glfw"
)

// workArea returns the part of the monitor holding p that windows may
// use, which leaves out the taskbar and the like, in screen
// coordinates. A point on no monitor takes the nearest one. It runs on
// the main thread.
func workArea(p geom.Point) geom.Rect {
	ms, err := glfw.GetMonitors()
	if err != nil {
		return geom.Rect{}
	}
	var nearest geom.Rect
	best := float32(math.MaxFloat32)
	for _, m := range ms {
		x, y, mw, mh, err := m.GetWorkarea()
		if err != nil || mw <= 0 || mh <= 0 {
			continue
		}
		r := geom.Rc(float32(x), float32(y), float32(mw), float32(mh))
		if r.Contains(p) {
			return r
		}
		if d := distanceTo(r, p); d < best {
			nearest, best = r, d
		}
	}
	return nearest
}

// distanceTo is how far p lies outside r, squared.
func distanceTo(r geom.Rect, p geom.Point) float32 {
	dx := max(r.Min.X-p.X, 0, p.X-r.Max.X)
	dy := max(r.Min.Y-p.Y, 0, p.Y-r.Max.Y)
	return dx*dx + dy*dy
}
