//go:build linux || darwin

package desktop

import "github.com/marrasen/gunim/internal/glfw"

// swapInterval is 1: the swap itself waits for the display.
const swapInterval = 1

// vblank is a stand-in here, where the swap does the waiting; wait
// reports false so that pace watches the swap.
type vblank struct{}

func newVBlank(*glfw.Window) *vblank { return &vblank{} }

func (*vblank) wait() bool { return false }

func (*vblank) close() {}

// bufferSize reports false: the buffer here changes size only as the
// main thread measures it.
func (*vblank) bufferSize() (w, h int, ok bool) { return 0, 0, false }
