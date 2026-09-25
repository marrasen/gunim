// SPDX-License-Identifier: Apache-2.0

package glfw

// StartDragOut hands the drag the pointer is carrying, with a button
// down in w, to other programs, as the files at paths. It is a gunim
// change. From here the platform's drag and drop takes the pointer's
// moves and the button's release, and end runs on the main thread once
// the drag is over, saying whether a program took the drop.
func (w *Window) StartDragOut(paths []string, end func(taken bool)) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	return w.platformStartDragOut(paths, end)
}

// CancelDragOut gives up a drag out of w that is waiting on a program
// that never answers; its end runs with taken false. It is a gunim
// change.
func (w *Window) CancelDragOut() {
	if !_glfw.initialized {
		return
	}
	w.platformCancelDragOut()
}
