package driver

import "errors"

// ErrNoDriver is returned when no platform driver is built in for the
// operating system gunim is running on.
var ErrNoDriver = errors.New("driver: no platform driver for this operating system")

// A Raiser is a [Window] that can raise the priority of the calling
// thread. The engine calls RaiseThread from its UI goroutine, which it
// keeps on one thread for life, so the work of making frames stays on
// time when other programs load the CPU.
type Raiser interface {
	RaiseThread()
}

// A TextInputter is a [Window] that can be told when the application is
// taking text, so the platform's input method composes into it: it
// holds the key presses it needs while the user composes, and reports
// the composition as [github.com/marrasen/gunim/input.Composing] and the
// result as [github.com/marrasen/gunim/input.TextInput]. Setting it false ends any composition.
type TextInputter interface {
	SetTextInput(active bool)
}

// Redraw is sent on [Window.Input] when the window needs drawing again
// with no input behind it: after a resize, a move to another monitor,
// or the display server asking for the contents back.
type Redraw struct{}
