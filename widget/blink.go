package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/theme"
)

// Caret tokens: how long the caret shows, and hides, in each blink, in seconds, and for how long after the last
// key or click it blinks before it stays lit. A CaretBlink of 0 keeps the caret lit.
var (
	CaretBlink    = theme.Number("caret.blink", 0.53)
	CaretBlinkFor = theme.Number("caret.blink.for", 10)
)

// blinkFade is how long the caret takes to fade out or in.
const blinkFade = 110 * time.Millisecond

// blinker blinks a caret while its widget has the keyboard, and stops once the user has left it alone a while, so
// an idle window draws nothing.
type blinker struct {
	lit  *anim.Float
	stop func()
	// left counts the fades still to come before the caret stays lit.
	left int
}

// value returns how lit the caret is, from 0 to 1.
func (b *blinker) value() float32 {
	if b.lit == nil {
		return 1
	}
	return min(max(b.lit.Value(), 0), 1)
}

// restart lights the caret at once and starts it blinking again, after a key, a click or the keyboard arriving.
func (b *blinker) restart(u *gunim.UI) {
	b.halt()
	if b.lit == nil {
		b.lit = anim.NewFloat(1)
	}
	b.lit.Jump(1)
	half := CaretBlink.Get(u.Theme())
	if half <= 0 {
		return
	}
	b.left = int(CaretBlinkFor.Get(u.Theme()) / half)
	b.next(u, half)
}

// next fades the caret the other way after half a blink, and goes on until no fades are left.
func (b *blinker) next(u *gunim.UI, half float32) {
	b.stop = u.After(time.Duration(half*float32(time.Second)), func(u *gunim.UI) {
		b.stop = nil
		out := b.lit.Target() > 0.5
		if b.left <= 0 && out {
			return // lit, and done blinking
		}
		b.left--
		b.lit.Animate(map[bool]float32{false: 1, true: 0}[out], anim.Tween{Duration: blinkFade, Ease: anim.EaseInOut})
		u.Invalidate()
		b.next(u, half)
	})
}

// halt stops the blinking and leaves the caret lit, as the keyboard leaves.
func (b *blinker) halt() {
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
	if b.lit != nil {
		b.lit.Jump(1)
	}
}

// step implements the blinker's part of an [gunim.Animator].
func (b *blinker) step(dt time.Duration) bool {
	if b.lit == nil {
		return false
	}
	return b.lit.Step(dt)
}
