//go:build android

package android

import (
	"math"
	"time"

	"github.com/marrasen/gunim/driver/internal/compass"
	"github.com/marrasen/gunim/input"
)

// HasCompass implements [driver.Compass]: the phone has a rotation vector sensor, or an accelerometer and a
// magnetometer.
func (w *Window) HasCompass() bool { return w.d.hasCompass() }

// WatchHeading implements [driver.Compass]. The sensors run, at the rate Android keeps for a user interface, while
// any window watches, and each reading goes to every window that does.
func (w *Window) WatchHeading(on bool) { w.d.watchHeading(w, on) }

// hasCompass reports whether the phone has the sensors, asking Java once.
func (d *Driver) hasCompass() bool {
	d.compassOnce.Do(func() { d.compass = hasCompass() })
	return d.compass
}

// watchHeading notes whether w watches the heading, and starts or stops the sensors to suit.
func (d *Driver) watchHeading(w *Window, on bool) {
	if !d.hasCompass() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if on && !w.closed {
		d.headingTo[w] = true
	} else {
		delete(d.headingTo, w)
	}
	d.runCompassLocked()
}

// runCompassLocked starts the sensors while a window watches the heading, and stops them once none does. It runs
// with mu held; Java only queues the change for its UI thread, and calls nothing back.
func (d *Driver) runCompassLocked() {
	if want := len(d.headingTo) > 0; want != d.compassOn {
		d.compassOn = want
		watchHeading(want)
	}
}

// heading hands a reading of the sensors to every window that watches the heading: the phone's rotation matrix r,
// with the screen turned by rot, the sensor's accuracy as Android's SENSOR_STATUS values number it, and how far off
// it reckons the heading may be, in radians, or a negative number where it does not say.
func (d *Driver) heading(r [9]float32, rot compass.Rotation, accuracy int, errRad float32) {
	deg, ok := compass.Heading(r, rot)
	if !ok {
		return
	}
	e := input.Heading{
		Degrees:  deg,
		Accuracy: input.HeadingAccuracy(min(max(accuracy, int(input.HeadingUnreliable)), int(input.HeadingHigh))),
		Time:     time.Now(),
	}
	if errRad > 0 {
		e.Error = errRad * 180 / math.Pi
	}
	d.mu.Lock()
	ws := make([]*Window, 0, len(d.headingTo))
	for w := range d.headingTo {
		ws = append(ws, w)
	}
	d.mu.Unlock()
	for _, w := range ws {
		w.in.Push(e)
	}
}
