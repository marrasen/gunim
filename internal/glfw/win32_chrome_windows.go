// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It lets a window draw its own
// title bar while Windows goes on doing what a title bar does: moving
// and snapping by the caption, resizing by the edges, the shadow, and
// on Windows 11 the snap layouts over the maximize button.
//
// The window keeps its caption and thick frame styles, which is what
// gives it those, and answers WM_NCCALCSIZE so the client area covers
// the whole window. WM_NCHITTEST then says what each point is: an edge
// from the frame's thickness, and the caption and the maximize button
// from the application, through HitTestCallback.

package glfw

import (
	"unsafe"

	"github.com/marrasen/gunim/internal/winver"
)

const (
	_WM_NCCALCSIZE      = 0x0083
	_WM_NCHITTEST       = 0x0084
	_WM_NCMOUSEMOVE     = 0x00A0
	_WM_NCLBUTTONDOWN   = 0x00A1
	_WM_NCLBUTTONUP     = 0x00A2
	_WM_NCLBUTTONDBLCLK = 0x00A3
	_WM_NCMOUSELEAVE    = 0x02A2

	_HTCAPTION     = 2
	_HTMAXBUTTON   = 9
	_HTLEFT        = 10
	_HTRIGHT       = 11
	_HTTOP         = 12
	_HTTOPLEFT     = 13
	_HTTOPRIGHT    = 14
	_HTBOTTOM      = 15
	_HTBOTTOMLEFT  = 16
	_HTBOTTOMRIGHT = 17

	_SM_CXFRAME        = 32
	_SM_CYFRAME        = 33
	_SM_CXPADDEDBORDER = 92

	_TME_NONCLIENT = 0x00000010

	// _DWMWA_WINDOW_CORNER_PREFERENCE asks Windows 11 how to draw the
	// window's corners: _DWMWCP_ROUND round, _DWMWCP_DONOTROUND square.
	_DWMWA_WINDOW_CORNER_PREFERENCE = 33
	_DWMWCP_DONOTROUND              = 1
	_DWMWCP_ROUND                   = 2
)

var procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

// roundCorners asks Windows 11 to round the window's corners, as it
// does its own windows', or with round unset to square them, as for a
// window filling its monitor. Earlier versions have no round corners
// and refuse, which leaves the window as it was.
func (w *Window) roundCorners(round bool) {
	if procDwmSetWindowAttribute.Find() != nil {
		return
	}
	pref := uint32(_DWMWCP_DONOTROUND)
	if round {
		pref = _DWMWCP_ROUND
	}
	_, _, _ = procDwmSetWindowAttribute.Call(uintptr(w.platform.handle), _DWMWA_WINDOW_CORNER_PREFERENCE,
		uintptr(unsafe.Pointer(&pref)), unsafe.Sizeof(pref))
}

type _NCCALCSIZE_PARAMS struct {
	rgrc  [3]_RECT
	lppos uintptr
}

// adjustRect is AdjustWindowRectEx, for a window that has a frame. A
// chromeless window has none: its window is its client area.
func (w *Window) adjustRect(rect *_RECT, style uint32, menu bool, exStyle uint32) error {
	if w.platform.chromeless {
		return nil
	}
	return _AdjustWindowRectEx(rect, style, menu, exStyle)
}

// adjustRectForDpi is AdjustWindowRectExForDpi, as adjustRect is.
func (w *Window) adjustRectForDpi(rect *_RECT, style uint32, menu bool, exStyle uint32, dpi uint32) error {
	if w.platform.chromeless {
		return nil
	}
	return _AdjustWindowRectExForDpi(rect, style, menu, exStyle, dpi)
}

func (w *Window) platformSetChromeless(on bool) error {
	if w.platform.chromeless == on {
		return nil
	}
	width, height, err := w.platformGetWindowSize()
	if err != nil {
		return err
	}
	w.platform.chromeless = on
	if on {
		w.roundCorners(w.monitor == nil)
	}
	// The frame is worked out again, and the window keeps the size of
	// its content.
	if err := _SetWindowPos(w.platform.handle, _HWND_TOP, 0, 0, 0, 0,
		_SWP_FRAMECHANGED|_SWP_NOACTIVATE|_SWP_NOZORDER|_SWP_NOMOVE|_SWP_NOSIZE); err != nil {
		return err
	}
	return w.platformSetWindowSize(width, height)
}

// frameThickness is how far a sizing frame reaches, in pixels, at the
// window's DPI.
func (w *Window) frameThickness() (x, y int32) {
	dpi := _GetDpiForWindow(w.platform.handle)
	metric := func(i int32) int32 {
		if winver.IsWindows10AnniversaryUpdateOrGreater() {
			v, _ := _GetSystemMetricsForDpi(i, dpi)
			return v
		}
		v, _ := _GetSystemMetrics(i)
		return v
	}
	pad := metric(_SM_CXPADDEDBORDER)
	return metric(_SM_CXFRAME) + pad, metric(_SM_CYFRAME) + pad
}

// chromeMessage handles the messages a chromeless window answers itself,
// and reports false for the rest.
func (w *Window) chromeMessage(uMsg uint32, wParam _WPARAM, lParam _LPARAM) (uintptr, bool) {
	if !w.platform.chromeless {
		return 0, false
	}
	switch uMsg {
	case _WM_NCCALCSIZE:
		if wParam == 0 {
			return 0, false
		}
		// The client area is the whole window. Maximized, the window
		// reaches past the screen by the frame's thickness, which the
		// client area leaves out.
		if _IsZoomed(w.platform.handle) {
			params := (*_NCCALCSIZE_PARAMS)(unsafe.Pointer(lParam))
			fx, fy := w.frameThickness()
			params.rgrc[0].left += fx
			params.rgrc[0].top += fy
			params.rgrc[0].right -= fx
			params.rgrc[0].bottom -= fy
		}
		return 0, true

	case _WM_NCHITTEST:
		return uintptr(w.chromeHit(lParam)), true

	case _WM_NCMOUSEMOVE, _WM_NCLBUTTONDOWN, _WM_NCLBUTTONUP, _WM_NCLBUTTONDBLCLK:
		if wParam != _HTMAXBUTTON {
			if w.platform.overMaximize {
				w.platform.overMaximize = false
				w.inputCursorEnter(false)
			}
			return 0, false
		}
		// The maximize button is the application's to draw and to
		// press, though Windows sees it as the button, for the snap
		// layouts. Its pointer goes to the application as the client
		// area's does.
		x, y := w.clientPoint(lParam)
		if !w.platform.overMaximize {
			w.platform.overMaximize = true
			w.inputCursorEnter(true)
			tme := _TRACKMOUSEEVENT{
				cbSize:    uint32(unsafe.Sizeof(_TRACKMOUSEEVENT{})),
				dwFlags:   _TME_LEAVE | _TME_NONCLIENT,
				hwndTrack: w.platform.handle,
			}
			_ = _TrackMouseEvent(&tme)
		}
		w.inputCursorPos(float64(x), float64(y))
		switch uMsg {
		case _WM_NCLBUTTONDOWN, _WM_NCLBUTTONDBLCLK:
			w.inputMouseClick(MouseButtonLeft, Press, getKeyMods())
		case _WM_NCLBUTTONUP:
			w.inputMouseClick(MouseButtonLeft, Release, getKeyMods())
		}
		return 0, true

	case _WM_NCMOUSELEAVE:
		if w.platform.overMaximize {
			w.platform.overMaximize = false
			w.inputCursorEnter(false)
		}
		return 0, false
	}
	return 0, false
}

// clientPoint is a message's screen point in client coordinates.
func (w *Window) clientPoint(lParam _LPARAM) (x, y int32) {
	p := _POINT{x: int32(int16(_LOWORD(uint32(lParam)))), y: int32(int16(_HIWORD(uint32(lParam))))}
	_ = _ScreenToClient(w.platform.handle, &p)
	return p.x, p.y
}

// chromeHit is what the point in a WM_NCHITTEST is: an edge to size the
// window by, unless it is maximized, and otherwise what the application
// says.
func (w *Window) chromeHit(lParam _LPARAM) int {
	x, y := w.clientPoint(lParam)
	width, height, err := w.platformGetWindowSize()
	if err != nil {
		return _HTCLIENT
	}
	if !_IsZoomed(w.platform.handle) && w.resizable {
		fx, fy := w.frameThickness()
		left, right := x < fx, x >= int32(width)-fx
		top, bottom := y < fy, y >= int32(height)-fy
		switch {
		case top && left:
			return _HTTOPLEFT
		case top && right:
			return _HTTOPRIGHT
		case bottom && left:
			return _HTBOTTOMLEFT
		case bottom && right:
			return _HTBOTTOMRIGHT
		case left:
			return _HTLEFT
		case right:
			return _HTRIGHT
		case top:
			return _HTTOP
		case bottom:
			return _HTBOTTOM
		}
	}
	if w.hitTest != nil {
		switch w.hitTest(w, int(x), int(y)) {
		case HitCaption:
			return _HTCAPTION
		case HitMaximize:
			return _HTMAXBUTTON
		case HitClient:
		}
	}
	return _HTCLIENT
}

func (w *Window) platformChromeless() bool { return w.platform.chromeless }

func (w *Window) platformStartMoveResize(direction int) error {
	// Windows sizes and moves from the hit test: nothing to start.
	return nil
}
