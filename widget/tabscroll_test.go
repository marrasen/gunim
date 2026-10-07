package widget

import (
	"fmt"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// newWideTabs stages 20 tabs in a row 300 wide, far narrower than their titles.
func newWideTabs(t *testing.T) (tabs *Tabs, run func(int), in func(any)) {
	t.Helper()
	titles := make([]string, 20)
	pages := make([]gunim.Node, 20)
	for i := range titles {
		titles[i] = fmt.Sprintf("Page %d", i)
		pages[i] = &recorder{}
	}
	tabs = NewTabs(titles, pages...)
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	if tabs.wide <= tabs.room {
		t.Fatalf("the titles are %v wide in a row of %v; the test wants them wider", tabs.wide, tabs.room)
	}
	return tabs, run, w.Input
}

// tabShown fails unless tab i lies wholly inside the row, scrolled as it is now.
func tabShown(t *testing.T, tabs *Tabs, i int, when string) {
	t.Helper()
	sp, off := tabs.spans[i], tabs.off.Value()
	if sp[0]-off < -0.5 || sp[1]-off > tabs.room+0.5 {
		t.Fatalf("%s: tab %d spans %v..%v in a row of %v", when, i, sp[0]-off, sp[1]-off, tabs.room)
	}
}

func TestTheKeysBringAChosenTabIntoView(t *testing.T) {
	tabs, run, in := newWideTabs(t)
	in(input.PointerDown{Pos: geom.Pt(20, 10), Clicks: 1})
	in(input.PointerUp{Pos: geom.Pt(20, 10)})
	in(input.KeyPress{Key: input.KeyEnd})
	end := tabs.wide - tabs.room
	last := float32(0)
	for f := range 40 {
		run(1)
		at := tabs.off.Value()
		if at < last-overshoot(end) || at > end+overshoot(end) {
			t.Fatalf("frame %d: the row at %v after %v, want it moving to %v", f, at, last, end)
		}
		last = at
	}
	tabShown(t, tabs, 19, "after End")
	in(input.KeyPress{Key: input.KeyHome})
	run(40)
	tabShown(t, tabs, 0, "after Home")
}

func TestTheWheelScrollsTheTitlesAndAClickChoosesTheOneUnderIt(t *testing.T) {
	tabs, run, in := newWideTabs(t)
	in(input.Scroll{Pos: geom.Pt(150, 10), Delta: geom.Pt(0, -250)})
	run(40)
	if at := tabs.off.Value(); at < 240 || at > 260 {
		t.Fatalf("the wheel moved the row to %v, want 250", at)
	}
	// The row stays where the wheel left it, with the first tab still chosen out of view.
	run(10)
	if at := tabs.off.Value(); at < 240 {
		t.Fatalf("the row went back to %v", at)
	}
	x := float32(150)
	want := -1
	for i, sp := range tabs.spans {
		if x+tabs.off.Value() >= sp[0] && x+tabs.off.Value() < sp[1] {
			want = i
		}
	}
	in(input.PointerDown{Pos: geom.Pt(x, 10), Clicks: 1})
	in(input.PointerUp{Pos: geom.Pt(x, 10)})
	run(1)
	if want < 3 || tabs.Selected() != want {
		t.Fatalf("a click on tab %d chose %d", want, tabs.Selected())
	}
	// The wheel past the start passes on.
	in(input.Scroll{Pos: geom.Pt(150, 10), Delta: geom.Pt(0, 5000)})
	run(40)
	if at := tabs.off.Value(); at > 0.5 {
		t.Fatalf("a long wheel back left the row at %v", at)
	}
}

func TestTitlesThatFitDoNotScroll(t *testing.T) {
	tabs := NewTabs([]string{"One", "Two"}, &recorder{}, &recorder{})
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	w.Input(input.Scroll{Pos: geom.Pt(50, 10), Delta: geom.Pt(0, -200)})
	run(20)
	if at := tabs.off.Value(); at != 0 {
		t.Fatalf("titles that fit scrolled to %v", at)
	}
}

func TestTheTitlesFadeWhereMoreLiePastTheEdge(t *testing.T) {
	tabs, run, in := newWideTabs(t)
	full := ScrollFade.Default()
	box := geom.Sz(tabs.room, tabs.head)
	fade, ok := fadeIn(painted(tabs.bar, box))
	if !ok || fade.Left != 0 || fade.Right != full {
		t.Fatalf("at the start the titles fade by %v (%v), want only at the right, by %v", fade, ok, full)
	}
	in(input.Scroll{Pos: geom.Pt(150, 10), Delta: geom.Pt(0, -100)})
	run(40)
	fade, _ = fadeIn(painted(tabs.bar, box))
	if fade.Left != full || fade.Right != full {
		t.Fatalf("in the middle the titles fade by %v, want both sides by %v", fade, full)
	}
}
