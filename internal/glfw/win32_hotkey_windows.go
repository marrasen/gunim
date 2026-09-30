package glfw

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Keys for every program, a gunim addition: RegisterHotKey, with the
// presses coming as WM_HOTKEY to a message window of gunim's own, on
// the main thread, which the event loop pumps with the rest. It is on
// Windows alone.

var (
	user32hotkey          = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey    = user32hotkey.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32hotkey.NewProc("UnregisterHotKey")
	hotKeyClassRegistered bool
	hotKeyWindow          windows.HWND
	hotKeyProcPtr         = windows.NewCallback(hotKeyProc)
	hotKeyCalls           = map[uintptr]func(){}
	hotKeyNext            uintptr
	hotKeyClassName       = "GunimHotKeys"
)

const (
	wmHotKey      = 0x0312
	modAltKey     = 0x1
	modControlKey = 0x2
	modShiftKey   = 0x4
	modWinKey     = 0x8
	modNoRepeat   = 0x4000
	// errorHotKeyAlreadyRegistered is ERROR_HOTKEY_ALREADY_REGISTERED.
	errorHotKeyAlreadyRegistered = windows.Errno(1409)
)

// ErrHotKeyTaken says another program holds the key.
var ErrHotKeyTaken = errors.New("glfw: the key is held by another program")

// HotKey mods, as RegisterHotKey takes them.
const (
	HotKeyAlt     = modAltKey
	HotKeyControl = modControlKey
	HotKeyShift   = modShiftKey
	HotKeyWin     = modWinKey
)

// RegisterHotKey calls fn each time virtual key vk is pressed with
// mods, whatever program has the keyboard, and returns the key's number
// for UnregisterHotKey. It must be called on the main thread, and fn is
// called there.
func RegisterHotKey(mods uint32, vk uint32, fn func()) (uintptr, error) {
	if !_glfw.initialized {
		return 0, NotInitialized
	}
	if hotKeyWindow == 0 {
		if !hotKeyClassRegistered {
			var wc _WNDCLASSEXW
			wc.cbSize = uint32(unsafe.Sizeof(wc))
			wc.lpfnWndProc = _WNDPROC(hotKeyProcPtr)
			wc.hInstance = _glfw.platformWindow.instance
			name, err := windows.UTF16PtrFromString(hotKeyClassName)
			if err != nil {
				return 0, err
			}
			wc.lpszClassName = name
			if _, err := _RegisterClassExW(&wc); err != nil {
				return 0, err
			}
			hotKeyClassRegistered = true
		}
		h, err := _CreateWindowExW(0, hotKeyClassName, "", 0, 0, 0, 0, 0, trayHWNDMessage, 0, _glfw.platformWindow.instance, nil)
		if err != nil {
			return 0, err
		}
		hotKeyWindow = h
	}
	hotKeyNext++
	id := hotKeyNext
	r, _, e := procRegisterHotKey.Call(uintptr(hotKeyWindow), id, uintptr(mods|modNoRepeat), uintptr(vk))
	if r == 0 {
		if errors.Is(e, errorHotKeyAlreadyRegistered) {
			return 0, ErrHotKeyTaken
		}
		return 0, e
	}
	hotKeyCalls[id] = fn
	return id, nil
}

// UnregisterHotKey lets key id go. It must be called on the main
// thread.
func UnregisterHotKey(id uintptr) {
	if _, ok := hotKeyCalls[id]; !ok {
		return
	}
	delete(hotKeyCalls, id)
	_, _, _ = procUnregisterHotKey.Call(uintptr(hotKeyWindow), id)
}

// hotKeyProc hears the keys' presses.
func hotKeyProc(hWnd windows.HWND, uMsg uint32, wParam _WPARAM, lParam _LPARAM) uintptr {
	if uMsg == wmHotKey && hWnd == hotKeyWindow {
		if fn := hotKeyCalls[uintptr(wParam)]; fn != nil {
			fn()
		}
		return 0
	}
	return uintptr(_DefWindowProcW(hWnd, uMsg, wParam, lParam))
}
