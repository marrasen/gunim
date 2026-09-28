package desktop

import (
	"fmt"
	"image/color"
	"math"
	"os"
)

// drawnShadowMode is GUNIM_DRAWN_SHADOW: "0" leaves a chromeless window's shadow to the system, and "measure" draws
// green bands instead of the drawn shadow, for measuring how closely it follows.
var drawnShadowMode = os.Getenv("GUNIM_DRAWN_SHADOW")

// startShadow gives a chromeless window a drawn shadow where it presents through DXGI, unless GUNIM_DRAWN_SHADOW=0.
// It runs on the main thread.
func startShadow(w *Window) {
	// Only DXGI's frames carry the alpha the cut corners need
	if drawnShadowMode == "0" || !w.d.dxgi {
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

// applyBorder gives a chromeless window the border the application asked for, drawn with its shadow or by the
// system. It runs on the main thread.
func applyBorder(w *Window) error {
	if !w.Chromeless() {
		return nil
	}
	w.mu.Lock()
	b, drawn := w.border, w.shadow
	w.mu.Unlock()
	set := w.gw.SetBorderColors
	if drawn {
		set = w.gw.SetShadowBorder
	}
	inactive := b.Inactive
	if inactive == (color.NRGBA{}) {
		inactive = b.Color
	}
	if err := set(b.Color, inactive, b.None); err != nil {
		w.debugf("border: %v", err)
		return fmt.Errorf("desktop: set the window's border: %w", err)
	}
	return nil
}

// fadeShadow shows the drawn shadow at opacity o, or hides it at 0. It runs on the main thread.
func fadeShadow(w *Window, o float32) {
	if err := w.gw.SetShadowOpacity(o); err != nil {
		w.debugf("shadow: %v", err)
	}
}

// cornerRadius is the radius, in device pixels, a window with a drawn shadow cuts its corners to, as Windows 11
// rounds its own, and the edge it leaves for the shadow's border, or 0 and 0 while it is maximized or fills its
// monitor, and no edge where the application asked for no border. Both follow the monitor's scale alone, not the
// window's zoom, as the system's do.
func (w *Window) cornerRadius() (radius, edge float32) {
	if w.Maximized() || w.FullScreen() {
		return 0, 0
	}
	w.mu.Lock()
	on, k, none := w.shadow, w.content, w.border.None
	w.mu.Unlock()
	if !on {
		return 0, 0
	}
	if none {
		return 8 * k, 0
	}
	return 8 * k, float32(math.Ceil(float64(k)))
}
