package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// testModal is a modal that takes the keyboard, as a dialog does.
type testModal struct{ focusRecorder }

func (*testModal) Modal() bool { return true }

// scopeBox is a box that keeps the modals inside it to itself.
type scopeBox struct{ Box }

func (*scopeBox) ModalScope() {}

// A modal in a scope holds the keyboard there only: the rest of the
// window still takes it, and Tab into the scope lands on the modal.
func TestAModalInAScopeHoldsTheKeyboardThereOnly(t *testing.T) {
	w := newTestWindow()
	outside, inner := &focusRecorder{}, &focusRecorder{}
	scope := &scopeBox{}
	w.ui.Insert(w.ui.Root(), outside)
	w.ui.Insert(w.ui.Root(), scope)
	w.ui.Insert(scope, inner)
	run(w, 1)
	w.ui.Focus(outside)
	m := &testModal{}
	w.ui.Insert(scope, m)
	run(w, 1)
	if w.ui.Focused() != outside {
		t.Fatalf("a modal coming in a scope took the keyboard from outside it")
	}
	if w.ui.Focus(inner) {
		t.Fatalf("the node under the scope's modal took the keyboard")
	}
	if !w.ui.Focus(m) || !w.ui.Focus(outside) {
		t.Fatalf("the keyboard did not go to the modal and back out of the scope")
	}
	w.ui.FocusNext(true)
	if w.ui.Focused() != m {
		t.Fatalf("Tab from outside went to %T, want the scope's modal", w.ui.Focused())
	}
	// A modal over the whole window keeps the keyboard from everything else.
	whole := &testModal{}
	w.ui.Insert(w.ui.Root(), whole)
	run(w, 1)
	if w.ui.Focused() != whole || w.ui.Focus(outside) || w.ui.Focus(m) {
		t.Fatalf("a modal over the whole window let the keyboard go")
	}
}

// A modal that came into its scope while the keyboard was elsewhere,
// and was then given it, gives it back inside the scope as it leaves,
// not to where it was before.
func TestAScopedModalGivesTheKeyboardBackInItsScope(t *testing.T) {
	w := newTestWindow()
	outside, inner := &focusRecorder{}, &focusRecorder{}
	scope := &scopeBox{}
	w.ui.Insert(w.ui.Root(), outside)
	w.ui.Insert(w.ui.Root(), scope)
	w.ui.Insert(scope, inner)
	run(w, 1)
	w.ui.Focus(outside)
	m := &testModal{}
	w.ui.Insert(scope, m)
	run(w, 1)
	w.ui.Focus(m)
	w.ui.Remove(m)
	run(w, 2)
	if f := w.ui.Focused(); f == outside {
		t.Fatalf("the keyboard went back out of the scope as its modal left")
	}
}

// Keys pressed in a scoped modal go no further than the modal.
func TestKeysInAScopedModalStopThere(t *testing.T) {
	w := newTestWindow()
	scope := &scopeBox{}
	catcher := &recorder{}
	w.ui.Insert(w.ui.Root(), catcher)
	w.ui.Insert(catcher, scope)
	m := &passModal{}
	w.ui.Insert(scope, m)
	run(w, 1)
	w.ui.Focus(m)
	w.ui.keyEvent(input.KeyPress{Key: input.KeyA, Time: time.Now()})
	if catcher.got(input.KeyPress{}) {
		t.Fatalf("a key pressed in a scoped modal reached the node round its scope")
	}
}

// passModal is a modal that takes the keyboard and lets every event by.
type passModal struct{ testModal }

func (*passModal) Handle(input.Event, *UI) bool { return false }

// A node given an ID takes views mounted under it, and the intents sent
// from inside them come from those views; the ID goes with the node.
func TestANodeGivenAnIDTakesViews(t *testing.T) {
	w := newTestWindow()
	RegisterView(w, "page", func(struct{}) *focusRecorder { return &focusRecorder{} }, nil)
	host := &Box{}
	w.ui.Insert(w.ui.Root(), host)
	if err := w.ui.SetID(host, "host"); err != nil {
		t.Fatal(err)
	}
	other := &Box{}
	w.ui.Insert(w.ui.Root(), other)
	if err := w.ui.SetID(other, "host"); err == nil {
		t.Fatalf("a second node took an ID already taken")
	}
	c := w.Client()
	if err := c.Mount("host", "host/page", "page", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	page := w.ui.ids["host/page"]
	if page == nil || page.parent != w.ui.index[host] {
		t.Fatalf("the view did not mount under the node given the ID")
	}
	if got := w.ui.idOf(page.node); got != "host/page" {
		t.Fatalf("an intent from the view comes from %q, want host/page", got)
	}
	if got := w.ui.idOf(host); got != "host" {
		t.Fatalf("an intent from the node comes from %q, want host", got)
	}
	w.ui.Remove(host)
	run(w, 60)
	if _, ok := w.ui.ids["host"]; ok {
		t.Fatalf("the ID stayed once its node had left")
	}
	if err := w.ui.SetID(other, "host"); err != nil {
		t.Fatalf("the ID could not be given again once its node had left: %v", err)
	}
}
