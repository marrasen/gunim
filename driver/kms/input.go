//go:build linux

package kms

import (
	"log"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The double tap's reach, as on Android.
const (
	doubleTapTime  = 300 * time.Millisecond
	doubleTapSpace = 24
)

// touchState is the touch in progress, guarded by the driver's mu.
type touchState struct {
	// window is the window the first finger came down in, which keeps
	// the touch until every finger lifts. pointer is the slot of the
	// finger that drives the pointer, or -1.
	window  *Window
	pointer int
	// fingers is the window each finger of its own came down in, by slot.
	fingers map[int]*Window
	lastTap geom.Point
	lastAt  time.Time
	clicks  int
}

// touchMap maps a touch screen's positions onto the panel.
type touchMap struct {
	x, y absInfo
}

// panel returns where a touch at x, y lies on a panel of w×h pixels.
func (m touchMap) panel(x, y int32, w, h int) geom.Point {
	span := func(v int32, a absInfo, n int) float32 {
		r := a.maximum - a.minimum + 1
		if r <= 0 {
			return float32(v)
		}
		return (float32(v-a.minimum) + 0.5) * float32(n) / float32(r)
	}
	return geom.Pt(span(x, m.x, w), span(y, m.y, h))
}

// at returns where a touch at x, y lies on the picture, in logical pixels.
func (d *Driver) at(m touchMap, x, y int32) geom.Point {
	p := m.panel(x, y, d.panelW, d.panelH)
	p = d.turn.toPicture(p, float32(d.panelW), float32(d.panelH))
	return geom.Pt(p.X/d.density, p.Y/d.density)
}

// touchReport takes one report of the touch screen's fingers, and sends
// the windows what they did. The first finger drives the pointer: it
// presses, moves and lets go as a mouse's primary button does, marked as
// touch, and leaves the window as it lifts, since a finger has no hover.
// In a window that takes fingers each finger is an [input.Finger] too,
// with the first one's FingerDown ahead of its press.
func (d *Driver) touchReport(m touchMap, cs []contact, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.touchAt.IsZero() {
		d.touchAt = now
	}
	t := &d.input
	if t.fingers == nil {
		t.fingers = map[int]*Window{}
		t.pointer = -1
	}
	for _, c := range cs {
		at := d.at(m, c.x, c.y)
		switch c.phase {
		case phaseDown:
			if t.window == nil {
				t.window = d.hitLocked(at)
			}
			w := t.window
			if w == nil {
				continue
			}
			pos := at.Sub(w.rectLocked().Min)
			if w.takesFingers {
				t.fingers[c.slot] = w
				w.in.Push(input.Finger{ID: c.slot, Pos: pos, Phase: input.FingerDown, Time: now})
			}
			if t.pointer < 0 {
				t.pointer = c.slot
				if now.Sub(t.lastAt) < doubleTapTime && abs(at.X-t.lastTap.X) < doubleTapSpace && abs(at.Y-t.lastTap.Y) < doubleTapSpace {
					t.clicks++
				} else {
					t.clicks = 1
				}
				t.lastTap, t.lastAt = at, now
				w.in.Push(input.PointerMove{Pos: pos, Touch: true, Time: now})
				w.in.Push(input.PointerDown{Pos: pos, Button: input.ButtonPrimary, Clicks: t.clicks, Touch: true, Time: now})
			}
		case phaseMove:
			if w := t.fingers[c.slot]; w != nil {
				w.in.Push(input.Finger{ID: c.slot, Pos: at.Sub(w.rectLocked().Min), Phase: input.FingerMove, Time: now})
			}
			if c.slot == t.pointer && t.window != nil {
				t.window.in.Push(input.PointerMove{Pos: at.Sub(t.window.rectLocked().Min), Touch: true, Time: now})
			}
		case phaseUp:
			if w := t.fingers[c.slot]; w != nil {
				w.in.Push(input.Finger{ID: c.slot, Pos: at.Sub(w.rectLocked().Min), Phase: input.FingerUp, Time: now})
				delete(t.fingers, c.slot)
			}
			if c.slot == t.pointer {
				t.pointer = -1
				if w := t.window; w != nil {
					w.in.Push(input.PointerUp{Pos: at.Sub(w.rectLocked().Min), Button: input.ButtonPrimary, Touch: true, Time: now})
					w.in.Push(input.PointerLeave{Time: now})
				}
			}
		}
	}
	if t.pointer < 0 && len(t.fingers) == 0 {
		t.window = nil
	}
}

// hitLocked returns the topmost window shown at p, in logical pixels of the
// picture, that takes the pointer. It runs with mu held.
func (d *Driver) hitLocked(p geom.Point) *Window {
	for i := len(d.windows) - 1; i >= 0; i-- {
		w := d.windows[i]
		if w.hidden || w.passthrough {
			continue
		}
		if w.rectLocked().Contains(p) {
			return w
		}
	}
	return nil
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// startInput starts a reader for the touch screen, or the recording to play,
// and for each keyboard.
func (d *Driver) startInput() {
	run := func(fn func()) {
		d.readers.Add(1)
		go func() {
			defer d.readers.Done()
			fn()
		}()
	}
	switch {
	case d.o.Replay != nil:
		run(d.replay)
	case d.touch != nil:
		run(func() { d.readTouch(d.touch) })
	}
	for _, k := range d.keys {
		run(func() { d.readKeys(k) })
	}
}

// readTouch reads the touch screen until the driver quits. It takes the
// device for itself, so nothing else on the system hears its touches, and
// has its events stamped on the monotonic clock, as flips are.
func (d *Driver) readTouch(dev *inputDev) {
	if err := ioctlInt(dev.fd, eviocgrab, 1); err != nil {
		log.Printf("gunim: kms: %s: could not take the touch screen for itself: %v", dev.path, err)
	}
	clock := int32(unix.CLOCK_MONOTONIC)
	if err := ioctlIntPtr(dev.fd, eviocsclockid, &clock); err != nil {
		log.Printf("gunim: kms: %s: touch times are not on the monotonic clock, so touch to flip is off: %v", dev.path, err)
	}
	m := touchMap{x: dev.x, y: dev.y}
	dec := newMTDecoder(dev.slots)
	dec.resync = dev.mtSlots
	d.readEvents(dev, func(e event) {
		if d.rec != nil {
			d.rec.write(e)
		}
		if cs := dec.feed(e); cs != nil {
			d.touchReport(m, cs, monoTime(e.time()))
		}
	})
}

// readKeys reads a keyboard until the driver quits, for Ctrl+Alt+Fn.
func (d *Driver) readKeys(dev *inputDev) {
	var ctrl, alt bool
	d.readEvents(dev, func(e event) {
		if e.typ != evKey {
			return
		}
		down := e.value != 0
		switch e.code {
		case keyLeftCtrl, keyRightCtrl:
			ctrl = down
		case keyLeftAlt, keyRightAlt:
			alt = down
		default:
			if n := vtKey(e.code); n > 0 && e.value == 1 && ctrl && alt && d.o.VTSwitch && d.vt != nil && n != d.vt.num {
				if err := d.vt.activate(n); err != nil {
					log.Printf("gunim: kms: switch to VT %d: %v", n, err)
				}
			}
		}
	})
}

// readEvents calls fn for each event dev reads, until the driver quits.
func (d *Driver) readEvents(dev *inputDev, fn func(event)) {
	buf := make([]byte, 64*liveEventSize)
	fds := []unix.PollFd{{Fd: int32(dev.fd), Events: unix.POLLIN}}
	for {
		select {
		case <-d.quit:
			return
		default:
		}
		fds[0].Revents = 0
		n, err := unix.Poll(fds, 250)
		if err == unix.EINTR || n == 0 {
			continue
		}
		if err != nil || fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			select {
			case <-d.quit:
				// Closed as the driver quit.
			default:
				log.Printf("gunim: kms: %s: gone: %v", dev.path, err)
			}
			return
		}
		n, err = unix.Read(dev.fd, buf)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if err != nil {
			log.Printf("gunim: kms: %s: %v", dev.path, err)
			return
		}
		for o := 0; o+liveEventSize <= n; o += liveEventSize {
			fn(decodeEvent(buf[o:], liveEventSize))
		}
	}
}

// replay plays a recording through the same path the touch screen's events
// take, at the pace it was made.
func (d *Driver) replay() {
	p, err := readRecording(d.o.Replay)
	if err != nil {
		log.Printf("gunim: kms: %v", err)
		return
	}
	m := touchMap{x: p.x, y: p.y}
	dec := newMTDecoder(p.slots)
	if len(p.events) == 0 {
		return
	}
	start, first := time.Now(), p.events[0].time()
	for _, e := range p.events {
		if wait := time.Until(start.Add(e.time() - first)); wait > 0 {
			select {
			case <-d.quit:
				return
			case <-time.After(wait):
			}
		}
		if cs := dec.feed(e); cs != nil {
			d.touchReport(m, cs, time.Now())
		}
	}
}

// ioctlIntPtr calls an ioctl whose argument points at an int.
func ioctlIntPtr(fd int, req uintptr, v *int32) error {
	return ioctl(fd, req, unsafe.Pointer(v))
}
