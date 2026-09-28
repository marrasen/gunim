package desktop

import (
	"math"
	"os"
)

// drawnShadowMode is GUNIM_DRAWN_SHADOW: "1" draws a chromeless window's shadow in a window of gunim's own instead
// of the system's, and "measure" draws green bands there, for measuring how closely it follows.
var drawnShadowMode = os.Getenv("GUNIM_DRAWN_SHADOW")

// startShadow gives a chromeless window a drawn shadow where GUNIM_DRAWN_SHADOW asks for one and the window presents
// through DXGI. It runs on the main thread.
func startShadow(w *Window) {
	// Only DXGI's frames carry the alpha the cut corners need
	if drawnShadowMode != "1" && drawnShadowMode != "measure" || !w.d.dxgi {
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

// cornerRadius is the radius, in device pixels, a window with a drawn shadow cuts its corners to, as Windows 11
// rounds its own, and the edge it leaves for the shadow's border, or 0 and 0 while it is maximized or fills its
// monitor. Both follow the monitor's scale alone, not the window's zoom, as the system's do.
func (w *Window) cornerRadius() (radius, edge float32) {
	if w.Maximized() || w.FullScreen() {
		return 0, 0
	}
	w.mu.Lock()
	on, k := w.shadow, w.content
	w.mu.Unlock()
	if !on {
		return 0, 0
	}
	return 8 * k, float32(math.Ceil(float64(k)))
}
