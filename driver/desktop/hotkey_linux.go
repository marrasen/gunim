//go:build linux

package desktop

import (
	"errors"
	"os"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// RegisterHotKey implements [driver.HotKeyer]: on X11, a key grabbed on
// the root window. Under Wayland a grab through XWayland only sees keys
// while an X window has the keyboard, so there are none there.
func (d *Driver) RegisterHotKey(k driver.HotKey, fn func()) (func(), error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return func() {}, driver.ErrNoHotKeys
	}
	sym, ok := keysym(k.Key)
	if !ok {
		return func() {}, errors.New("that key cannot be taken from every program")
	}
	var mods uint32
	for _, m := range []struct {
		in  input.Mods
		out uint32
	}{{input.ModAlt, glfw.X11Alt}, {input.ModControl, glfw.X11Control}, {input.ModShift, glfw.X11Shift}, {input.ModSuper, glfw.X11Super}} {
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
		id, set, err := glfw.RegisterHotKeyX11(mods, sym)
		if err == nil {
			set(func() { go fn() })
		}
		done <- result{id, err}
	}) {
		return func() {}, driver.ErrNoHotKeys
	}
	r := <-done
	switch {
	case errors.Is(r.err, glfw.ErrX11HotKeyTaken):
		return func() {}, driver.ErrHotKeyTaken
	case r.err != nil:
		return func() {}, r.err
	}
	return func() { d.post(func() { glfw.UnregisterHotKeyX11(r.id) }) }, nil
}

// keysym is the X keysym of k, where it has one a hot key can take.
func keysym(k input.Key) (uint32, bool) {
	switch {
	case k >= input.KeyA && k <= input.KeyZ:
		return uint32('a' + (k - input.KeyA)), true
	case k >= input.Key0 && k <= input.Key9:
		return uint32('0' + (k - input.Key0)), true
	case k >= input.KeyF1 && k <= input.KeyF12:
		return uint32(0xffbe + (k - input.KeyF1)), true
	}
	if sym, ok := otherKeysyms[k]; ok {
		return sym, true
	}
	return 0, false
}

// otherKeysyms are the keysyms of the other keys a hot key can take.
var otherKeysyms = map[input.Key]uint32{
	input.KeySpace: 0x20, input.KeyEnter: 0xff0d, input.KeyEscape: 0xff1b, input.KeyTab: 0xff09,
}
