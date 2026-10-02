package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/theme"
)

// CaretBlink is how long the caret shows, and hides, in each blink, in seconds, fades included. 0 keeps the caret
// lit.
var CaretBlink = theme.Number("caret.blink", 0.53)

// The caret fades out and in over blinkFade, in fadeSteps steps on timers rather than on every frame, so a blink
// costs the same few frames a second at any refresh rate.
const (
	blinkFade = 120 * time.Millisecond
	fadeSteps = 4
)

// blinker blinks a caret while its widget has the keyboard and its window is active, fading it out and back in.
type blinker struct {
	// level is how lit the caret is, from 0 to 1, while it blinks.
	level float32
	on    bool
	stop  func()
	// away hides the caret while the window does not have the keyboard.
	away bool
}

// value returns how lit the caret is, from 0 to 1.
func (b *blinker) value() float32 {
	if b.away {
		return 0
	}
	if !b.on {
		return 1
	}
	return b.level
}

// restart lights the caret at once and starts it blinking again, after a key, a click or the keyboard arriving. In
// a window without the keyboard, as a field focused in a window behind another, it stays hidden until the keyboard
// comes.
func (b *blinker) restart(u *gunim.UI) {
	b.halt()
	b.away = u.KeyboardAway()
	if b.away {
		return
	}
	half := CaretBlink.Get(u.Theme())
	if half <= 0 {
		return
	}
	b.on, b.level = true, 1
	b.hold(u, time.Duration(half*float32(time.Second)))
}

// hold keeps the caret as it is for what half a blink leaves after its fade, then fades it the other way.
func (b *blinker) hold(u *gunim.UI, half time.Duration) {
	from := b.level
	b.stop = u.After(max(half-blinkFade, 0), func(u *gunim.UI) { b.fade(u, half, from, 1) })
}

// fade takes step i of the fade away from level from, and goes on to the next step, or to holding at the end.
func (b *blinker) fade(u *gunim.UI, half time.Duration, from float32, i int) {
	t := float32(i) / fadeSteps
	eased := t * t * (3 - 2*t)
	b.level = from + (1-2*from)*eased
	u.Invalidate()
	if i < fadeSteps {
		b.stop = u.After(blinkFade/fadeSteps, func(u *gunim.UI) { b.fade(u, half, from, i+1) })
		return
	}
	b.hold(u, half)
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
	b.on, b.level = false, 1
}

// step implements the blinker's part of an [gunim.Animator]: the fade steps on timers, so it has nothing to animate.
func (b *blinker) step(time.Duration) bool { return false }
