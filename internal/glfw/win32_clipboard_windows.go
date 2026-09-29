// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. The Ebitengine port left the Win32
// clipboard unimplemented, and panicked on any use of it. This follows
// C GLFW's win32_window.c.

package glfw

import (
	"bytes"
	"fmt"
	"image/png"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	_CF_UNICODETEXT = 13
	_GMEM_MOVEABLE  = 0x0002
)

var (
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

// openClipboard opens the clipboard for the helper window. Another
// process may hold it for a moment, so it tries a few times.
func openClipboard() error {
	var err error
	for range 3 {
		r, _, e := procOpenClipboard.Call(uintptr(_glfw.platformWindow.helperWindowHandle))
		if r != 0 {
			return nil
		}
		err = e
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("glfw: failed to open clipboard: %w", err)
}

func closeClipboard() {
	_, _, _ = procCloseClipboard.Call()
}

func platformSetClipboardString(str string) error {
	text, err := windows.UTF16FromString(str)
	if err != nil {
		return fmt.Errorf("glfw: clipboard string contains a NUL: %w", err)
	}
	size := uintptr(len(text)) * unsafe.Sizeof(text[0])

	object, _, e := procGlobalAlloc.Call(_GMEM_MOVEABLE, size)
	if object == 0 {
		return fmt.Errorf("glfw: failed to allocate global handle for clipboard: %w", e)
	}
	buffer, _, e := procGlobalLock.Call(object)
	if buffer == 0 {
		_, _, _ = procGlobalFree.Call(object)
		return fmt.Errorf("glfw: failed to lock global handle: %w", e)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(buffer)), len(text)), text)
	_, _, _ = procGlobalUnlock.Call(object)

	if err := openClipboard(); err != nil {
		_, _, _ = procGlobalFree.Call(object)
		return err
	}
	defer closeClipboard()
	_, _, _ = procEmptyClipboard.Call()
	// The clipboard owns the memory once this succeeds.
	if r, _, e := procSetClipboardData.Call(_CF_UNICODETEXT, object); r == 0 {
		_, _, _ = procGlobalFree.Call(object)
		return fmt.Errorf("glfw: failed to set clipboard data: %w", e)
	}
	return nil
}

func platformGetClipboardString() (string, error) {
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer closeClipboard()

	object, _, _ := procGetClipboardData.Call(_CF_UNICODETEXT)
	if object == 0 {
		return "", fmt.Errorf("glfw: failed to convert clipboard to string: %w", FormatUnavailable)
	}
	buffer, _, e := procGlobalLock.Call(object)
	if buffer == 0 {
		return "", fmt.Errorf("glfw: failed to lock global handle: %w", e)
	}
	defer procGlobalUnlock.Call(object)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(buffer))), nil
}

// Clipboard formats that hold a picture, besides PNG, which has no number of its own.
const (
	_CF_DIB   = 8
	_CF_DIBV5 = 17
)

var (
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
)

// platformGetClipboardImage returns the clipboard's picture as PNG: its PNG as it is, which browsers and the
// Snipping Tool put there, or else its bitmap, which Print Screen puts there, encoded.
func platformGetClipboardImage() ([]byte, error) {
	name, err := windows.UTF16PtrFromString("PNG")
	if err != nil {
		return nil, err
	}
	pngFormat, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	if err := openClipboard(); err != nil {
		return nil, err
	}
	defer closeClipboard()
	if pngFormat != 0 {
		data, ok, err := clipboardBytes(pngFormat)
		if err != nil || ok {
			return data, err
		}
	}
	for _, format := range []uintptr{_CF_DIBV5, _CF_DIB} {
		data, ok, err := clipboardBytes(format)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		img, err := decodeDIB(data)
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		if err := enc.Encode(&b, img); err != nil {
			return nil, fmt.Errorf("glfw: encoding the clipboard's picture: %w", err)
		}
		return b.Bytes(), nil
	}
	return nil, nil
}

// clipboardBytes returns a copy of the open clipboard's data in format, and false when it holds none.
func clipboardBytes(format uintptr) ([]byte, bool, error) {
	if r, _, _ := procIsClipboardFormatAvailable.Call(format); r == 0 {
		return nil, false, nil
	}
	object, _, e := procGetClipboardData.Call(format)
	if object == 0 {
		return nil, false, fmt.Errorf("glfw: failed to get clipboard data: %w", e)
	}
	size, _, _ := procGlobalSize.Call(object)
	buffer, _, e := procGlobalLock.Call(object)
	if buffer == 0 {
		return nil, false, fmt.Errorf("glfw: failed to lock global handle: %w", e)
	}
	defer procGlobalUnlock.Call(object)
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(buffer)), size))
	return out, true, nil
}
