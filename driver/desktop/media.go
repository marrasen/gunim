//go:build linux || windows

package desktop

import (
	"time"

	"github.com/marrasen/gunim/input"
)

// mediaWindow returns the window the system's media controls speak to:
// the one used last, or any window that is no popup. It runs on the
// main thread.
func (d *Driver) mediaWindow() *Window {
	if d.lastUsed != nil {
		return d.lastUsed
	}
	for _, w := range d.windows {
		if w.parent == nil {
			return w
		}
	}
	return nil
}

// mediaKey takes a press of a button of the system's media controls to
// the window they speak to, as its media key. It may be called from any
// goroutine.
func (d *Driver) mediaKey(k input.Key) {
	d.post(func() {
		if w := d.mediaWindow(); w != nil {
			now := time.Now()
			w.in.Push(input.KeyPress{Key: k, Time: now})
			w.in.Push(input.KeyRelease{Key: k, Time: now})
		}
	})
}

// mediaSeek takes a move along the media controls' bar to at to the
// window they speak to. It may be called from any goroutine.
func (d *Driver) mediaSeek(at time.Duration) {
	d.post(func() {
		if w := d.mediaWindow(); w != nil {
			w.in.Push(input.MediaSeek{At: max(at, 0), Time: time.Now()})
		}
	})
}
