package main

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/widget"
)

// The page builds from saved values, and a setup shows All values with
// the changed values alone.
func TestThePageShowsTheSavedValuesAndTakesASetup(t *testing.T) {
	var p *page
	w := gunimtest.New(t, geom.Sz(1100, 700), widget.NewSurface())
	gunim.RegisterView(w, "page", func(s Page) *page { p = buildPage(s); return p }, nil)
	gunim.RegisterPatch(w, "page", func(p *page, s Setup, u *gunim.UI) { p.setup(s, u) })
	if err := w.Client().Mount(gunim.Root, "page", "page", Page{Saved: changes()}); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Patch("page", Setup{Tab: "all", Filter: "changed"}); err != nil {
		t.Fatal(err)
	}
	for range 30 {
		w.Frame(time.Second / 60)
	}
	if p.editor.Overrides().Len() != 5 {
		t.Fatalf("the saved values did not reach the editor: %v", p.editor.Overrides().Keys())
	}
	listed := p.editor.Listed()
	slices.Sort(listed)
	if want := p.editor.Overrides().Keys(); !slices.Equal(listed, want) {
		t.Fatalf("the changed values listed are %v, want %v", listed, want)
	}
}
