package gunim

import "testing"

// tabGroupPair is a focus pair that Tab visits as one stop.
type tabGroupPair struct{ focusPair }

func (*tabGroupPair) TabGroup() {}

func TestTabVisitsAGroupAsOneStopAndComesBackToWhereItWas(t *testing.T) {
	w := newTestWindow()
	before, after := &focusRecorder{}, &focusRecorder{}
	g := &tabGroupPair{focusPair{fa: &focusRecorder{}, fb: &focusRecorder{}}}
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
