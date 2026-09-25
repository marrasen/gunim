//go:build windows || darwin

package desktop

// Screen readers reach windows on Windows through UI Automation and on
// macOS through NSAccessibility, which the driver has yet to speak. So
// these are empty, and the engine gathers no tree.

type driverAccess struct{}

type windowAccess struct{}

func (w *Window) accessOpen()         {}
func (w *Window) accessClose()        {}
func (w *Window) accessFocus(in bool) {}
