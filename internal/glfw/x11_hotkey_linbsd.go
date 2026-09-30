//go:build freebsd || linux || netbsd

package glfw

import (
	"errors"

	"github.com/ebitengine/purego"
)

// Keys for every program on X11, a gunim addition: XGrabKey on the root
// window, on glfw's own display and thread, and the presses caught in
// processEvent before it looks for a window. Each key is grabbed with
// and without Caps Lock and Num Lock, which X counts as modifiers.

var (
	xGrabKey         func(display uintptr, keycode int32, modifiers uint32, window _XID, ownerEvents bool, pointerMode, keyboardMode int32) int32
	xUngrabKey       func(display uintptr, keycode int32, modifiers uint32, window _XID) int32
	xKeysymToKeycode func(display uintptr, keysym _KeySym) uint8
	x11HotKeysBound  bool
	x11HotKeys       = map[uintptr]x11HotKey{}
	x11HotKeyNext    uintptr
)

// x11HotKey is a key grabbed: its keycode and modifiers, and what a
// press calls.
type x11HotKey struct {
	keycode int32
	mods    uint32
	fn      func()
	// down says the key is held, so the presses a held key repeats are
	// not heard again until it is let go, as on Windows.
	down bool
}

// x11LockMasks are the modifiers a key is grabbed under as well, so it
// works whatever Caps Lock and Num Lock are.
var x11LockMasks = []uint32{0, _LockMask, _Mod2Mask, _LockMask | _Mod2Mask}

// ErrX11HotKeyTaken says another program has grabbed the key.
var ErrX11HotKeyTaken = errors.New("glfw: x11: another program has the key")

// X11 modifier masks, as RegisterHotKeyX11 takes them.
const (
	X11Shift   = _ShiftMask
	X11Control = 1 << 2
	X11Alt     = 1 << 3
	X11Super   = _Mod4Mask
)

// badAccess is X's BadAccess, what grabbing a key grabbed already gives.
const badAccess = 10

// RegisterHotKeyX11 calls fn each time the key keysym is pressed with
// mods, whatever window has the keyboard, and returns the key's number
// for UnregisterHotKeyX11. It must be called on the main thread, and fn
// is called there.
func RegisterHotKeyX11(mods uint32, keysym uint32) (uintptr, func(fn func()), error) {
	if !_glfw.initialized {
		return 0, nil, NotInitialized
	}
	if !x11HotKeysBound {
		purego.RegisterLibFunc(&xGrabKey, libX11, "XGrabKey")
		purego.RegisterLibFunc(&xUngrabKey, libX11, "XUngrabKey")
		purego.RegisterLibFunc(&xKeysymToKeycode, libX11, "XKeysymToKeycode")
		x11HotKeysBound = true
	}
	display, root := _glfw.platformWindow.display, _glfw.platformWindow.root
	keycode := int32(xKeysymToKeycode(display, _KeySym(keysym)))
	if keycode == 0 {
		return 0, nil, errors.New("glfw: x11: no key on this keyboard makes that key")
	}
	for _, k := range x11HotKeys {
		if k.keycode == keycode && k.mods == mods {
			// Grabbed already here: as another program's, as Windows has it.
			return 0, nil, ErrX11HotKeyTaken
		}
	}
	grabErrorHandlerX11()
	for _, lock := range x11LockMasks {
		xGrabKey(display, keycode, mods|lock, root, false, _GrabModeAsync, _GrabModeAsync)
	}
	releaseErrorHandlerX11()
	if code := _glfw.platformWindow.errorCode; code != _Success {
		for _, lock := range x11LockMasks {
			xUngrabKey(display, keycode, mods|lock, root)
		}
		xSync(display, false)
		if code == badAccess {
			return 0, nil, ErrX11HotKeyTaken
		}
		return 0, nil, errors.New("glfw: x11: the key could not be grabbed")
	}
	x11HotKeyNext++
	id := x11HotKeyNext
	set := func(fn func()) { x11HotKeys[id] = x11HotKey{keycode: keycode, mods: mods, fn: fn} }
	return id, set, nil
}

// UnregisterHotKeyX11 lets key id go. It must be called on the main
// thread.
func UnregisterHotKeyX11(id uintptr) {
	k, ok := x11HotKeys[id]
	if !ok {
		return
	}
	delete(x11HotKeys, id)
	display, root := _glfw.platformWindow.display, _glfw.platformWindow.root
	for _, lock := range x11LockMasks {
		xUngrabKey(display, k.keycode, k.mods|lock, root)
	}
	xSync(display, false)
}

// x11HotKeyPressed calls the key grabbed that a press on the root
// window is, once until it is let go, and reports whether it was one.
func x11HotKeyPressed(keycode int32, state uint32) bool {
	state &^= _LockMask | _Mod2Mask
	for id, k := range x11HotKeys {
		if k.keycode == keycode && k.mods == state&(X11Shift|X11Control|X11Alt|X11Super) {
			if !k.down && k.fn != nil {
				k.fn()
			}
			k.down = true
			x11HotKeys[id] = k
			return true
		}
	}
	return false
}

// x11HotKeyReleased notes a key grabbed let go, and reports whether a
// release on the root window was one.
func x11HotKeyReleased(keycode int32) bool {
	found := false
	for id, k := range x11HotKeys {
		if k.keycode == keycode {
			k.down = false
			x11HotKeys[id] = k
			found = true
		}
	}
	return found
}
