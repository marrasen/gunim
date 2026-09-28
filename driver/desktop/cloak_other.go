//go:build linux || darwin

package desktop

// cloak does nothing where a window's surface starts out opaque.
func cloak(*Window, bool) {}

// showFrame does nothing where the system draws no shadow round a
// chromeless window.
func showFrame(*Window, bool) {}
