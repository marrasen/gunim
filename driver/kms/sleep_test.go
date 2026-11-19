//go:build linux && (arm64 || amd64)

package kms

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// Sleep fades a lit screen to black over 600 ms and turns it off, as an
// untouched one does after Off, and a touch wakes it. It does nothing while
// a window keeps the screen awake, or a finger is down.
func TestSleepTurnsTheScreenOffNow(t *testing.T) {
	b := newBlanker(Blank{Dim: time.Minute, Off: 5 * time.Minute}, 1, bt0)
	b.sleep(after(time.Second))
	if b.state != screenFading {
		t.Fatalf("after Sleep: %v, want fading", b.state)
	}
	if !b.step(after(time.Second+offFade)) || b.state != screenOff || b.bright(after(time.Second+offFade)) != 0 {
		t.Fatalf("after the fade: %v", b.state)
	}
	if !b.touched(after(time.Minute)) || b.state != screenOn {
		t.Fatalf("a touch on the dark screen: %v", b.state)
	}

	b.setKeep(true, after(2*time.Minute))
	b.sleep(after(3 * time.Minute))
	if b.state != screenOn {
		t.Fatalf("Sleep while kept awake: %v", b.state)
	}
	b.setKeep(false, after(3*time.Minute))
	b.fingers = true
	b.sleep(after(4 * time.Minute))
	if b.state != screenOn {
		t.Fatalf("Sleep with a finger down: %v", b.state)
	}
}

// Untouched counts from the last touch: a touch sets it back to about
// nothing.
func TestUntouchedCountsFromTheLastTouch(t *testing.T) {
	d := testDriver(0, 1)
	d.blank = newBlanker(Blank{}, 1, time.Now().Add(-time.Hour))
	if u := d.Untouched(); u < 59*time.Minute {
		t.Fatalf("untouched for %v, want about an hour", u)
	}
	w, _ := d.NewWindow(driver.Options{})
	win := w.(*Window)
	defer func() { _ = win.Close() }()
	m := touchMap{x: absInfo{maximum: 719}, y: absInfo{maximum: 1279}}
	d.touchReport(m, []contact{{slot: 0, id: 1, x: 10, y: 10, phase: phaseDown}}, time.Now())
	if u := d.Untouched(); u > time.Minute {
		t.Fatalf("untouched for %v after a touch", u)
	}
	if _, ok := nextInput(t, win).(input.PointerMove); !ok {
		t.Fatal("the touch on a lit screen did not reach the window")
	}
}
