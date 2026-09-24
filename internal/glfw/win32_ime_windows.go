// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It gives the Win32 port the
// input-method API of the X11 one (x11_ime_linbsd.go), through IMM32.

package glfw

import (
	"structs"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	_WM_IME_STARTCOMPOSITION = 0x010D
	_WM_IME_ENDCOMPOSITION   = 0x010E
	_WM_IME_COMPOSITION      = 0x010F

	_GCS_COMPSTR   = 0x0008
	_GCS_COMPATTR  = 0x0010
	_GCS_CURSORPOS = 0x0080
	_GCS_RESULTSTR = 0x0800

	_ATTR_TARGET_CONVERTED    = 0x01
	_ATTR_TARGET_NOTCONVERTED = 0x03

	_NI_COMPOSITIONSTR = 0x0015
	_CPS_CANCEL        = 0x0004

	_CFS_POINT   = 0x0002
	_CFS_EXCLUDE = 0x0080

	_IACE_DEFAULT = 0x0010
)

type _HIMC uintptr

type _COMPOSITIONFORM struct {
	_            structs.HostLayout
	dwStyle      uint32
	ptCurrentPos _POINT
	rcArea       _RECT
}

type _CANDIDATEFORM struct {
	_            structs.HostLayout
	dwIndex      uint32
	dwStyle      uint32
	ptCurrentPos _POINT
	rcArea       _RECT
}

var (
	imm32 = windows.NewLazySystemDLL("imm32.dll")

	procImmAssociateContextEx    = imm32.NewProc("ImmAssociateContextEx")
	procImmGetCompositionStringW = imm32.NewProc("ImmGetCompositionStringW")
	procImmGetContext            = imm32.NewProc("ImmGetContext")
	procImmNotifyIME             = imm32.NewProc("ImmNotifyIME")
	procImmReleaseContext        = imm32.NewProc("ImmReleaseContext")
	procImmSetCandidateWindow    = imm32.NewProc("ImmSetCandidateWindow")
	procImmSetCompositionWindow  = imm32.NewProc("ImmSetCompositionWindow")
)

// immAvailable reports whether IMM32 can be called. It is missing on
// some editions of Windows, such as the one on Xbox.
func immAvailable() bool {
	return imm32.Load() == nil
}

func _ImmGetContext(hWnd windows.HWND) _HIMC {
	r, _, _ := procImmGetContext.Call(uintptr(hWnd))
	return _HIMC(r)
}

func _ImmReleaseContext(hWnd windows.HWND, hIMC _HIMC) {
	_, _, _ = procImmReleaseContext.Call(uintptr(hWnd), uintptr(hIMC))
}

// _ImmGetCompositionStringW returns what the call returns: the size in
// bytes of the value asked for, or for GCS_CURSORPOS the position.
// It is negative on failure.
func _ImmGetCompositionStringW(hIMC _HIMC, index uint32, buf unsafe.Pointer, size uint32) int32 {
	r, _, _ := procImmGetCompositionStringW.Call(uintptr(hIMC), uintptr(index), uintptr(buf), uintptr(size))
	return int32(r)
}

func _ImmNotifyIME(hIMC _HIMC, action, index, value uint32) {
	_, _, _ = procImmNotifyIME.Call(uintptr(hIMC), uintptr(action), uintptr(index), uintptr(value))
}

func _ImmSetCompositionWindow(hIMC _HIMC, form *_COMPOSITIONFORM) {
	_, _, _ = procImmSetCompositionWindow.Call(uintptr(hIMC), uintptr(unsafe.Pointer(form)))
}

func _ImmSetCandidateWindow(hIMC _HIMC, form *_CANDIDATEFORM) {
	_, _, _ = procImmSetCandidateWindow.Call(uintptr(hIMC), uintptr(unsafe.Pointer(form)))
}

func _ImmAssociateContextEx(hWnd windows.HWND, hIMC _HIMC, flags uint32) {
	_, _, _ = procImmAssociateContextEx.Call(uintptr(hWnd), uintptr(hIMC), uintptr(flags))
}

// PreeditCallback is called when the composition the input method shows
// changes. selStartInBytes and selEndInBytes delimit the highlighted part of
// text, and are both the caret position when nothing is highlighted. An empty
// text means the composition has ended.
type PreeditCallback func(w *Window, text string, selStartInBytes, selEndInBytes int)

// TextInputCallback is called with text produced by the keyboard input path,
// which is either committed by the input method or typed directly.
type TextInputCallback func(w *Window, text string)

// TextInputActiveCallback reports whether the application is taking text
// input.
type TextInputActiveCallback func(w *Window) bool

// SetPreeditCallback sets the callback reporting composition updates, and
// returns the previously set one.
//
// Compositions are reported only while the application is taking text input.
// Otherwise the input method draws the composition itself.
func (w *Window) SetPreeditCallback(cbfun PreeditCallback) (PreeditCallback, error) {
	if !_glfw.initialized {
		return nil, NotInitialized
	}
	old := w.platform.preeditCallback
	w.platform.preeditCallback = cbfun
	return old, nil
}

// SetTextInputCallback sets the callback reporting committed text, and returns
// the previously set one.
func (w *Window) SetTextInputCallback(cbfun TextInputCallback) (TextInputCallback, error) {
	if !_glfw.initialized {
		return nil, NotInitialized
	}
	old := w.platform.textInputCallback
	w.platform.textInputCallback = cbfun
	return old, nil
}

// SetTextInputActiveCallback sets the callback reporting whether the
// application is taking text input, and returns the previously set one.
//
// While the application is taking text input, the window takes the input
// method's composition and draws it itself, and the key presses the input
// method takes are not reported.
func (w *Window) SetTextInputActiveCallback(cbfun TextInputActiveCallback) (TextInputActiveCallback, error) {
	if !_glfw.initialized {
		return nil, NotInitialized
	}
	old := w.platform.textInputActiveCallback
	w.platform.textInputActiveCallback = cbfun
	return old, nil
}

// SetInputMethodEnabled turns the input method on or off for the window. A
// window without one takes key presses as typed, which suits a window that is
// not taking text: the input method would otherwise hold the key presses it
// composes with.
//
// SetInputMethodEnabled must be called from the main thread.
func (w *Window) SetInputMethodEnabled(enabled bool) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	if !immAvailable() {
		return nil
	}
	if enabled {
		_ImmAssociateContextEx(w.platform.handle, 0, _IACE_DEFAULT)
	} else {
		_ImmAssociateContextEx(w.platform.handle, 0, 0)
	}
	return nil
}

// SetInputMethodCaret tells the input method where the text caret is, in
// client-area pixels, so that its windows open beside it: the candidate window
// opens next to the caret without covering it.
//
// SetInputMethodCaret must be called from the main thread.
func (w *Window) SetInputMethodCaret(x, y, width, height int) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	w.platform.imeCaret = _RECT{
		left:   int32(x),
		top:    int32(y),
		right:  int32(x + width),
		bottom: int32(y + height),
	}
	w.placeInputMethod()
	return nil
}

// placeInputMethod moves the input method's windows to the caret.
//
// placeInputMethod must be called from the main thread.
func (w *Window) placeInputMethod() {
	if !immAvailable() {
		return
	}
	himc := _ImmGetContext(w.platform.handle)
	if himc == 0 {
		return
	}
	defer _ImmReleaseContext(w.platform.handle, himc)

	caret := w.platform.imeCaret
	pos := _POINT{x: caret.left, y: caret.top}
	_ImmSetCompositionWindow(himc, &_COMPOSITIONFORM{
		dwStyle:      _CFS_POINT,
		ptCurrentPos: pos,
	})
	_ImmSetCandidateWindow(himc, &_CANDIDATEFORM{
		dwStyle:      _CFS_EXCLUDE,
		ptCurrentPos: pos,
		rcArea:       caret,
	})
}

// ResetInputContext discards the composition the input method holds. The
// discarded composition is not reported.
//
// ResetInputContext must be called from the main thread.
func (w *Window) ResetInputContext() error {
	if !_glfw.initialized {
		return NotInitialized
	}
	// The input method reports the cancelled composition as cleared while
	// the call runs. Forgetting it first keeps that from being reported.
	w.platform.preeditText, w.platform.preeditStart, w.platform.preeditEnd = "", 0, 0
	if !immAvailable() {
		return nil
	}
	himc := _ImmGetContext(w.platform.handle)
	if himc == 0 {
		return nil
	}
	_ImmNotifyIME(himc, _NI_COMPOSITIONSTR, _CPS_CANCEL, 0)
	_ImmReleaseContext(w.platform.handle, himc)
	return nil
}

// textInputActive reports whether the application is taking text input.
func (w *Window) textInputActive() bool {
	if w.platform.textInputActiveCallback == nil {
		return false
	}
	return w.platform.textInputActiveCallback(w)
}

// inputText reports text produced by the keyboard input path. plain reports
// whether the text was produced without a modifier combination the platform
// treats as a shortcut. Text from a shortcut chord, and control characters,
// are dropped: neither is text the application can insert.
func (w *Window) inputText(text string, plain bool) {
	if !plain || w.platform.textInputCallback == nil {
		return
	}
	filtered := make([]rune, 0, len(text))
	for _, r := range text {
		if isTextCodepoint(r) {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return
	}
	w.platform.textInputCallback(w, string(filtered))
}

// handleIMEMessage handles an input-method message, and reports whether it
// did. One it does not handle goes on to DefWindowProc, whose input method
// window then draws the composition and turns the result into WM_CHAR.
//
// handleIMEMessage must be called from the main thread.
func (w *Window) handleIMEMessage(uMsg uint32, lParam _LPARAM) bool {
	if !immAvailable() || !w.textInputActive() {
		return false
	}
	switch uMsg {
	case _WM_IME_STARTCOMPOSITION:
		w.placeInputMethod()
	case _WM_IME_COMPOSITION:
		himc := _ImmGetContext(w.platform.handle)
		if himc == 0 {
			return true
		}
		defer _ImmReleaseContext(w.platform.handle, himc)
		if lParam&_GCS_RESULTSTR != 0 {
			if s, _ := compositionString(himc, _GCS_RESULTSTR); len(s) > 0 {
				w.reportPreedit("", 0, 0)
				w.inputText(string(utf16.Decode(s)), true)
			}
		}
		// The composition is read whatever the message says changed: a
		// message with no flags at all is the input method clearing it.
		text, start, end := readComposition(himc)
		w.reportPreedit(text, start, end)
	case _WM_IME_ENDCOMPOSITION:
		w.reportPreedit("", 0, 0)
	default:
		return false
	}
	return true
}

// reportPreedit reports a composition, unless it is the one reported last.
func (w *Window) reportPreedit(text string, start, end int) {
	p := &w.platform
	if text == p.preeditText && start == p.preeditStart && end == p.preeditEnd {
		return
	}
	p.preeditText, p.preeditStart, p.preeditEnd = text, start, end
	if p.preeditCallback != nil {
		p.preeditCallback(w, text, start, end)
	}
}

// readComposition returns the input method's composition, with its highlighted
// clause or else its caret as a range in bytes of the text.
func readComposition(himc _HIMC) (text string, startInBytes, endInBytes int) {
	s, ok := compositionString(himc, _GCS_COMPSTR)
	if !ok || len(s) == 0 {
		return "", 0, 0
	}
	// The attributes are one byte per UTF-16 unit of the composition.
	var attrs []byte
	if n := _ImmGetCompositionStringW(himc, _GCS_COMPATTR, nil, 0); n > 0 {
		attrs = make([]byte, n)
		if _ImmGetCompositionStringW(himc, _GCS_COMPATTR, unsafe.Pointer(&attrs[0]), uint32(n)) != n {
			attrs = nil
		}
	}
	first, last := targetClause(attrs, len(s))
	if first < 0 {
		caret := int(_ImmGetCompositionStringW(himc, _GCS_CURSORPOS, nil, 0))
		first = min(max(caret, 0), len(s))
		last = first
	}
	text = string(utf16.Decode(s))
	return text, utf16Bytes(s[:first]), utf16Bytes(s[:last])
}

// targetClause returns the range of UTF-16 units of the clause the input
// method is converting, or -1 when there is none.
func targetClause(attrs []byte, n int) (first, last int) {
	first, last = -1, -1
	for i := 0; i < min(len(attrs), n); i++ {
		if attrs[i] == _ATTR_TARGET_CONVERTED || attrs[i] == _ATTR_TARGET_NOTCONVERTED {
			if first < 0 {
				first = i
			}
			last = i + 1
			continue
		}
		if first >= 0 {
			break
		}
	}
	return first, last
}

// utf16Bytes returns the length in UTF-8 of UTF-16 text.
func utf16Bytes(s []uint16) int {
	return len(string(utf16.Decode(s)))
}

// compositionString returns a string of the input method's composition, such
// as GCS_COMPSTR or GCS_RESULTSTR, in UTF-16 units.
func compositionString(himc _HIMC, index uint32) ([]uint16, bool) {
	n := _ImmGetCompositionStringW(himc, index, nil, 0)
	if n < 0 {
		return nil, false
	}
	if n == 0 {
		return nil, true
	}
	buf := make([]uint16, n/2)
	if got := _ImmGetCompositionStringW(himc, index, unsafe.Pointer(&buf[0]), uint32(n)); got != n {
		return nil, false
	}
	return buf, true
}
