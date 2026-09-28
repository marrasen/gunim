package desktop

import "os"

// drawnShadowMode is GUNIM_DRAWN_SHADOW: "1" draws a chromeless window's shadow in a window of gunim's own instead
// of the system's, and "measure" draws green bands there, for measuring how closely it follows.
var drawnShadowMode = os.Getenv("GUNIM_DRAWN_SHADOW")

// startShadow gives a chromeless window a drawn shadow where GUNIM_DRAWN_SHADOW asks for one. It runs on the main
// thread.
func startShadow(w *Window) {
	if drawnShadowMode != "1" && drawnShadowMode != "measure" {
		return
	}
	if err := w.gw.SetDrawnShadow(drawnShadowMode == "measure"); err != nil {
		w.debugf("shadow left to the system: %v", err)
		return
	}
	w.mu.Lock()
	w.shadow = true
	w.mu.Unlock()
}

// fadeShadow shows the drawn shadow at opacity o, or hides it at 0. It runs on the main thread.
func fadeShadow(w *Window, o float32) {
	if err := w.gw.SetShadowOpacity(o); err != nil {
		w.debugf("shadow: %v", err)
	}
}
