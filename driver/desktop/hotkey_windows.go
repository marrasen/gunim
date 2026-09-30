//go:build windows

package desktop

import (
	"errors"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// RegisterHotKey implements [driver.HotKeyer], on the main thread.
func (d *Driver) RegisterHotKey(k driver.HotKey, fn func()) (func(), error) {
	vk, ok := virtualKey(k.Key)
	if !ok {
		return func() {}, errors.New("that key cannot be taken from every program")
	}
	var mods uint32
	for _, m := range []struct {
		in  input.Mods
		out uint32
	}{{input.ModAlt, glfw.HotKeyAlt}, {input.ModControl, glfw.HotKeyControl}, {input.ModShift, glfw.HotKeyShift}, {input.ModSuper, glfw.HotKeyWin}} {
		if k.Mods.Has(m.in) {
			mods |= m.out
		}
	}
	type result struct {
		id  uintptr
		err error
	}
	done := make(chan result, 1)
	if !d.post(func() {
		id, err := glfw.RegisterHotKey(mods, vk, func() { go fn() })
		done <- result{id, err}
	}) {
		return func() {}, driver.ErrNoHotKeys
	}
	r := <-done
	if errors.Is(r.err, glfw.ErrHotKeyTaken) {
		return func() {}, driver.ErrHotKeyTaken
	}
	if r.err != nil {
		return func() {}, r.err
	}
	return func() { d.post(func() { glfw.UnregisterHotKey(r.id) }) }, nil
}

// virtualKey is the Windows virtual key of k, where it has one a hot
// key can take.
func virtualKey(k input.Key) (uint32, bool) {
	switch {
	case k >= input.KeyA && k <= input.KeyZ:
		return uint32('A' + (k - input.KeyA)), true
	case k >= input.Key0 && k <= input.Key9:
		return uint32('0' + (k - input.Key0)), true
	case k >= input.KeyF1 && k <= input.KeyF12:
		return uint32(0x70 + (k - input.KeyF1)), true
	}
	if vk, ok := otherVirtualKeys[k]; ok {
		return vk, true
	}
	return 0, false
}

// otherVirtualKeys are the virtual keys of the other keys a hot key can
// take.
var otherVirtualKeys = map[input.Key]uint32{
	input.KeySpace: 0x20, input.KeyEnter: 0x0d, input.KeyEscape: 0x1b, input.KeyTab: 0x09,
}
