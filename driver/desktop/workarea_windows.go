//go:build windows

package desktop

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/glfw"
)

// workArea returns the part of the monitor holding p, or nearest it,
// that windows may use, which leaves out the taskbar, in screen
// coordinates. Windows is asked which monitor that is, rather than
// GLFW's list, whose first monitor a point matched to none would get.
// It runs on the main thread.
func workArea(p geom.Point) geom.Rect {
	x, y, w, h, ok := glfw.WorkareaAt(int(p.X), int(p.Y))
	if !ok || w <= 0 || h <= 0 {
		return geom.Rect{}
	}
	return geom.Rc(float32(x), float32(y), float32(w), float32(h))
}
