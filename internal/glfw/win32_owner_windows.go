// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It lets a popup belong to one window, staying just above it rather than above
// every window.

package glfw

import (
	"errors"
)

// SetOwner keeps the window just above owner, below any window in front of owner, in place of above every window. It
// is a gunim addition, on Windows alone.
func (w *Window) SetOwner(owner *Window) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	_SetWindowLongPtrW(w.platform.handle, _GWLP_HWNDPARENT, uintptr(owner.platform.handle))
	if _GetWindow(w.platform.handle, _GW_OWNER) != owner.platform.handle {
		return errors.New("glfw: SetOwner: the window's owner did not change")
	}
	w.platform.owner = owner.platform.handle
	w.floating = false
	if err := _SetWindowPos(w.platform.handle, _HWND_NOTOPMOST, 0, 0, 0, 0,
		_SWP_NOACTIVATE|_SWP_NOMOVE|_SWP_NOSIZE|_SWP_NOOWNERZORDER); err != nil {
		return err
	}
	return w.raiseToOwner()
}

// raiseToOwner puts the window just above its owner, below the window in front of the owner.
func (w *Window) raiseToOwner() error {
	after := _GetWindow(w.platform.owner, _GW_HWNDPREV)
	if after == w.platform.handle {
		return nil
	}
	if after != 0 {
		ex, err := _GetWindowLongW(after, _GWL_EXSTYLE)
		if err != nil {
			return err
		}
		// Going under a topmost window would make this one topmost
		if uint32(ex)&_WS_EX_TOPMOST != 0 {
			after = _HWND_TOP
		}
	}
	return _SetWindowPos(w.platform.handle, after, 0, 0, 0, 0, _SWP_NOACTIVATE|_SWP_NOMOVE|_SWP_NOSIZE|_SWP_NOOWNERZORDER)
}
