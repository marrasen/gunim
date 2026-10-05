// SPDX-License-Identifier: Apache-2.0

package glfw

// platformPointerOnScreen is GetCursorPos, in the virtual screen's
// device pixels.
func platformPointerOnScreen() (x, y int, ok bool) {
	p, err := _GetCursorPos()
	if err != nil {
		return 0, 0, false
	}
	return int(p.x), int(p.y), true
}
