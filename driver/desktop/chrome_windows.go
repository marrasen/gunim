package desktop

import (
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// nativeFrame says the system moves and sizes a chromeless window from
// its hit test, as Windows does.
const nativeFrame = true

// watchMoveSize reports the user starting to move or size the window, which Windows takes the press for. It runs on
// the main thread.
func watchMoveSize(w *Window) {
	w.gw.SetMoveSizeCallback(func(*glfw.Window) { w.in.Push(driver.MoveStarted{}) })
	w.gw.SetHistoryCallback(func(_ *glfw.Window, forward bool) {
		w.in.Push(input.HistoryStep{Forward: forward, Time: time.Now()})
	})
}
