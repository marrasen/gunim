package gunim

import (
	"testing"

	"github.com/marrasen/gunim/input"
)

// tabGroupPair is a focus pair that Tab visits as one stop, and that records the focus coming and going.
type tabGroupPair struct {
	focusPair
	heard []input.FocusRing
}

func (*tabGroupPair) TabGroup() {}

func (g *tabGroupPair) Handle(e input.Event, _ *UI) bool {
	if r, ok := e.(input.FocusRing); ok {
		g.heard = append(g.heard, r)
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

func TestAGroupShowsItsRingWhileTheKeyboardIsInIt(t *testing.T) {
	w := newTestWindow()
	before := &focusRecorder{}
	g := &tabGroupPair{focusPair: focusPair{fa: &focusRecorder{}, fb: &focusRecorder{}}}
	w.ui.Insert(w.ui.Root(), before)
	w.ui.Insert(w.ui.Root(), g)
	run(w, 1)
	on := func() []bool {
		out := make([]bool, 0, len(g.heard))
		for _, e := range g.heard {
			out = append(out, e.On)
		}
		return out
	}
	// Focus put in the group with no Tab shows no ring; Tab shows it
	w.ui.Focus(g.fa)
	if len(g.heard) != 0 {
		t.Fatalf("with the mouse in use the group heard %v, want nothing", g.heard)
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	if len(g.heard) != 0 {
		t.Fatalf("an arrow key showed the group's ring: %v", g.heard)
	}
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.ui.FocusWithin(g, true)
	w.ui.Focus(before)
	if got := on(); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("the group's ring went %v, want on as Tab was pressed, and off as the focus left", got)
	}
	if !g.heard[0].Within {
		t.Fatal("the group's ring is not for the focus within it")
	}
	// A click hides the ring of the node with the focus
	w.ui.Focus(g.fa)
	w.Input(input.PointerDown{Button: input.ButtonPrimary})
	if got := on(); len(got) != 4 || !got[2] || got[3] {
		t.Fatalf("the group's ring went %v, want on again, then off after the click", got)
	}
}
