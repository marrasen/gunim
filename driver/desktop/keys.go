//go:build linux || windows || darwin

package desktop

import (
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/glfw"
)

// keys maps GLFW's key codes to gunim's. GLFW names keys by their place
// on a US keyboard, which is what [input.Key] promises.
var keys = map[glfw.Key]input.Key{
	glfw.KeyA:            input.KeyA,
	glfw.KeyB:            input.KeyB,
	glfw.KeyC:            input.KeyC,
	glfw.KeyD:            input.KeyD,
	glfw.KeyE:            input.KeyE,
	glfw.KeyF:            input.KeyF,
	glfw.KeyG:            input.KeyG,
	glfw.KeyH:            input.KeyH,
	glfw.KeyI:            input.KeyI,
	glfw.KeyJ:            input.KeyJ,
	glfw.KeyK:            input.KeyK,
	glfw.KeyL:            input.KeyL,
	glfw.KeyM:            input.KeyM,
	glfw.KeyN:            input.KeyN,
	glfw.KeyO:            input.KeyO,
	glfw.KeyP:            input.KeyP,
	glfw.KeyQ:            input.KeyQ,
	glfw.KeyR:            input.KeyR,
	glfw.KeyS:            input.KeyS,
	glfw.KeyT:            input.KeyT,
	glfw.KeyU:            input.KeyU,
	glfw.KeyV:            input.KeyV,
	glfw.KeyW:            input.KeyW,
	glfw.KeyX:            input.KeyX,
	glfw.KeyY:            input.KeyY,
	glfw.KeyZ:            input.KeyZ,
	glfw.Key0:            input.Key0,
	glfw.Key1:            input.Key1,
	glfw.Key2:            input.Key2,
	glfw.Key3:            input.Key3,
	glfw.Key4:            input.Key4,
	glfw.Key5:            input.Key5,
	glfw.Key6:            input.Key6,
	glfw.Key7:            input.Key7,
	glfw.Key8:            input.Key8,
	glfw.Key9:            input.Key9,
	glfw.KeyF1:           input.KeyF1,
	glfw.KeyF2:           input.KeyF2,
	glfw.KeyF3:           input.KeyF3,
	glfw.KeyF4:           input.KeyF4,
	glfw.KeyF5:           input.KeyF5,
	glfw.KeyF6:           input.KeyF6,
	glfw.KeyF7:           input.KeyF7,
	glfw.KeyF8:           input.KeyF8,
	glfw.KeyF9:           input.KeyF9,
	glfw.KeyF10:          input.KeyF10,
	glfw.KeyF11:          input.KeyF11,
	glfw.KeyF12:          input.KeyF12,
	glfw.KeyEscape:       input.KeyEscape,
	glfw.KeyEnter:        input.KeyEnter,
	glfw.KeyTab:          input.KeyTab,
	glfw.KeySpace:        input.KeySpace,
	glfw.KeyBackspace:    input.KeyBackspace,
	glfw.KeyDelete:       input.KeyDelete,
	glfw.KeyInsert:       input.KeyInsert,
	glfw.KeyLeft:         input.KeyLeft,
	glfw.KeyRight:        input.KeyRight,
	glfw.KeyUp:           input.KeyUp,
	glfw.KeyDown:         input.KeyDown,
	glfw.KeyHome:         input.KeyHome,
	glfw.KeyEnd:          input.KeyEnd,
	glfw.KeyPageUp:       input.KeyPageUp,
	glfw.KeyPageDown:     input.KeyPageDown,
	glfw.KeyMinus:        input.KeyMinus,
	glfw.KeyEqual:        input.KeyEqual,
	glfw.KeyLeftBracket:  input.KeyLeftBracket,
	glfw.KeyRightBracket: input.KeyRightBracket,
	glfw.KeyBackslash:    input.KeyBackslash,
	glfw.KeySemicolon:    input.KeySemicolon,
	glfw.KeyApostrophe:   input.KeyApostrophe,
	glfw.KeyGraveAccent:  input.KeyGraveAccent,
	glfw.KeyComma:        input.KeyComma,
	glfw.KeyPeriod:       input.KeyPeriod,
	glfw.KeySlash:        input.KeySlash,
	glfw.KeyLeftShift:    input.KeyLeftShift,
	glfw.KeyRightShift:   input.KeyRightShift,
	glfw.KeyLeftControl:  input.KeyLeftControl,
	glfw.KeyRightControl: input.KeyRightControl,
	glfw.KeyLeftAlt:      input.KeyLeftAlt,
	glfw.KeyRightAlt:     input.KeyRightAlt,
	glfw.KeyLeftSuper:    input.KeyLeftSuper,
	glfw.KeyRightSuper:   input.KeyRightSuper,
	glfw.KeyCapsLock:     input.KeyCapsLock,
	glfw.KeyMenu:         input.KeyMenu,
}

func keyOf(k glfw.Key) input.Key { return keys[k] }

func modsOf(m glfw.ModifierKey) input.Mods {
	var out input.Mods
	if m&glfw.ModShift != 0 {
		out |= input.ModShift
	}
	if m&glfw.ModControl != 0 {
		out |= input.ModControl
	}
	if m&glfw.ModAlt != 0 {
		out |= input.ModAlt
	}
	if m&glfw.ModSuper != 0 {
		out |= input.ModSuper
	}
	return out
}

func buttonOf(b glfw.MouseButton) (input.Button, bool) {
	switch b {
	case glfw.MouseButtonLeft:
		return input.ButtonPrimary, true
	case glfw.MouseButtonRight:
		return input.ButtonSecondary, true
	case glfw.MouseButtonMiddle:
		return input.ButtonMiddle, true
	default:
		// Side buttons have no gunim name yet.
		return 0, false
	}
}
