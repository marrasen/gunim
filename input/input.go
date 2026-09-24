// Package input holds the events a window delivers to its nodes, and
// the keys, buttons and modifiers they carry.
//
// It sits below both gunim and the driver, so a driver can report input
// in the same types a node receives.
package input

import (
	"time"

	"github.com/marrasen/gunim/geom"
)

// An Event is something that happened to a node: pointer, keyboard or
// focus. Nodes receive events by implementing gunim.Handler.
type Event interface{ isEvent() }

// Pointer events carry Pos in the receiving node's own coordinate
// space, with the origin at its top-left corner, so a node can work out
// where it was clicked from Pos alone.

// PointerEnter arrives when the pointer moves onto a node.
//
// The engine tracks the node under the pointer between frames and sends
// PointerEnter and [PointerLeave] itself, because hover is the single
// most common thing to animate, and one implementation beats every
// widget writing its own.
type PointerEnter struct {
	Pos  geom.Point
	Time time.Time
}

// PointerLeave arrives when the pointer moves off a node.
type PointerLeave struct {
	Time time.Time
}

// PointerMove arrives as the pointer travels across a node.
type PointerMove struct {
	Pos  geom.Point
	Mods Mods
	Time time.Time
}

// PointerDown arrives when a button goes down over a node.
type PointerDown struct {
	Pos    geom.Point
	Button Button
	Mods   Mods
	// Clicks counts this press within a rapid sequence: 1 for a single
	// click, 2 for the second of a double click.
	Clicks int
	Time   time.Time
}

// PointerUp arrives when a button comes back up.
type PointerUp struct {
	Pos    geom.Point
	Button Button
	Mods   Mods
	Time   time.Time
}

// Scroll carries wheel or touchpad movement. Delta is in logical
// pixels, so momentum scrolling keeps the fractional precision the
// touchpad reported.
type Scroll struct {
	Pos   geom.Point
	Delta geom.Point
	Mods  Mods
	Time  time.Time
}

// Keyboard events split press from release, and deliver the text a
// keystroke produced as its own [TextInput]. Polled key state collapses
// the two, so it reads Ctrl+C and the letter c the same way, which is
// the exact reason gridterm carries a fork of Ebitengine. The same
// shape of bug would rule gunim out for anything that cares about
// modifiers, held keys or key repeat, so the split is designed in from
// the start.

// KeyPress arrives when a key goes down, and again for each key repeat.
type KeyPress struct {
	Key  Key
	Mods Mods
	// Repeat is true when the system's key repeat produced the press,
	// and false for a fresh one.
	Repeat bool
	Time   time.Time
}

// KeyRelease arrives when a key comes back up.
type KeyRelease struct {
	Key  Key
	Mods Mods
	Time time.Time
}

// TextInput carries text the keystroke produced, after the input method
// has had its say. A node that wants characters reads this one; a node
// that wants shortcuts reads [KeyPress].
type TextInput struct {
	Text string
	Time time.Time
}

// Composing carries in-progress input method text: show it, and wait
// for [TextInput] to commit it.
type Composing struct {
	Text string
	// Selected is the range the input method has highlighted, as byte
	// offsets into Text.
	Selected [2]int
}

// FocusGained arrives when a node takes keyboard focus.
type FocusGained struct{ Time time.Time }

// FocusLost arrives when a node gives keyboard focus up.
type FocusLost struct{ Time time.Time }

func (PointerEnter) isEvent() {}
func (PointerLeave) isEvent() {}
func (PointerMove) isEvent()  {}
func (PointerDown) isEvent()  {}
func (PointerUp) isEvent()    {}
func (Scroll) isEvent()       {}
func (KeyPress) isEvent()     {}
func (KeyRelease) isEvent()   {}
func (TextInput) isEvent()    {}
func (Composing) isEvent()    {}
func (FocusGained) isEvent()  {}
func (FocusLost) isEvent()    {}

// Button identifies a pointer button.
type Button uint8

// The pointer buttons gunim reports.
const (
	ButtonPrimary Button = iota
	ButtonSecondary
	ButtonMiddle
)

// Mods is the set of modifier keys held when an event happened.
type Mods uint8

// The modifier keys gunim reports.
const (
	ModShift Mods = 1 << iota
	ModControl
	ModAlt
	// ModSuper is Command on macOS and the Windows key elsewhere.
	ModSuper
)

// Has reports whether every modifier in m is held.
func (m Mods) Has(want Mods) bool { return m&want == want }

// Key is a physical key, whatever layout is in force. Matching on the
// physical key keeps a shortcut on the same spot on the keyboard for
// QWERTY and Dvorak alike. Each key is named after what it shows on a
// US keyboard.
type Key uint16

// The keys gunim reports.
const (
	KeyUnknown Key = iota

	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ

	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9

	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	KeyEscape
	KeyEnter
	KeyTab
	KeySpace
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown

	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyApostrophe
	KeyGraveAccent
	KeyComma
	KeyPeriod
	KeySlash

	KeyLeftShift
	KeyRightShift
	KeyLeftControl
	KeyRightControl
	KeyLeftAlt
	KeyRightAlt
	KeyLeftSuper
	KeyRightSuper
	KeyCapsLock
	KeyMenu
)
