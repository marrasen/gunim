package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// wentTo is the intent an address bar sends for a path.
type wentTo struct{ path string }

func TestAnAddressBarsPlacesAreOneTabStopAndEnterGoesThere(t *testing.T) {
	a := NewAddressBar()
	a.OnGo = func(path string) gunim.Intent { return wentTo{path} }
	after := NewButton("After")
	w, run := stage(t, &frame{child: Row(a, after).Grow(a, 1), size: geom.Sz(600, 40)})
	focused := focusProbe(t, w, run)
	type set struct{}
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ set, u *gunim.UI) {
		a.SetPath("/x/y/z", []Crumb{{"x", "/x"}, {"y", "/x/y"}, {"z", "/x/y/z"}}, u)
	})
	if err := w.Client().Patch("stage", set{}); err != nil {
		t.Fatal(err)
	}
	run(60)
	press := func(k input.Key, mods input.Mods) {
		w.Input(input.KeyPress{Key: k, Mods: mods})
		w.Input(input.KeyRelease{Key: k, Mods: mods})
		run(20)
	}
	on := func() int {
		for i, c := range a.crumbs.crumbs {
			if focused() == gunim.Node(c.btn) {
				return i
			}
		}
		return -1
	}
	press(input.KeyTab, 0)
	if on() != 0 {
		t.Fatal("Tab did not land on the first place")
	}
	if a.crumbs.ring.Value() < 0.9 {
		t.Fatal("the bar shows no ring with the keyboard on a place")
	}
	press(input.KeyTab, 0)
	if focused() != after {
		t.Fatal("a second Tab did not leave the places, which are one stop")
	}
	press(input.KeyTab, input.ModShift)
	press(input.KeyRight, 0)
	if on() != 1 {
		t.Fatal("Right did not move to the second place")
	}
	press(input.KeyEnter, 0)
	var went []string
	for _, ev := range sent(w) {
		if g, ok := ev.(wentTo); ok {
			went = append(went, g.path)
		}
	}
	if len(went) != 1 || went[0] != "/x/y" {
		t.Fatalf("Enter on the second place went to %v, want /x/y", went)
	}
}
