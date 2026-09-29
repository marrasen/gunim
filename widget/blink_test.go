package widget

import "testing"

func TestTheCaretBlinksThenStaysLit(t *testing.T) {
	wr := newWriter(t, 400)
	b := &wr.area.blink
	if b.value() != 1 {
		t.Fatalf("caret at %v on arriving, want lit", b.value())
	}
	// Half a blink and its fade later, the caret is out.
	wr.run(45)
	if v := b.value(); v > 0.05 {
		t.Fatalf("caret at %v half a blink in, want out", v)
	}
	// A key lights it at once.
	wr.typeText("a")
	if v := b.value(); v != 1 {
		t.Fatalf("caret at %v after a key, want lit", v)
	}
	wr.run(90)
	if v := b.value(); v < 0.95 {
		t.Fatalf("caret at %v a whole blink in, want lit", v)
	}
	// Left alone past CaretBlinkFor, it stays lit and asks for no more frames.
	wr.run(11 * 60)
	for range 3 * 60 {
		wr.run(1)
		if v := b.value(); v != 1 {
			t.Fatalf("caret at %v long after the last key, want it lit and still", v)
		}
	}
	if b.stop != nil {
		t.Fatal("a blink is still waiting long after the last key")
	}
}
