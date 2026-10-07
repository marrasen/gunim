package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type chooseTab struct{ I int }

// A field on a page that hides keeps the keyboard no longer: it goes to the first field of the page shown.
func TestTabsTakeTheKeyboardOffAPageAsItHides(t *testing.T) {
	a, b := NewTextField(), NewTextField()
	tabs := NewTabs([]string{"One", "Two"}, Column(a), Column(b))
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, c chooseTab, u *gunim.UI) { tabs.Select(c.I, u) })
	focused := focusProbe(t, w, run)
	click(w, 20, tabs.head+10)
	run(1)
	if f := focused(); f != a {
		t.Fatalf("a click on the first page's field put the keyboard on %T", f)
	}
	u := stageUI(t, w, run)
	if err := w.Client().Patch("stage", chooseTab{1}); err != nil {
		t.Fatal(err)
	}
	run(1)
	// The new page takes the keyboard on the frame that first lays it out, before it paints.
	if f := u.Focused(); f != b {
		t.Fatalf("a frame after the second page was chosen, the keyboard is on %T %p, want the second page's field", f, f)
	}
	w.Input(input.TextInput{Text: "x"})
	run(1)
	if a.Text() != "" || b.Text() != "x" {
		t.Fatalf("typing went to %q and %q, want the shown page's field", a.Text(), b.Text())
	}
}

// A checkbox, a switch or a slider disabled with the keyboard on it lets its ring go as the keyboard moves on.
func TestAControlDisabledWithTheKeyboardLetsItsRingGo(t *testing.T) {
	box, sw, sl := NewCheckbox("Box"), NewSwitch("Switch"), NewSlider(0, 1)
	after := NewButton("After")
	w, run := stage(t, &frame{child: Column(box, sw, sl, after), size: geom.Sz(300, 300)})
	rings := []*struct {
		name string
		ring func() float32
		off  func()
	}{
		{"checkbox", box.ring.Value, func() { box.Disabled = true }},
		{"switch", sw.ring.Value, func() { sw.Disabled = true }},
		{"slider", sl.ring.Value, func() { sl.Disabled = true }},
	}
	for i, c := range rings {
		tab(w, run, 0)
		run(30)
		if r := c.ring(); r < 0.9 {
			t.Fatalf("Tab %d left the %s's ring at %v", i+1, c.name, r)
		}
		c.off()
	}
	tab(w, run, 0)
	run(60)
	for _, c := range rings {
		if r := c.ring(); r > 0.01 {
			t.Fatalf("the %s, disabled and passed by, keeps its ring at %v", c.name, r)
		}
	}
}

// The reset mark acts on a click let go on it, and only on a slider that is enabled.
func TestASliderRowResetsOnlyAnEnabledSliderOnARelease(t *testing.T) {
	s := NewSlider(-100, 100)
	s.Rest, s.HasRest = 0, true
	s.Set(80)
	row := NewSliderRow("Contrast", s)
	w, run := stage(t, &frame{child: row, size: geom.Sz(300, 28)})
	run(30)
	c := row.reset.Center()
	w.Input(input.PointerDown{Pos: c, Clicks: 1})
	run(1)
	if s.Value() != 80 {
		t.Fatalf("a press alone on the reset mark left %v, want 80 until the release", s.Value())
	}
	w.Input(input.PointerUp{Pos: geom.Pt(20, 14)})
	run(1)
	if s.Value() != 80 {
		t.Fatalf("a press let go off the reset mark left %v, want 80", s.Value())
	}
	s.Disabled = true
	run(1)
	click(w, c.X, c.Y)
	run(1)
	if s.Value() != 80 {
		t.Fatalf("a click on a disabled slider's reset mark left %v, want 80", s.Value())
	}
}
