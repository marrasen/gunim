// SPDX-License-Identifier: Apache-2.0

package glfw

// platformPointerOnScreen reports nothing: reading it waits on Cocoa
// support.
func platformPointerOnScreen() (x, y int, ok bool) { return 0, 0, false }
