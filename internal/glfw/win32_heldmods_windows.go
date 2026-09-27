// SPDX-License-Identifier: Apache-2.0

package glfw

var procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")

// platformHeldModifiers reads the keys' state now, which GetKeyState
// does not while another program has the keyboard.
func (w *Window) platformHeldModifiers() ModifierKey {
	held := func(vk int32) bool {
		r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
		return uint16(r)&0x8000 != 0
	}
	var mods ModifierKey
	if held(_VK_SHIFT) {
		mods |= ModShift
	}
	if held(_VK_CONTROL) {
		mods |= ModControl
	}
	if held(_VK_MENU) {
		mods |= ModAlt
	}
	if held(_VK_LWIN) || held(_VK_RWIN) {
		mods |= ModSuper
	}
	return mods
}
