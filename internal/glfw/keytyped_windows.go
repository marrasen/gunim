// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. See keytyped.go.

package glfw

import "golang.org/x/sys/windows"

const _WM_DEADCHAR = 0x0103

// typedNext reports whether the key press being handled typed text.
// The message loop translated it before dispatching it, so the text it
// typed already waits in the queue as WM_CHAR, or as WM_DEADCHAR for a
// dead key. Text typed with Alt comes as WM_SYSCHAR instead, and
// control characters, such as Ctrl+C's, are no text.
func typedNext(hWnd windows.HWND) bool {
	var next _MSG
	if !_PeekMessageW(&next, hWnd, _WM_CHAR, _WM_DEADCHAR, _PM_NOREMOVE) {
		return false
	}
	if next.message == _WM_DEADCHAR {
		return true
	}
	c := rune(next.wParam)
	return (c >= 0xd800 && c <= 0xdbff) || isTextCodepoint(c)
}
