package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/widget"
)

// The page builds from saved values and runs, and the cursor moves on
// along its line.
func TestThePageShowsTheSavedValuesAndTheCursorMoves(t *testing.T) {
	var p *page
	w := gunimtest.New(t, geom.Sz(1100, 640), widget.NewSurface())
	gunim.RegisterView(w, "page", func(s Page) *page { p = buildPage(s); return p }, nil)
	saved := []byte(`{"motion.caret": {"response": 0, "damping": 1}}`)
	if err := w.Client().Mount(gunim.Root, "page", "page", Page{Saved: saved}); err != nil {
		t.Fatal(err)
	}
	for range 60 {
		w.Frame(time.Second / 60)
	}
	if !p.editor.Overrides().Has(widget.Caret.Key()) {
		t.Fatal("the saved cursor did not reach the editor")
	}
	if ty := p.typist; ty.col == 2 && ty.row == 0 {
		t.Fatal("a second on, the cursor has not moved")
	}
}
