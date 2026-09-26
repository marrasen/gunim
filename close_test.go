package gunim

import (
	"errors"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
)

// quitAsked is what the application hears when the user asks to close
// its window.
type quitAsked struct{}

func TestAWindowThatAsksToCloseStaysOpenUntilTheApplicationCloses(t *testing.T) {
	w := newTestWindow()
	w.askToClose = quitAsked{}
	d := w.mustOffscreen(t)
	go w.loop()
	d.Post(driver.CloseAsked{})
	select {
	case env := <-w.Client().Intents():
		if env.Intent != (quitAsked{}) {
			t.Fatalf("asked to close, the application heard %#v", env.Intent)
		}
	case <-time.After(time.Second):
		t.Fatal("asked to close, the application heard nothing")
	}
	// Still open: it takes input, and its intents stay open.
	d.Post(driver.WindowFocus{Focused: true})
	w.Client().Close()
	for range w.Client().Intents() {
	}
}

func TestAWindowCloses(t *testing.T) {
	w := newTestWindow()
	d := w.mustOffscreen(t)
	go w.loop()
	d.Post(driver.CloseAsked{})
	select {
	case _, open := <-w.Client().Intents():
		if open {
			t.Fatal("asked to close, the window sent an intent")
		}
	case <-time.After(time.Second):
		t.Fatal("asked to close, the window stayed open")
	}
}

func TestAnOffscreenWindowHasNoPixelsToShoot(t *testing.T) {
	w := newTestWindow()
	if _, err := w.Client().Shot(t.Context()); !errors.Is(err, ErrNoPixels) {
		t.Fatalf("shot offscreen: %v", err)
	}
}
