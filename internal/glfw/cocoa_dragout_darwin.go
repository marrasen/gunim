// SPDX-License-Identifier: Apache-2.0

package glfw

import "errors"

// Dragging out of a window is a gunim change that Cocoa has yet to get.

func (w *Window) platformStartDragOut([]string, func(bool, bool)) error {
	return errors.New("glfw: dragging out of a window waits on Cocoa support")
}

func (w *Window) platformCancelDragOut() {}
