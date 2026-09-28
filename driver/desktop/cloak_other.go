//go:build linux || darwin

package desktop

// cloak does nothing where a window's surface starts out opaque.
func cloak(*Window, bool) {}
