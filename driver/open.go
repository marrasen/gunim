package driver

import (
	"errors"

	"github.com/marrasen/gunim/geom"
)

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

// A CaretPlacer is a [Window] that can tell the platform's input method
// where the text caret is, in logical pixels of window space, so its
// candidate window opens beside the text being composed.
type CaretPlacer interface {
	SetTextCaret(r geom.Rect)
}

// Redraw is sent on [Window.Input] when the window needs drawing again
// with no input behind it: after a resize, a move to another monitor,
// or the display server asking for the contents back.
type Redraw struct{}

// A Placer is a popup [Window] that can move and resize: to fit new
// content, or to follow its anchor. anchor is in the parent's logical
// space, as in [Options], and size is in logical pixels.
type Placer interface {
	Place(anchor geom.Rect, size geom.Size) error
}

// WindowFocus is sent on [Window.Input] when the window gains or loses
// the keyboard.
type WindowFocus struct{ Focused bool }

// A Transparent is a [Window] that can say whether it shows what is
// behind it wherever nothing is painted. A popup is transparent where
// the display server can blend windows.
type Transparent interface {
	Transparent() bool
}

// A Screener is a [Window] that knows where it is on the screen. It
// turns a point in its own logical space into screen coordinates, and
// back, so a drag can go from one window to another.
type Screener interface {
	ToScreen(p geom.Point) geom.Point
	FromScreen(p geom.Point) geom.Point
}
