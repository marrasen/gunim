//go:build darwin || (windows && !amd64 && !arm64)

package desktop

// Screen readers reach windows on macOS through NSAccessibility, which
// the driver has yet to speak, and 32-bit Windows is left out of UI
// Automation. So these are empty, and the engine gathers no tree.

type driverAccess struct{}

type windowAccess struct{}

func (w *Window) accessOpen()         {}
func (w *Window) accessClose()        {}
func (w *Window) accessFocus(in bool) {}
