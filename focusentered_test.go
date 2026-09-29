package gunim

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

// enteredPair is a focus pair that counts the times the focus came into
// it from outside.
type enteredPair struct {
	focusPair
	entered int
}

func (p *enteredPair) Handle(e input.Event, _ *UI) bool {
	if _, ok := e.(input.FocusEntered); ok {
		p.entered++
	}
	return false
}

// A node round the focus hears it come in from outside once, and not as
// it moves among the nodes inside.
func TestANodeHearsTheFocusComeIntoIt(t *testing.T) {
	w := newTestWindow()
	outside := &focusRecorder{}
	p := &enteredPair{focusPair: focusPair{fa: &focusRecorder{}, fb: &focusRecorder{}}}
	w.ui.Insert(w.ui.Root(), outside)
	w.ui.Insert(w.ui.Root(), p)
	run(w, 1)
	w.ui.Focus(outside)
	w.ui.Focus(p.fa)
	w.ui.Focus(p.fb)
	if p.entered != 1 {
		t.Fatalf("coming in and moving inside, the pair heard %d entries, want 1", p.entered)
	}
	w.ui.Focus(outside)
	w.ui.Focus(p.fb)
	if p.entered != 2 {
		t.Fatalf("coming in again, the pair heard %d entries, want 2", p.entered)
	}
}
