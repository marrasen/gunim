package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// catcher is a [KeyCatcher] that takes F10 and AltTapped, and records what it was offered.
type catcher struct{ caught []input.Event }

func (c *catcher) CatchKey(e input.Event, _ *UI) bool {
	c.caught = append(c.caught, e)
	switch e := e.(type) {
	case input.KeyPress:
		return e.Key == input.KeyF10
	case input.AltTapped:
		return true
	}
	return false
}

func (c *catcher) Layout(cs Constraints, _ Frame, _ Children) geom.Size { return geom.Sz(cs.Max.W, 10) }
func (c *catcher) Paint(*paint.Painter, Frame, geom.Size, Children)     {}

// taker is a focusable node that takes F1 alone.
type taker struct{ took int }

func (k *taker) Focusable() bool { return true }
func (k *taker) Handle(e input.Event, _ *UI) bool {
	if p, ok := e.(input.KeyPress); ok && p.Key == input.KeyF1 {
		k.took++
		return true
	}
	return false
}
func (k *taker) Layout(cs Constraints, _ Frame, _ Children) geom.Size { return geom.Sz(cs.Max.W, 10) }
func (k *taker) Paint(*paint.Painter, Frame, geom.Size, Children)     {}

// taps counts the AltTapped events in es.
func taps(es []input.Event) int {
	n := 0
	for _, e := range es {
		if _, ok := e.(input.AltTapped); ok {
			n++
		}
	}
	return n
}

func TestAKeyCatcherHearsWhatTheFocusedNodeLeaves(t *testing.T) {
	w := newTestWindow()
	c, k := &catcher{}, &taker{}
	w.ui.Insert(w.ui.Root(), c)
	w.ui.Insert(w.ui.Root(), k)
	run(w, 1)
	w.ui.Focus(k)
	w.ui.handlePlatform(input.KeyPress{Key: input.KeyF1})
	w.ui.handlePlatform(input.KeyPress{Key: input.KeyF10})
	if k.took != 1 || len(c.caught) != 1 || c.caught[0] != (input.KeyPress{Key: input.KeyF10}) {
		t.Fatalf("the focused node took %d keys and the catcher heard %v; want F1 taken and F10 caught", k.took, c.caught)
	}
}

func TestAltPressedAndLetGoAloneIsATap(t *testing.T) {
	w := newTestWindow()
	c := &catcher{}
	w.ui.Insert(w.ui.Root(), c)
	run(w, 1)
	key := func(k input.Key, mods input.Mods, down bool) {
		if down {
			w.ui.handlePlatform(input.KeyPress{Key: k, Mods: mods, Time: time.Now()})
		} else {
			w.ui.handlePlatform(input.KeyRelease{Key: k, Mods: mods, Time: time.Now()})
		}
	}
	tapAlt := func() {
		key(input.KeyLeftAlt, input.ModAlt, true)
		key(input.KeyLeftAlt, 0, false)
	}
	tapAlt()
	if n := taps(c.caught); n != 1 {
		t.Fatalf("Alt pressed and let go made %d taps, want 1", n)
	}

	// Alt+F, AltGr, a click and a held Alt that repeats.
	key(input.KeyLeftAlt, input.ModAlt, true)
	key(input.KeyF, input.ModAlt, true)
	key(input.KeyF, input.ModAlt, false)
	key(input.KeyLeftAlt, 0, false)
	key(input.KeyRightAlt, input.ModAlt|input.ModControl, true)
	key(input.KeyRightAlt, input.ModControl, false)
	key(input.KeyLeftAlt, input.ModAlt, true)
	press(w, 5, 5)
	key(input.KeyLeftAlt, 0, false)
	if n := taps(c.caught); n != 1 {
		t.Fatalf("Alt+F, AltGr and Alt with a click made %d more taps, want none", n-1)
	}
	key(input.KeyLeftAlt, input.ModAlt, true)
	w.ui.handlePlatform(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt, Repeat: true})
	key(input.KeyLeftAlt, 0, false)
	if n := taps(c.caught); n != 2 {
		t.Fatalf("Alt held until it repeats made %d taps, want 1", n-1)
	}
}
