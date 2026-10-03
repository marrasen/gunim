//go:build linux

package desktop

import (
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
)

// driverAccess is the application's AT-SPI connection, made once the
// session has accessibility on. The driver watches for that from when
// the first window opens, so a screen reader started later still finds
// the windows. a belongs to the main thread.
type driverAccess struct {
	once sync.Once
	a    *atspi
}

// windowAccess is a window's place in AT-SPI, set on the main thread
// and read by the engine's.
type windowAccess struct {
	aw atomic.Pointer[atspiWindow]
}

// watchAccess starts watching for accessibility, once. When it comes
// on, the main thread connects and registers every open window. It
// runs on the main thread.
func (d *Driver) watchAccess() {
	d.acc.once.Do(func() {
		watchAccessibility(func() {
			d.post(func() {
				if d.acc.a != nil {
					return
				}
				d.acc.a = startATSPI()
				for _, w := range d.windows {
					w.accessOpen()
					// Something now listens, so the window draws once more
					// to hand over its tree.
					w.in.Push(driver.Redraw{})
				}
			})
		})
	})
}

// accessOpen registers the window with AT-SPI, once accessibility is
// on. It runs on the main thread.
func (w *Window) accessOpen() {
	w.d.watchAccess()
	if a := w.d.acc.a; a != nil && w.acc.aw.Load() == nil {
		w.acc.aw.Store(a.add(w, w.popup))
	}
}

// accessClose unregisters the window.
func (w *Window) accessClose() {
	if aw := w.acc.aw.Swap(nil); aw != nil {
		aw.a.remove(aw)
	}
}

// accessFocus tells screen readers the window took or lost the keyboard.
func (w *Window) accessFocus(in bool) {
	if aw := w.acc.aw.Load(); aw != nil {
		aw.a.focused(aw, in)
	}
}

// AccessWanted implements [driver.AccessPublisher].
func (w *Window) AccessWanted() bool { return w.acc.aw.Load() != nil }

// PublishAccess implements [driver.AccessPublisher].
func (w *Window) PublishAccess(t *access.Tree) {
	if aw := w.acc.aw.Load(); aw != nil {
		aw.a.publish(aw, t)
	}
}
