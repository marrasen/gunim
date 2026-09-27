// SPDX-License-Identifier: Apache-2.0

package glfw

// platformHeldModifiers reports none: reading them waits on Cocoa
// support.
func (w *Window) platformHeldModifiers() ModifierKey { return 0 }
