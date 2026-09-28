package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// A window shows what its surface holds from the moment it is shown,
// and before its first frame that is whatever the surface started with:
// on Windows, where the window blends with the desktop, a see-through
// window with a dark strip, for a frame. So a new window is cloaked as
// it is shown, which keeps it off the screen while it draws, and
// uncloaked once its first frame is presented.

var procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// dwmwaCloak is DWMWA_CLOAK.
const dwmwaCloak = 13

// cloak keeps w off the screen, or with on unset puts it back. It runs
// on the main thread. Where the system refuses, the window shows as it
// would have.
func cloak(w *Window, on bool) {
	if procDwmSetWindowAttribute.Find() != nil {
		return
	}
	hwnd, err := w.gw.GetWin32Window()
	if err != nil {
		return
	}
	v := uint32(0)
	if on {
		v = 1
	}
	_, _, _ = procDwmSetWindowAttribute.Call(uintptr(hwnd), dwmwaCloak, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
}
