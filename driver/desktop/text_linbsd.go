//go:build linux

package desktop

import (
	"time"

	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// installText connects text input through X11's input method: typed and
// committed text through the text input callback, and compositions
// through the preedit callback. The character callback stays unset, as
// it would report typed text a second time.
func installText(w *Window) {
	gw := w.gw
	_, _ = gw.SetTextInputCallback(func(_ *glfw.Window, s string) {
		w.in.push(input.TextInput{Text: s, Time: time.Now()})
	})
	_, _ = gw.SetPreeditCallback(func(_ *glfw.Window, s string, start, end int) {
		w.in.push(input.Composing{Text: s, Selected: [2]int{start, end}})
	})
	_, _ = gw.SetTextInputActiveCallback(func(*glfw.Window) bool { return w.textInput.Load() })
}

// resetInputMethod ends a composition in progress. It runs on the main
// thread.
func resetInputMethod(w *Window) {
	if !w.closed {
		_ = w.gw.ResetInputContext()
	}
}
