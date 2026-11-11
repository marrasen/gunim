//go:build windows

package desktop

import (
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/internal/glfw"
)

var procSendMessageW = user32.NewProc("SendMessageW")

func TestTheSystemLookReads(t *testing.T) {
	l, err := readLook()
	if err != nil {
		t.Fatal(err)
	}
	if l.TextScale < 1 || l.TextScale > 2.25 {
		t.Errorf("text scale = %v, want 1 to 2.25", l.TextScale)
	}
	if l.Contrast != nil && l.Contrast.Window == l.Contrast.WindowText {
		t.Errorf("a contrast theme has text the colour of its window, %v", l.Contrast.Window)
	}
}

func TestAChangedSettingReachesTheWatch(t *testing.T) {
	if pump == nil {
		t.Skip("no display")
	}
	var mu sync.Mutex
	now := driver.SystemLook{TextScale: 1}
	lookNow = func() (driver.SystemLook, error) {
		mu.Lock()
		defer mu.Unlock()
		return now, nil
	}
	defer func() { lookNow = readLook }()

	heard := make(chan driver.SystemLook, 4)
	stop, err := pump.WatchSystemLook(func(l driver.SystemLook, err error) {
		if err != nil {
			t.Error(err)
		}
		heard <- l
	})
	if err != nil {
		t.Fatal(err)
	}
	announce := func() {
		done := make(chan struct{})
		pump.post(func() {
			_, _, _ = procSendMessageW.Call(glfw.SettingsWindow(), 0x001a, 0, 0) // WM_SETTINGCHANGE
			close(done)
		})
		<-done
	}

	mu.Lock()
	now = driver.SystemLook{Dark: true, TextScale: 1.25,
		Contrast: &driver.ContrastColors{Window: color.NRGBA{A: 0xff}}}
	mu.Unlock()
	announce()
	select {
	case l := <-heard:
		if !l.Dark || l.TextScale != 1.25 || l.Contrast == nil {
			t.Errorf("the watch heard %+v, want the dark, larger, contrast look set", l)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a changed setting never reached the watch")
	}

	// The same look again says nothing.
	announce()
	select {
	case l := <-heard:
		t.Errorf("the watch heard %+v, though nothing changed", l)
	case <-time.After(200 * time.Millisecond):
	}

	stop()
	stop()
	gone := make(chan uintptr, 1)
	pump.post(func() { gone <- glfw.SettingsWindow() })
	if h := <-gone; h != 0 {
		t.Errorf("the settings window is still up after stop")
	}
}
