// SPDX-License-Identifier: Apache-2.0

//go:build freebsd || linux || netbsd

package glfw

// platformHeldModifiers reads the modifiers from the pointer's state.
func (w *Window) platformHeldModifiers() ModifierKey {
	var root, child _XID
	var rx, ry, wx, wy int32
	var mask uint32
	if !xQueryPointer(_glfw.platformWindow.display, w.platform.handle, &root, &child, &rx, &ry, &wx, &wy, &mask) {
		return 0
	}
	return translateState(mask) &^ (ModCapsLock | ModNumLock)
}

// platformEscapeHeld reports false: a press raises the window on X11,
// and it hears Escape itself.
func (w *Window) platformEscapeHeld() bool { return false }
