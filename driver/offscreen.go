package driver

import (
	"sync"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Offscreen returns a window backed by no display.
//
// It keeps the op list from the last frame instead of drawing it, which
// makes it the driver for tests, for golden-image comparisons once a
// rasteriser exists, and for running an interface headlessly on a build
// machine. Its Frames and Input channels stay quiet, so whoever holds
// it decides when a frame happens.
func Offscreen(size geom.Size) *OffscreenWindow {
	return &OffscreenWindow{
		size:   size,
		scale:  1,
		rate:   60,
		frames: make(chan Frame),
		input:  make(chan any),
	}
}

// An OffscreenWindow records frames rather than showing them.
type OffscreenWindow struct {
	mu    sync.Mutex
	size  geom.Size
	scale float32
	rate  float64
	ops   []paint.Op

	frames chan Frame
	input  chan any
}

// Frames implements [Window]. It stays quiet, because the holder of an
// offscreen window decides when a frame happens.
func (w *OffscreenWindow) Frames() <-chan Frame { return w.frames }

// Input implements [Window]. Feed it with [OffscreenWindow.Post].
func (w *OffscreenWindow) Input() <-chan any { return w.input }

// Tick delivers one display refresh, blocking until the window takes
// it.
//
// A driver with a screen behind it sends without blocking and lets a
// tick go when nobody is waiting. This one waits, so that a test knows
// the frame it asked for has been picked up.
func (w *OffscreenWindow) Tick() { w.frames <- Frame{} }

// Post delivers a platform event, blocking until the window reads it.
func (w *OffscreenWindow) Post(ev any) { w.input <- ev }

// Present implements [Window] by keeping the op list.
func (w *OffscreenWindow) Present(ops []paint.Op, _ geom.Rect) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ops = ops
	return nil
}

// Ops returns the commands recorded by the last frame.
func (w *OffscreenWindow) Ops() []paint.Op {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ops
}

// Resize changes the window's size for the next frame.
func (w *OffscreenWindow) Resize(size geom.Size) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.size = size
}

// Size implements [Window].
func (w *OffscreenWindow) Size() geom.Size {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// Scale implements [Window].
func (w *OffscreenWindow) Scale() float32 { return w.scale }

// RefreshRate implements [Window].
func (w *OffscreenWindow) RefreshRate() float64 { return w.rate }

// Close implements [Window].
func (w *OffscreenWindow) Close() error { return nil }
