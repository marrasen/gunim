//go:build linux || darwin

package desktop

// nativeFrame says the system moves and sizes a chromeless window from
// its hit test, as Windows does; here the engine starts moves and
// resizes itself.
const nativeFrame = false

// watchMoveSize does nothing where a move or a resize starts with a press the engine sees, which closes popups as
// any press outside them does.
func watchMoveSize(*Window) {}
