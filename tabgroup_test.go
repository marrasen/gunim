package gunim

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

// tabGroupPair is a focus pair that Tab visits as one stop, and that records the focus coming and going.
type tabGroupPair struct {
	focusPair
	heard []input.Event
}

func (*tabGroupPair) TabGroup() {}

func (g *tabGroupPair) Handle(e input.Event, _ *UI) bool {
	switch e.(type) {
	case input.FocusEntered, input.FocusLeft:
		g.heard = append(g.heard, e)
	}
	return false
}

func TestTabVisitsAGroupAsOneStopAndComesBackToWhereItWas(t *testing.T) {
	w := newTestWindow()
	before, after := &focusRecorder{}, &focusRecorder{}
	g := &tabGroupPair{focusPair: focusPair{fa: &focusRecorder{}, fb: &focusRecorder{}}}
	w.ui.Insert(w.ui.Root(), before)
	w.ui.Insert(w.ui.Root(), g)
	w.ui.Insert(w.ui.Root(), after)
	run(w, 1)
	// The recorders take every key, so the test moves the focus as Tab does
	tab := func(shift bool) Node {
		w.ui.FocusNext(!shift)
		run(w, 1)
		return w.ui.Focused()
	}
	order := make([]Node, 0, 4)
	for range 4 {
		order = append(order, tab(false))
	}
	if order[0] != before || order[1] != g.fa || order[2] != after || order[3] != before {
		t.Fatalf("Tab went %v, want before, the group's first, after, and round to before", order)
	}
	// The group's arrows move within it, and Tab comes back to where they left it
	tab(false)
	if !w.ui.FocusWithin(g, true) || w.ui.Focused() != g.fb {
		t.Fatal("FocusWithin did not move to the group's second node")
	}
	if w.ui.FocusWithin(g, true) {
		t.Fatal("FocusWithin moved past the group's last node")
	}
	tab(false)
	if got := tab(true); got != g.fb {
		t.Fatalf("Shift+Tab back into the group focused %v, want the node that last had the focus", got)
	}
}

func TestAGroupHearsTheFocusComeInAndGoOnce(t *testing.T) {
	w := newTestWindow()
	before := &focusRecorder{}
	g := &tabGroupPair{focusPair: focusPair{fa: &focusRecorder{}, fb: &focusRecorder{}}}
	w.ui.Insert(w.ui.Root(), before)
	w.ui.Insert(w.ui.Root(), g)
	run(w, 1)
	w.ui.Focus(before)
	w.ui.Focus(g.fa)
	w.ui.FocusWithin(g, true)
	w.ui.Focus(before)
	if len(g.heard) != 2 {
		t.Fatalf("the group heard %v, want the focus entering and leaving, once each", g.heard)
	}
	if _, ok := g.heard[0].(input.FocusEntered); !ok {
		t.Fatalf("the group first heard %v, want FocusEntered", g.heard[0])
	}
	if _, ok := g.heard[1].(input.FocusLeft); !ok {
		t.Fatalf("the group then heard %v, want FocusLeft", g.heard[1])
	}
}
