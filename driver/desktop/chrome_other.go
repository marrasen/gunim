//go:build linux || darwin

package desktop

// nativeFrame says the system moves and sizes a chromeless window from
// its hit test, as Windows does; here the engine starts moves and
// resizes itself.
const nativeFrame = false
