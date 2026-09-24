package desktop

import (
	"math"
	"time"

	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// installText connects text input through the Windows input method:
// typed and committed text through the text input callback, and
// compositions through the preedit callback. The character callback
// stays unset, as it would report typed text a second time.
//
// The input method stays off until a text field takes focus, so that
// key presses elsewhere are not composed.
func installText(w *Window) {
	gw := w.gw
	_, _ = gw.SetTextInputCallback(func(_ *glfw.Window, s string) {
		w.in.push(input.TextInput{Text: s, Time: time.Now()})
	})
	_, _ = gw.SetPreeditCallback(func(_ *glfw.Window, s string, start, end int) {
		w.in.push(input.Composing{Text: s, Selected: [2]int{start, end}})
	})
	_, _ = gw.SetTextInputActiveCallback(func(*glfw.Window) bool { return w.textInput.Load() })
	_ = gw.SetInputMethodEnabled(false)
}

// textInputChanged turns the input method on while the application
// takes text, and off otherwise, ending any composition in progress. It
// runs on the main thread.
func textInputChanged(w *Window, active bool) {
	if w.closed {
		return
	}
	if !active {
		_ = w.gw.ResetInputContext()
	}
	_ = w.gw.SetInputMethodEnabled(active)
	if active {
		placeInputMethod(w)
	}
}

// caretMoved moves the input method's windows to the new caret.
func caretMoved(w *Window) {
	w.d.post(func() { placeInputMethod(w) })
}

// placeInputMethod tells the input method where the caret is, in
// client-area pixels. It runs on the main thread.
func placeInputMethod(w *Window) {
	if w.closed {
		return
	}
	w.mu.Lock()
	r, f := w.caret, w.scale/w.perCoord
	w.mu.Unlock()
	x0, y0 := math.Floor(float64(r.Min.X*f)), math.Floor(float64(r.Min.Y*f))
	x1, y1 := math.Ceil(float64(r.Max.X*f)), math.Ceil(float64(r.Max.Y*f))
	_ = w.gw.SetInputMethodCaret(int(x0), int(y0), int(x1-x0), int(y1-y0))
}
