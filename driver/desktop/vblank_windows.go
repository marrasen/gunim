//go:build windows

package desktop

import (
	"time"
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
//
// The wait itself runs on a goroutine of its own. When a monitor goes
// away, as when it is switched off and Windows puts a stand-in in its
// place, a wait on it can block for good, and the window would stop
// drawing. So the render thread gives up after vblankTimeout, closes
// the adapter, and paces itself by the clock, until the wait comes
// back. Only one wait is ever in flight, so a wait that never returns
// holds one goroutine and no more.
type vblank struct {
	hwnd    windows.HWND
	monitor uintptr
	adapter uint32
	source  uint32

	// req carries a wait to the waiting goroutine, and res its status
	// back. busy is set while a wait is in flight.
	req   chan waitForVerticalBlank
	res   chan uintptr
	busy  bool
	timer *time.Timer
}

// vblankTimeout is how long the render thread waits for a vertical
// blank before it gives up on the monitor: two refreshes at 20 Hz.
const vblankTimeout = 100 * time.Millisecond

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
		v.closeAdapter()
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
	if v.busy {
		// The last wait never came back. Wait for it, not a new one.
		select {
		case <-v.res:
			v.busy = false
		default:
			return false
		}
	}
	if v.req == nil {
		v.req, v.res = make(chan waitForVerticalBlank), make(chan uintptr, 1)
		v.timer = time.NewTimer(vblankTimeout)
		go vblankWaiter(v.req, v.res)
	}
	v.req <- waitForVerticalBlank{adapter: v.adapter, source: v.source}
	v.busy = true
	if !v.timer.Stop() {
		select {
		case <-v.timer.C:
		default:
		}
	}
	v.timer.Reset(vblankTimeout)
	select {
	case status := <-v.res:
		v.busy = false
		return status == 0
	case <-v.timer.C:
		// The monitor may be gone. Closing the adapter may end the wait;
		// the next frame opens the adapter of whatever monitor holds the
		// window then.
		v.closeAdapter()
		return false
	}
}

// vblankWaiter runs each wait it is handed and reports its status.
func vblankWaiter(req <-chan waitForVerticalBlank, res chan<- uintptr) {
	for w := range req {
		status, _, _ := procD3DKMTWaitForVerticalBlank.Call(uintptr(unsafe.Pointer(&w)))
		res <- status
	}
}

// close lets go of the adapter and ends the waiting goroutine once it
// is free.
func (v *vblank) close() {
	v.closeAdapter()
	if v.req != nil {
		close(v.req)
		v.req = nil
	}
}

func (v *vblank) closeAdapter() {
	if v.adapter == 0 {
		return
	}
	a := v.adapter
	_, _, _ = procD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&a)))
	v.adapter = 0
}
