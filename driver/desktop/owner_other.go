//go:build linux || darwin

package desktop

// own does nothing where a popup stays above every window.
func own(*Window, *Window) error { return nil }
