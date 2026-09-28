// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It draws a chromeless window's
// shadow and border in a window of its own: a click-through layered
// window, owned by the chromeless window, that follows it from the
// window's own messages.

package glfw

import (
	"math"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	_WM_SHOWWINDOW       = 0x0018
	_WM_TIMER            = 0x0113
	_WM_WINDOWPOSCHANGED = 0x0047

	_SW_PARENTOPENING  = 3
	_SW_SHOWNOACTIVATE = 4
	_HTTRANSPARENT     = ^uintptr(0)
	_ULW_ALPHA         = 2
	_AC_SRC_ALPHA      = 1

	shadowClassName = "gunim shadow"

	// shadowTimer is the main window's timer that fades the shadow back in once the window has restored.
	shadowTimer = 0x67756e69
	// shadowRestoreDelay is how long the shadow waits for the restore animation, in milliseconds.
	shadowRestoreDelay = 150
	// shadowFadeIn is how long the shadow takes to come back after it, in milliseconds.
	shadowFadeIn = 80
	shadowTick   = 16
)

var (
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")

	shadowClassRegistered bool
)

// drawnShadow is the window a chromeless window's shadow is drawn in.
type drawnShadow struct {
	hwnd windows.HWND
	dc   _HDC
	bmp  _HBITMAP
	// measure draws solid green bands instead of a shadow, for measuring how closely it follows.
	measure bool

	// size, dpi and the window's size are what the bitmap was drawn for.
	w, h, dpi int32
	winW      int32
	winH      int32
	// left and top are how far the shadow reaches past the window's left and top edges.
	left, top int32
	x, y      int32
	alpha     uint8

	// opacity is the application's, restore the fade back in after the window restores.
	opacity float32
	restore float32

	minimized bool
}

// SetDrawnShadow draws the window's shadow and border in a window of gunim's own, or with measure set, green bands
// for measuring. The window's corners stay square, so Windows draws no shadow of its own. The shadow stays hidden
// until SetShadowOpacity shows it. It is a gunim addition, on Windows alone.
func (w *Window) SetDrawnShadow(measure bool) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	if !w.platform.chromeless || w.platform.shadow != nil {
		return nil
	}
	if !shadowClassRegistered {
		var wc _WNDCLASSEXW
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		wc.lpfnWndProc = _WNDPROC(windows.NewCallback(shadowProc))
		wc.hInstance = _glfw.platformWindow.instance
		name, err := windows.UTF16PtrFromString(shadowClassName)
		if err != nil {
			return err
		}
		wc.lpszClassName = name
		if _, err := _RegisterClassExW(&wc); err != nil {
			return err
		}
		shadowClassRegistered = true
	}
	h, err := _CreateWindowExW(_WS_EX_LAYERED|_WS_EX_TRANSPARENT|_WS_EX_TOOLWINDOW|_WS_EX_NOACTIVATE, shadowClassName, "",
		_WS_POPUP, 0, 0, 1, 1, w.platform.handle, 0, _glfw.platformWindow.instance, nil)
	if err != nil {
		return err
	}
	dc, _, e := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		_ = _DestroyWindow(h)
		return e
	}
	w.platform.shadow = &drawnShadow{hwnd: h, dc: _HDC(dc), measure: measure, restore: 1}
	w.roundCorners(false)
	return nil
}

// SetShadowOpacity shows the drawn shadow at opacity o, or hides it at 0.
func (w *Window) SetShadowOpacity(o float32) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	s := w.platform.shadow
	if s == nil {
		return nil
	}
	s.opacity = o
	return w.placeShadow()
}

// shadowProc lets the pointer through the shadow, and keeps it hidden as its window restores; see shadowMessage.
func shadowProc(hWnd windows.HWND, uMsg uint32, wParam _WPARAM, lParam _LPARAM) uintptr {
	switch uMsg {
	case _WM_NCHITTEST:
		return _HTTRANSPARENT
	case _WM_MOUSEACTIVATE:
		return _MA_NOACTIVATE
	case _WM_SHOWWINDOW:
		if wParam != 0 && lParam == _SW_PARENTOPENING {
			return 0
		}
	}
	return uintptr(_DefWindowProcW(hWnd, uMsg, wParam, lParam))
}

// shadowMessage keeps the drawn shadow with its window as it moves, sizes, minimizes and restores.
func (w *Window) shadowMessage(uMsg uint32, wParam _WPARAM) {
	s := w.platform.shadow
	if s == nil {
		return
	}
	switch uMsg {
	case _WM_WINDOWPOSCHANGED:
	case _WM_SIZE:
		switch {
		case wParam == _SIZE_MINIMIZED:
			s.minimized = true
		case s.minimized:
			// Held back through the restore animation, then faded in
			s.minimized = false
			s.restore = 0
			procSetTimer.Call(uintptr(w.platform.handle), shadowTimer, shadowRestoreDelay, 0)
		}
	case _WM_TIMER:
		if wParam != shadowTimer {
			return
		}
		s.restore = min(s.restore+float32(shadowTick)/shadowFadeIn, 1)
		if s.restore >= 1 {
			procKillTimer.Call(uintptr(w.platform.handle), shadowTimer)
		} else {
			procSetTimer.Call(uintptr(w.platform.handle), shadowTimer, shadowTick, 0)
		}
	default:
		return
	}
	if err := w.placeShadow(); err != nil {
		_glfw.errors = append(_glfw.errors, err)
	}
}

// placeShadow puts the drawn shadow round the window, drawing it again where the window's size has changed, or hides
// it where the window is hidden, minimized, maximized or fills its monitor.
func (w *Window) placeShadow() error {
	s := w.platform.shadow
	main := w.platform.handle
	alpha := s.opacity * s.restore
	if alpha <= 0 || !_IsWindowVisible(main) || _IsIconic(main) || _IsZoomed(main) || w.monitor != nil {
		if _IsWindowVisible(s.hwnd) {
			_ShowWindow(s.hwnd, _SW_HIDE)
		}
		return nil
	}
	r, err := _GetWindowRect(main)
	if err != nil {
		return err
	}
	dpi := int32(_GetDpiForWindow(main))
	winW, winH := r.right-r.left, r.bottom-r.top
	redraw := winW != s.winW || winH != s.winH || dpi != s.dpi
	if redraw {
		if err := s.draw(winW, winH, dpi); err != nil {
			return err
		}
	}
	x, y := r.left-s.left, r.top-s.top
	a := uint8(alpha*255 + 0.5)
	if redraw || a != s.alpha {
		screen, err := _GetDC(0)
		if err != nil {
			return err
		}
		pos := _POINT{x: x, y: y}
		size := [2]int32{s.w, s.h}
		src := _POINT{}
		blend := [4]byte{0, 0, a, _AC_SRC_ALPHA}
		ok, _, e := procUpdateLayeredWindow.Call(uintptr(s.hwnd), uintptr(screen), uintptr(unsafe.Pointer(&pos)),
			uintptr(unsafe.Pointer(&size)), uintptr(s.dc), uintptr(unsafe.Pointer(&src)), 0,
			uintptr(unsafe.Pointer(&blend)), _ULW_ALPHA)
		_ReleaseDC(0, screen)
		if ok == 0 {
			return e
		}
		s.alpha = a
	} else if x != s.x || y != s.y {
		if err := _SetWindowPos(s.hwnd, 0, x, y, 0, 0, _SWP_NOSIZE|_SWP_NOZORDER|_SWP_NOACTIVATE); err != nil {
			return err
		}
	}
	s.x, s.y = x, y
	if !_IsWindowVisible(s.hwnd) {
		_ShowWindow(s.hwnd, _SW_SHOWNOACTIVATE)
	}
	return nil
}

// draw draws the shadow and border for a window of winW by winH pixels at dpi, into a premultiplied bitmap: a
// rectangle offset downwards and blurred, with a grey line just outside the window.
func (s *drawnShadow) draw(winW, winH, dpi int32) error {
	k := float64(dpi) / 96
	sigma, drop, inset := 16*k, 16*k, 0.9*k
	const strength = 0.366
	side, top, bottom := int32(37*k+0.5), int32(22*k+0.5), int32(54*k+0.5)
	if s.measure {
		side, top, bottom = 24, 24, 24
	}
	W, H := winW+2*side, winH+top+bottom
	var bi _BITMAPV5HEADER
	bi.bV5Size = uint32(unsafe.Sizeof(bi))
	bi.bV5Width = W
	bi.bV5Height = -H
	bi.bV5Planes = 1
	bi.bV5BitCount = 32
	bmp, bits, err := _CreateDIBSection(0, &bi, _DIB_RGB_COLORS, 0, 0)
	if err != nil {
		return err
	}
	procSelectObject.Call(uintptr(s.dc), uintptr(bmp))
	if s.bmp != 0 {
		_ = _DeleteObject(_HGDIOBJ(s.bmp))
	}
	s.bmp = bmp
	s.w, s.h, s.dpi, s.winW, s.winH, s.left, s.top = W, H, dpi, winW, winH, side, top
	px := unsafe.Slice((*uint32)(unsafe.Pointer(bits)), int(W)*int(H))

	// The window's rectangle in the bitmap
	x0, y0, x1, y1 := side, top, side+winW, top+winH
	if s.measure {
		for y := int32(0); y < H; y++ {
			for x := int32(0); x < W; x++ {
				d := max(x0-x, x-x1+1, y0-y, y-y1+1)
				if d > 0 && d <= 16 {
					px[y*W+x] = 0xFF00FF00
				}
			}
		}
		return nil
	}
	// A blurred rectangle is the product of a blurred edge across and a blurred edge down
	blur := func(p, a, b float64) float64 {
		return 0.5 * (math.Erf((p-a)/(sigma*math.Sqrt2)) - math.Erf((p-b)/(sigma*math.Sqrt2)))
	}
	across := make([]float64, W)
	for x := range across {
		across[x] = blur(float64(x)+0.5, float64(x0)+inset, float64(x1)-inset)
	}
	for y := int32(0); y < H; y++ {
		down := strength * blur(float64(y)+0.5, float64(y0)+drop, float64(y1)+drop)
		row := px[y*W : (y+1)*W]
		for x := range row {
			xi := int32(x)
			inside := xi >= x0 && xi < x1 && y >= y0 && y < y1
			switch {
			case inside:
				row[x] = 0
			case xi >= x0-1 && xi <= x1 && y >= y0-1 && y <= y1:
				// The border: grey 0x5e at half opacity, premultiplied
				row[x] = 0x802F2F2F
			default:
				row[x] = uint32(down*across[x]*255+0.5) << 24
			}
		}
	}
	return nil
}

// dropShadow destroys the drawn shadow's window and bitmap.
func (w *Window) dropShadow() error {
	s := w.platform.shadow
	if s == nil {
		return nil
	}
	w.platform.shadow = nil
	procKillTimer.Call(uintptr(w.platform.handle), shadowTimer)
	if s.bmp != 0 {
		_ = _DeleteObject(_HGDIOBJ(s.bmp))
	}
	procDeleteDC.Call(uintptr(s.dc))
	return _DestroyWindow(s.hwnd)
}
