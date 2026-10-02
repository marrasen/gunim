// SPDX-License-Identifier: Apache-2.0

package glfw

// platformHeldModifiers reports none: reading them waits on Cocoa
// support.
func (w *Window) platformHeldModifiers() ModifierKey { return 0 }

// platformEscapeHeld reports false: a press raises the window on macOS,
// and it hears Escape itself.
func (w *Window) platformEscapeHeld() bool { return false }
