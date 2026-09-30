// SPDX-License-Identifier: Apache-2.0

// gunim change: this file is gunim's. It reports the Browser Back and Forward commands Windows sends, from a
// keyboard's keys or a mouse whose driver sends commands in place of its side buttons.

package glfw

const (
	_WM_APPCOMMAND               = 0x0319
	_APPCOMMAND_BROWSER_BACKWARD = 1
	_APPCOMMAND_BROWSER_FORWARD  = 2
	_FAPPCOMMAND_MASK            = 0xF000
	_APPCOMMAND_HANDLED          = 1
)

// SetHistoryCallback sets the function told when the user asks to go back, or forward with forward set, with a
// Browser Back or Forward key. It is a gunim addition, on Windows alone; a mouse's side buttons arrive as buttons 4
// and 5.
func (w *Window) SetHistoryCallback(f func(w *Window, forward bool)) { w.platform.history = f }

// historyMessage tells the history callback of a Browser Back or Forward command, and reports whether it was one.
func (w *Window) historyMessage(uMsg uint32, lParam _LPARAM) bool {
	if uMsg != _WM_APPCOMMAND || w.platform.history == nil {
		return false
	}
	switch uint32(lParam>>16) &^ _FAPPCOMMAND_MASK {
	case _APPCOMMAND_BROWSER_BACKWARD:
		w.platform.history(w, false)
	case _APPCOMMAND_BROWSER_FORWARD:
		w.platform.history(w, true)
	default:
		return false
	}
	return true
}
