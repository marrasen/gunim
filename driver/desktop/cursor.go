//go:build linux || windows || darwin

package desktop

import (
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// standardCursors are the platform's shapes for gunim's.
var standardCursors = map[input.Cursor]glfw.StandardCursor{
	input.CursorText:       glfw.IBeamCursor,
	input.CursorHand:       glfw.HandCursor,
	input.CursorCrosshair:  glfw.CrosshairCursor,
	input.CursorResizeH:    glfw.HResizeCursor,
	input.CursorResizeV:    glfw.VResizeCursor,
	input.CursorMove:       glfw.ResizeAllCursor,
	input.CursorNotAllowed: glfw.NotAllowedCursor,
}

// SetCursor implements [driver.CursorSetter].
func (w *Window) SetCursor(c input.Cursor) {
	w.d.post(func() {
		if w.closed {
			return
		}
		_ = w.gw.SetCursor(w.d.cursor(c))
	})
}

// cursor returns the platform's cursor for c, made on first use, or nil
// for the arrow, and for a shape the platform lacks. It runs on the main
// thread.
func (d *Driver) cursor(c input.Cursor) *glfw.Cursor {
	shape, ok := standardCursors[c]
	if !ok {
		return nil
	}
	if cur, ok := d.cursors[c]; ok {
		return cur
	}
	cur, err := glfw.CreateStandardCursor(shape)
	if err != nil {
		cur = nil
	}
	if d.cursors == nil {
		d.cursors = map[input.Cursor]*glfw.Cursor{}
	}
	d.cursors[c] = cur
	return cur
}
