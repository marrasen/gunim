//go:build linux || darwin

package desktop

// startShadow does nothing where gunim draws no shadow of its own.
func startShadow(*Window) {}

// fadeShadow does nothing where gunim draws no shadow of its own.
func fadeShadow(*Window, float32) {}
