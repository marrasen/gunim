//go:build linux || darwin

package desktop

// startShadow does nothing where gunim draws no shadow of its own.
func startShadow(*Window) {}

// fadeShadow does nothing where gunim draws no shadow of its own.
func fadeShadow(*Window, float32) {}

// cornerRadius is 0 where gunim draws no shadow of its own.
func (w *Window) cornerRadius() (radius, edge float32) { return 0, 0 }
