package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

type segmentChosen struct{ I int }

type selectTwo struct{}

func TestASegmentedControlChoosesByClickAndArrows(t *testing.T) {
	s := NewSegmented("One", "Two", "Three")
	s.OnChange = func(i int) gunim.Intent { return segmentChosen{i} }
	w, run := stage(t, &frame{child: Row(s), size: geom.Sz(600, 40)})
	run(1)
	click(w, s.width*2.5, 14)
	run(1)
	if got := sent(w); s.Selected() != 2 || len(got) != 1 || got[0] != (segmentChosen{2}) {
		t.Fatalf("a click on Three segmentChosen %d and sent %v", s.Selected(), got)
	}
	run(60)
	if at := s.pill.Value(); at < 1.99 || at > 2.01 {
		t.Fatalf("the pill rests at %v, want on Three", at)
	}
	press := func(k input.Key) {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	press(input.KeyLeft)
	press(input.KeyLeft)
	press(input.KeyLeft)
	if got := sent(w); s.Selected() != 0 || len(got) != 2 || got[1] != (segmentChosen{0}) {
		t.Fatalf("Left three times from Three segmentChosen %d and sent %v, want One, and nothing past the end", s.Selected(), got)
	}
	press(input.KeyEnd)
	if s.Selected() != 2 {
		t.Fatalf("End segmentChosen %d, want the last", s.Selected())
	}
	sent(w)

	// Set from the view, it sends nothing.
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ selectTwo, u *gunim.UI) { s.SetSelected(1, u) })
	if err := w.Client().Patch("stage", selectTwo{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	if got := sent(w); s.Selected() != 1 || len(got) != 0 {
		t.Fatalf("SetSelected(1) segmentChosen %d and sent %v", s.Selected(), got)
	}
	if at := s.pill.Value(); at <= 1 || at >= 2 {
		t.Fatalf("a frame after SetSelected(1) the pill is at %v, want on its way from Three", at)
	}
}

func TestTabReachesASegmentedControl(t *testing.T) {
	b, s := NewButton("B"), NewSegmented()
	s.Icons = []*icon.Icon{icon.List, icon.LayoutGrid}
	w, run := stage(t, &frame{child: Column(b, s), size: geom.Sz(400, 200)})
	focused := focusProbe(t, w, run)
	tab(w, run, 0)
	tab(w, run, 0)
	if f := focused(); f != s {
		t.Fatalf("two Tabs put the keyboard on %T, want the segmented control", f)
	}
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(30)
	if s.Selected() != 1 || s.ring.Value() < 0.99 {
		t.Fatalf("Right segmentChosen %d with the ring %v grown; want the second, the ring all the way", s.Selected(), s.ring.Value())
	}
	info := s.Access()
	if len(info.Parts) != 2 || info.Parts[0].Name != "list" || !info.Parts[1].State.Has(access.StateChecked) {
		t.Fatalf("the control reads as %+v, want two options named by their icons, the second checked", info.Parts)
	}
}
