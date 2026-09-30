package driver

import (
	"errors"

	"github.com/marrasen/gunim/input"
)

// HotKey is a key and its modifiers that reach the application whatever
// program has the keyboard, as a launcher's key does.
type HotKey struct {
	Key  input.Key
	Mods input.Mods
}

// ErrHotKeyTaken says another program holds the key already, and
// ErrNoHotKeys that the platform gives an application none.
var (
	ErrHotKeyTaken = errors.New("another program has that key")
	ErrNoHotKeys   = errors.New("there are no keys for all programs here")
)

// A HotKeyer is a [Driver] that can take a key from every program.
type HotKeyer interface {
	// RegisterHotKey calls fn, on a goroutine of the driver's, each time
	// k is pressed, whatever program has the keyboard, until release is
	// called.
	RegisterHotKey(k HotKey, fn func()) (release func(), err error)
}
