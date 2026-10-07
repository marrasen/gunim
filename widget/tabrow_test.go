package widget

import (
	"strconv"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

type cutTitles struct{ N int }

// A row of tabs scrolled to its end, cut to a few titles, shows them from its start at once, the chosen one among
// them with its line under it, on every frame.
func TestTabsCutToAFewTitlesJumpBackAndKeepTheirLine(t *testing.T) {
	titles := make([]string, 20)
	pages := make([]gunim.Node, 20)
	for i := range titles {
		titles[i] = "Title " + strconv.Itoa(i)
		pages[i] = &recorder{}
	}
	tabs := NewTabs(titles, pages...)
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, c cutTitles, u *gunim.UI) {
		tabs.Titles = tabs.Titles[:c.N]
		u.Invalidate()
	})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, c chooseTab, u *gunim.UI) { tabs.Select(c.I, u) })
	if err := w.Client().Patch("stage", chooseTab{19}); err != nil {
		t.Fatal(err)
	}
	run(90)
	if tabs.off.Value() <= 0 {
		t.Fatal("choosing the last tab left the row unscrolled")
	}
	if err := w.Client().Patch("stage", cutTitles{3}); err != nil {
		t.Fatal(err)
	}
	for frame := range 90 {
		run(1)
		if off := tabs.off.Value(); off != 0 {
			t.Fatalf("frame %d: the row of three titles, all in view, is scrolled to %v", frame, off)
		}
		if s := tabs.Selected(); s != 2 {
			t.Fatalf("frame %d: tab %d is chosen of three", frame, s)
		}
		sp := tabs.spans[2]
		if line := tabs.line.Value(); line.X < sp[0] || line.Y > sp[1] {
			t.Fatalf("frame %d: the line runs %v to %v, want under the last title, %v", frame, line.X, line.Y, sp)
		}
	}
}
