// SPDX-License-Identifier: Apache-2.0

package glfw

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// stackWalk bounds the walk down the stack of top-level windows, which
// may change under it.
const stackWalk = 1 << 16

// platformDepths walks the top-level windows from the front, as
// GetTopWindow and GetWindow order them, topmost windows first, and
// counts the steps to each of ws. Nothing in the walk looks at the
// pointer, so neither a window under it that lets it through, such as
// a drag's picture, nor a window holding it on a button press changes
// the answer.
func platformDepths(ws []*Window, depths []int) {
	at := map[windows.HWND][]int{}
	for i, w := range ws {
		if w == nil || w.platform.handle == 0 || !_IsWindowVisible(w.platform.handle) || _IsIconic(w.platform.handle) {
			continue
		}
		at[w.platform.handle] = append(at[w.platform.handle], i)
	}
	if len(at) == 0 {
		return
	}
	found := 0
	h := _GetTopWindow()
	for step := 0; h != 0 && found < len(at) && step < stackWalk; step++ {
		if is, ok := at[h]; ok {
			for _, j := range is {
				depths[j] = step
			}
			found++
		}
		h = _GetWindow(h, _GW_HWNDNEXT)
	}
}

// platformCovered walks the top-level windows from the front, as
// platformDepths does, to the first that shows at x, y: one of ws, or
// another program's. The application's other windows, such as a drag's
// picture or a menu, are passed over, and so are windows that show
// nothing there: hidden, minimized, cloaked, as on another virtual
// desktop, or letting the pointer through, as an overlay does.
func platformCovered(ws []*Window, x, y int) (covered, ok bool) {
	mine := map[windows.HWND]bool{}
	for _, w := range _glfw.windows {
		mine[w.platform.handle] = false
	}
	for _, w := range ws {
		if w != nil && w.platform.handle != 0 {
			mine[w.platform.handle] = true
		}
	}
	h := _GetTopWindow()
	for step := 0; h != 0 && step < stackWalk; step++ {
		if wanted, ours := mine[h]; ours {
			if wanted && showsAt(h, x, y) {
				return false, true
			}
		} else if showsAt(h, x, y) && !passesPointer(h) {
			return true, true
		}
		h = _GetWindow(h, _GW_HWNDNEXT)
	}
	return false, false
}

// showsAt reports whether window h shows at the screen point x, y.
func showsAt(h windows.HWND, x, y int) bool {
	if !_IsWindowVisible(h) || _IsIconic(h) || cloaked(h) {
		return false
	}
	r, ok := frameBounds(h)
	return ok && int32(x) >= r.left && int32(x) < r.right && int32(y) >= r.top && int32(y) < r.bottom
}

// passesPointer reports whether window h lets the pointer through to
// the windows under it.
func passesPointer(h windows.HWND) bool {
	ex, err := _GetWindowLongW(h, _GWL_EXSTYLE)
	return err == nil && ex&_WS_EX_TRANSPARENT != 0
}

// The window attributes DwmGetWindowAttribute reads.
const (
	_DWMWA_EXTENDED_FRAME_BOUNDS = 9
	_DWMWA_CLOAKED               = 14
)

var procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")

// cloaked reports whether the system hides window h while it counts as
// shown, as it does windows on another virtual desktop.
func cloaked(h windows.HWND) bool {
	var v uint32
	r, _, _ := procDwmGetWindowAttribute.Call(uintptr(h), _DWMWA_CLOAKED, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return r == 0 && v != 0
}

// frameBounds is the rectangle window h shows on the screen: without the
// invisible borders a window on Windows 10 and 11 is resized by, where
// the system can say.
func frameBounds(h windows.HWND) (_RECT, bool) {
	var r _RECT
	if res, _, _ := procDwmGetWindowAttribute.Call(uintptr(h), _DWMWA_EXTENDED_FRAME_BOUNDS, uintptr(unsafe.Pointer(&r)), unsafe.Sizeof(r)); res == 0 {
		return r, true
	}
	r, err := _GetWindowRect(h)
	return r, err == nil
}
