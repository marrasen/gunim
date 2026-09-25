// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It lets the application answer
// WM_GETOBJECT, the message through which UI Automation, and screen
// readers with it, ask a window for its accessibility objects.

package glfw

const _WM_GETOBJECT = 0x003D

// GetObjectCallback answers WM_GETOBJECT: it returns the message's
// result and true, or false to leave the message to the system.
type GetObjectCallback func(w *Window, wParam, lParam uintptr) (uintptr, bool)

// SetGetObjectCallback sets the callback answering WM_GETOBJECT, and
// returns the previously set one.
func (w *Window) SetGetObjectCallback(cbfun GetObjectCallback) (GetObjectCallback, error) {
	if !_glfw.initialized {
		return nil, NotInitialized
	}
	old := w.platform.getObjectCallback
	w.platform.getObjectCallback = cbfun
	return old, nil
}
