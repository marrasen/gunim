//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"slices"

	"github.com/marrasen/gunim/driver"
)

// Secure implements [driver.Securer] for the one activity that the windows
// share: it is secure while any of them that is open asks.
func (w *Window) Secure(on bool) error {
	w.d.mu.Lock()
	w.secure = on && !w.closed
	w.d.mu.Unlock()
	return w.d.tellSecure()
}

// errSecure is returned where Android failed to keep a window to the
// screen.
var errSecure = errors.New("android: the window is not kept out of screenshots")

// tellSecure tells Java whether what the activity shows is kept to the
// screen, and returns once it is so. The UI thread does it, and may wait
// for mu meanwhile, so mu is not held.
func (d *Driver) tellSecure() error {
	d.secureMu.Lock()
	defer d.secureMu.Unlock()
	d.mu.Lock()
	on := slices.ContainsFunc(d.windows, func(w *Window) bool { return !w.closed && w.secure })
	d.mu.Unlock()
	if !on && !d.secureSent {
		return nil
	}
	d.secureSent = on
	switch C.gunim_secure(cBool(on)) {
	case 0:
		return nil
	case 1:
		return driver.ErrOverlaysStay
	}
	return errSecure
}

// errClipboard is returned where Android takes nothing to the clipboard, and
// errUnseen where it tells nothing of it.
var (
	errClipboard = errors.New("android: the clipboard took nothing")
	errUnseen    = errors.New("android: the clipboard tells nothing while the program is out of sight")
)

// SetClipboardSensitive implements [driver.SensitiveClipboard].
func (w *Window) SetClipboardSensitive(s string) error {
	u, p := utf16Of(s)
	ok := C.gunim_set_clipboard_sensitive(p, C.int(len(u)))
	runtime.KeepAlive(u)
	if ok == 0 {
		return errClipboard
	}
	return nil
}

// ClearClipboardIf implements [driver.SensitiveClipboard].
func (w *Window) ClearClipboardIf(s string) (bool, error) {
	u, p := utf16Of(s)
	r := C.gunim_clear_clipboard_if(p, C.int(len(u)))
	runtime.KeepAlive(u)
	if r == 2 {
		return false, errUnseen
	}
	return r == 1, nil
}

var (
	_ driver.Securer            = (*Window)(nil)
	_ driver.SensitiveClipboard = (*Window)(nil)
)
