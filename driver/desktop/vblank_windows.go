//go:build windows

package desktop

import (
	"unsafe"

	"github.com/marrasen/gunim/internal/glfw"
	"golang.org/x/sys/windows"
)

// swapInterval is 0 on Windows: GLFW's vsync there calls DwmFlush,
// which returns on DWM's next composition of any monitor, so a 60 Hz
// window beside a 120 Hz monitor would draw at 120 Hz. vblank waits on
// the window's own monitor instead.
const swapInterval = 0

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	gdi32                          = windows.NewLazySystemDLL("gdi32.dll")
	procMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	procCreateDCW                  = gdi32.NewProc("CreateDCW")
	procDeleteDC                   = gdi32.NewProc("DeleteDC")
	procD3DKMTOpenAdapterFromHdc   = gdi32.NewProc("D3DKMTOpenAdapterFromHdc")
	procD3DKMTCloseAdapter         = gdi32.NewProc("D3DKMTCloseAdapter")
	procD3DKMTWaitForVerticalBlank = gdi32.NewProc("D3DKMTWaitForVerticalBlankEvent")
)

// vblank waits for the vertical blank of the monitor holding a window.
// It belongs to the window's render thread.
type vblank struct {
	hwnd    windows.HWND
	monitor uintptr
	adapter uint32
	source  uint32
}

type monitorInfoEx struct {
	cbSize  uint32
	monitor [4]int32
	work    [4]int32
	flags   uint32
	device  [32]uint16
}

type openAdapterFromHdc struct {
	hdc     uintptr
	adapter uint32
	luid    [2]uint32
	source  uint32
}

type waitForVerticalBlank struct {
	adapter uint32
	device  uint32
	source  uint32
}

func newVBlank(gw *glfw.Window) *vblank {
	hwnd, err := gw.GetWin32Window()
	if err != nil {
		return &vblank{}
	}
	return &vblank{hwnd: hwnd}
}

// wait blocks until the next vertical blank of the window's monitor. It
// reports false when it could not wait, which leaves pace to hold the
// window to its rate.
func (v *vblank) wait() bool {
	const monitorDefaultToNearest = 2
	if v.hwnd == 0 {
		return false
	}
	m, _, _ := procMonitorFromWindow.Call(uintptr(v.hwnd), monitorDefaultToNearest)
	if m == 0 {
		return false
	}
	if m != v.monitor || v.adapter == 0 {
		v.close()
		mi := monitorInfoEx{cbSize: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		if ok, _, _ := procGetMonitorInfoW.Call(m, uintptr(unsafe.Pointer(&mi))); ok == 0 {
			return false
		}
		dc, _, _ := procCreateDCW.Call(0, uintptr(unsafe.Pointer(&mi.device[0])), 0, 0)
		if dc == 0 {
			return false
		}
		oa := openAdapterFromHdc{hdc: dc}
		status, _, _ := procD3DKMTOpenAdapterFromHdc.Call(uintptr(unsafe.Pointer(&oa)))
		_, _, _ = procDeleteDC.Call(dc)
		if status != 0 {
			return false
		}
		v.monitor, v.adapter, v.source = m, oa.adapter, oa.source
	}
	w := waitForVerticalBlank{adapter: v.adapter, source: v.source}
	status, _, _ := procD3DKMTWaitForVerticalBlank.Call(uintptr(unsafe.Pointer(&w)))
	return status == 0
}

func (v *vblank) close() {
	if v.adapter == 0 {
		return
	}
	a := v.adapter
	_, _, _ = procD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&a)))
	v.adapter = 0
}
