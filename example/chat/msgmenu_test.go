package main

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// onUI is a patch that runs a function with the window's UI.
type onUI struct{ fn func(u *gunim.UI) }

func TestAMessageOpensAMenuOfWhatItsToolbarDoes(t *testing.T) {
	h := newHarness(t)
	gunim.RegisterPatch(h.w, "chat", func(_ *chatView, p onUI, u *gunim.UI) { p.fn(u) })
	do := func(fn func(u *gunim.UI)) {
		t.Helper()
		if err := h.w.Client().Patch("chat", onUI{fn}); err != nil {
			t.Fatal(err)
		}
		h.frames(2)
	}
	h.typeAndSend("Mine to change")
	h.frames(5)
	it, _ := h.item("Mine to change")
	n, ok := h.v.list.Row(widget.Key(it.Key))
	if !ok {
		t.Fatal("the message is not built")
	}
	r, ok := n.(*msgRow)
	if !ok {
		t.Fatalf("the message is a %T, want a *msgRow", n)
	}
	// A right click, or a finger held on the message, as the engine
	// sends it.
	do(func(u *gunim.UI) {
		r.Handle(input.PointerDown{Pos: geom.Pt(200, 20), Button: input.ButtonSecondary, Clicks: 1, Touch: true}, u)
	})
	if r.menuList == nil {
		t.Fatal("no menu opened")
	}
	items := make([]string, 0, len(r.menuList.Items()))
	for _, it := range r.menuList.Items() {
		items = append(items, it.Label)
	}
	if want := []string{"React", "Reply", "Edit", "Withdraw"}; !slices.Equal(items, want) {
		t.Fatalf("the menu holds %q, want %q", items, want)
	}
	do(func(u *gunim.UI) { r.menuList.Pick(1, u) })
	h.frames(5)
	if r.menu != nil {
		t.Fatal("the menu stayed open after a pick")
	}
	if h.a.replying != it.ID {
		t.Fatalf("Reply left the reply on %q, want the message, %q", h.a.replying, it.ID)
	}
}
