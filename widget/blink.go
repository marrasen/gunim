package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/theme"
)

// CaretBlink is how long the caret shows, and hides, in each blink, in seconds. 0 keeps the caret lit.
var CaretBlink = theme.Number("caret.blink", 0.53)

// blinker blinks a caret while its widget has the keyboard and its window is active. The caret turns on and off
// rather than fading, so a blink costs two frames a second.
type blinker struct {
	dark bool
	stop func()
	// away hides the caret while the window does not have the keyboard.
	away bool
}

// value returns how lit the caret is: 1 or 0.
func (b *blinker) value() float32 {
	if b.away || b.dark {
		return 0
	}
	return 1
}

// restart lights the caret at once and starts it blinking again, after a key, a click or the keyboard arriving.
func (b *blinker) restart(u *gunim.UI) {
	b.halt()
	if half := CaretBlink.Get(u.Theme()); half > 0 {
		b.next(u, half)
	}
}

// next turns the caret the other way after half a blink, and goes on.
func (b *blinker) next(u *gunim.UI, half float32) {
	b.stop = u.After(time.Duration(half*float32(time.Second)), func(u *gunim.UI) {
		b.dark = !b.dark
		u.Invalidate()
		b.next(u, half)
	})
}

// windowFocus hides the caret while the window is without the keyboard, and shows it blinking again when the window
// has it back. It reports whether e was such a change, which goes on to the nodes around the widget too.
func (b *blinker) windowFocus(e input.Event, u *gunim.UI) bool {
	switch e.(type) {
	case input.WindowFocusLost:
		b.halt()
		b.away = true
	case input.WindowFocusGained:
		b.away = false
		b.restart(u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// halt stops the blinking and leaves the caret lit, as the keyboard leaves.
func (b *blinker) halt() {
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
	b.dark = false
}

// step implements the blinker's part of an [gunim.Animator]: the caret has nothing to animate.
func (b *blinker) step(time.Duration) bool { return false }
