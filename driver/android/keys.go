//go:build android

package android

import "github.com/marrasen/gunim/input"

// keyOf returns the key an Android key code names. Back is Escape,
// which closes what Escape closes.
func keyOf(code int) input.Key {
	switch {
	case code >= 29 && code <= 54:
		return input.KeyA + input.Key(code-29)
	case code >= 7 && code <= 16:
		return input.Key0 + input.Key(code-7)
	case code >= 131 && code <= 142:
		return input.KeyF1 + input.Key(code-131)
	case code >= 144 && code <= 153:
		return input.KeyKP0 + input.Key(code-144)
	}
	if k, ok := keys[code]; ok {
		return k
	}
	return input.KeyUnknown
}

// keys maps the Android key codes that keyOf leaves to a table.
var keys = map[int]input.Key{
	4:   input.KeyEscape, // KEYCODE_BACK
	111: input.KeyEscape,
	66:  input.KeyEnter,
	61:  input.KeyTab,
	62:  input.KeySpace,
	67:  input.KeyBackspace, // KEYCODE_DEL
	112: input.KeyDelete,    // KEYCODE_FORWARD_DEL
	124: input.KeyInsert,
	21:  input.KeyLeft,
	22:  input.KeyRight,
	19:  input.KeyUp,
	20:  input.KeyDown,
	122: input.KeyHome,
	123: input.KeyEnd,
	92:  input.KeyPageUp,
	93:  input.KeyPageDown,
	69:  input.KeyMinus,
	70:  input.KeyEqual,
	71:  input.KeyLeftBracket,
	72:  input.KeyRightBracket,
	73:  input.KeyBackslash,
	74:  input.KeySemicolon,
	75:  input.KeyApostrophe,
	68:  input.KeyGraveAccent,
	55:  input.KeyComma,
	56:  input.KeyPeriod,
	76:  input.KeySlash,
	59:  input.KeyLeftShift,
	60:  input.KeyRightShift,
	113: input.KeyLeftControl,
	114: input.KeyRightControl,
	57:  input.KeyLeftAlt,
	58:  input.KeyRightAlt,
	117: input.KeyLeftSuper,
	118: input.KeyRightSuper,
	115: input.KeyCapsLock,
	82:  input.KeyMenu,
	158: input.KeyKPDecimal,
	154: input.KeyKPDivide,
	155: input.KeyKPMultiply,
	156: input.KeyKPSubtract,
	157: input.KeyKPAdd,
	160: input.KeyKPEnter,
	161: input.KeyKPEqual,
	121: input.KeyPause,
	120: input.KeyPrintScreen,
	116: input.KeyScrollLock,
	143: input.KeyNumLock,
}

// Android's meta state bits.
const (
	metaShift = 0x1
	metaAlt   = 0x2
	metaCtrl  = 0x1000
	metaMeta  = 0x10000
)

// modsOf returns the modifiers an Android meta state holds.
func modsOf(meta int) input.Mods {
	var m input.Mods
	if meta&metaShift != 0 {
		m |= input.ModShift
	}
	if meta&metaCtrl != 0 {
		m |= input.ModControl
	}
	if meta&metaAlt != 0 {
		m |= input.ModAlt
	}
	if meta&metaMeta != 0 {
		m |= input.ModSuper
	}
	return m
}
