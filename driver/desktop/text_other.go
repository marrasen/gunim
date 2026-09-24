//go:build windows || darwin

package desktop

import (
	"time"

	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// installText connects text input through the character callback. The
// GLFW port reports no compositions on these platforms, so an input
// method there draws its composition in a window of its own and hands
// over the result.
func installText(w *Window) {
	_, _ = w.gw.SetCharCallback(func(_ *glfw.Window, r rune) {
		w.in.push(input.TextInput{Text: string(r), Time: time.Now()})
	})
}

// resetInputMethod has no composition to end here.
func resetInputMethod(*Window) {}
