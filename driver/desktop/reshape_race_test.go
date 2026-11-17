//go:build linux || windows || darwin

package desktop

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// reshapeFor is how long TestAWindowResizedWhileItDrawsKeepsDrawing resizes its window.
var reshapeFor = 3 * time.Second

// A window resized over and over while it draws a new frame every refresh keeps drawing. On macOS the main thread
// updates the window's OpenGL context as it resizes, and the render thread presents frames with that context; done
// at once, the system stopped the process with SIGTRAP.
func TestAWindowResizedWhileItDrawsKeepsDrawing(t *testing.T) {
	if display == nil {
		t.Skip("no display")
	}
	w, err := display.NewWindow(driver.Options{Title: "gunim reshape test", Size: geom.Sz(320, 200)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	rs, ok := w.(driver.Reshaper)
	if !ok {
		t.Skip("the window does not reshape")
	}
	stop := make(chan struct{})
	drawn := make(chan int)
	go func() {
		n := 0
		defer func() { drawn <- n }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// A new colour each frame, so the window never keeps the frame it shows.
			c := color.NRGBA{R: uint8(n), G: uint8(n * 3), B: 0x80, A: 0xff}
			ops := []paint.Op{&paint.RRectOp{Rect: geom.Rect{Max: w.Size().Point()}, Fill: paint.Solid(c),
				Transform: paint.Identity}}
			if err := w.Present(ops, []geom.Rect{paint.Everything}); err != nil {
				t.Error(err)
				return
			}
			select {
			case <-w.Presented():
			case <-time.After(5 * time.Second):
				t.Error("no frame presented")
				return
			}
			n++
		}
	}()
	deadline := time.Now().Add(reshapeFor)
	i := 0
	for ; time.Now().Before(deadline); i++ {
		d := float32(40)
		if i%2 == 1 {
			d = -d
		}
		if err := rs.Reshape(geom.Insets{Left: d, Bottom: d}); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	n := <-drawn
	t.Logf("%d frames drawn over %d resizes", n, i)
	if n < 10 {
		t.Errorf("the window drew %d frames while it resized, want more", n)
	}
}
