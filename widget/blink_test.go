package widget

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

func TestTheCaretBlinksOnAndOffForAsLongAsItHasTheKeyboard(t *testing.T) {
	wr := newWriter(t, 400)
	b := &wr.area.blink
	if b.value() != 1 {
		t.Fatalf("caret at %v on arriving, want lit", b.value())
	}
	// Half a blink later, the caret is out.
	wr.run(35)
	if v := b.value(); v != 0 {
		t.Fatalf("caret at %v half a blink in, want out", v)
	}
	// A key lights it at once.
	wr.typeText("a")
	if v := b.value(); v != 1 {
		t.Fatalf("caret at %v after a key, want lit", v)
	}
	// Long after the last key it still blinks.
	lit, dark := false, false
	wr.run(60 * 60)
	for range 90 {
		wr.run(1)
		if b.value() == 1 {
			lit = true
		} else {
			dark = true
		}
	}
	if !lit || !dark {
		t.Fatalf("a minute after the last key the caret was lit %v and dark %v, want it still blinking", lit, dark)
	}
}

func TestTheCaretHidesWhileTheWindowIsInactive(t *testing.T) {
	wr := newWriter(t, 400)
	b := &wr.area.blink
	wr.w.Input(input.WindowFocusLost{})
	wr.run(1)
	if v := b.value(); v != 0 {
		t.Fatalf("caret at %v with the window inactive, want hidden", v)
	}
	if b.stop != nil {
		t.Fatal("the caret still blinks with the window inactive")
	}
	wr.w.Input(input.WindowFocusGained{})
	wr.run(1)
	if v := b.value(); v != 1 {
		t.Fatalf("caret at %v with the window back, want lit", v)
	}
	if b.stop == nil {
		t.Fatal("the caret does not blink again with the window back")
	}
}
