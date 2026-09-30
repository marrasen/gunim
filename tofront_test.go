package gunim

import (
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// A window is brought to the front from any goroutine through its
// client, and from its own through the UI.
func TestToFront(t *testing.T) {
	w := NewOffscreen(geom.Sz(100, 100), nil)
	defer w.Close()
	var _ driver.Fronter = w.Offscreen()
	w.Client().ToFront()
	w.ui.ToFront()
	if got := w.Offscreen().Raised(); got != 2 {
		t.Fatalf("the window was brought to the front %d times, want 2", got)
	}
}
