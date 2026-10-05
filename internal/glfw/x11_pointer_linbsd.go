// SPDX-License-Identifier: Apache-2.0

//go:build freebsd || linux || netbsd

package glfw

// platformPointerOnScreen asks the X server where the pointer is on the
// root window.
func platformPointerOnScreen() (x, y int, ok bool) {
	var root, child _XID
	var rx, ry, wx, wy int32
	var mask uint32
	if !xQueryPointer(_glfw.platformWindow.display, _glfw.platformWindow.root, &root, &child, &rx, &ry, &wx, &wy, &mask) {
		return 0, 0, false
	}
	return int(rx), int(ry), true
}
