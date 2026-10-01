package widget

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// A field that shares a name with a method of anim.Group hides it, and
// the engine then never steps the widget. These fail to compile if one
// does.
var (
	_ gunim.Animator = (*Button)(nil)
	_ gunim.Animator = (*Checkbox)(nil)
	_ gunim.Animator = (*Switch)(nil)
	_ gunim.Animator = (*Slider)(nil)
	_ gunim.Animator = (*Tabs)(nil)
	_ gunim.Animator = (*Menu)(nil)
	_ gunim.Animator = (*Dropdown)(nil)
	_ gunim.Animator = (*Image)(nil)
	_ gunim.Animator = (*Scroll)(nil)
	_ gunim.Animator = (*VirtualList)(nil)
	_ gunim.Animator = (*Hero)(nil)
	_ gunim.Animator = (*CodeEditor)(nil)
)

type (
	flipped struct{ On bool }
	slid    struct{ V float32 }
	tabbed  struct{ I int }
)

// sent drains the window's intents.
func sent(w *gunim.Window) []gunim.Intent {
	var out []gunim.Intent
	for {
		select {
		case e := <-w.Client().Intents():
			out = append(out, e.Intent)
		default:
			return out
		}
	}
}

func click(w *gunim.Window, x, y float32) {
	w.Input(input.PointerDown{Pos: geom.Pt(x, y), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(x, y), Time: time.Now()})
}

func TestCheckboxFlipsOnClickAndSpace(t *testing.T) {
	c := NewCheckbox("Tick")
	c.OnChange = func(on bool) gunim.Intent { return flipped{on} }
	w, run := stage(t, &frame{child: c, size: geom.Sz(200, 28)})
	click(w, 10, 14)
	run(1)
	if !c.On {
		t.Fatal("a click left the checkbox off")
	}
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(60)
	if c.On || c.lit.Value() != 0 {
		t.Fatalf("Space left the checkbox on at %v", c.lit.Value())
	}
	if got := sent(w); len(got) != 2 || got[0] != (flipped{true}) || got[1] != (flipped{false}) {
		t.Fatalf("intents %v, want on then off", got)
	}
}

func TestSwitchStartsWhereOnSays(t *testing.T) {
	s := NewSwitch("Power")
	s.On = true
	_, run := stage(t, &frame{child: s, size: geom.Sz(200, 28)})
	run(1)
	if v := s.lit.Value(); v != 1 {
		t.Fatalf("a switch made on starts at %v, want 1", v)
	}
}

func TestSliderFollowsPointerAndKeys(t *testing.T) {
	s := NewSlider(0, 100)
	s.Snap = 1
	s.OnChange = func(v float32) gunim.Intent { return slid{v} }
	w, run := stage(t, &frame{child: s, size: geom.Sz(218, 28)})
	// The track runs from 9 to 209, inside the knob's half at each end.
	w.Input(input.PointerDown{Pos: geom.Pt(109, 14), Clicks: 1})
	run(1)
	if s.Value() != 50 {
		t.Fatalf("a press mid-track set %v, want 50", s.Value())
	}
	w.Input(input.PointerMove{Pos: geom.Pt(159, 14)})
	w.Input(input.PointerUp{Pos: geom.Pt(159, 14)})
	run(60)
	if s.Value() != 75 || s.at.Value() != 0.75 {
		t.Fatalf("a drag to three quarters left %v with the knob at %v", s.Value(), s.at.Value())
	}
	for _, k := range []input.Key{input.KeyRight, input.KeyRight, input.KeyEnd, input.KeyPageDown} {
		w.Input(input.KeyPress{Key: k})
	}
	run(1)
	if s.Value() != 90 {
		t.Fatalf("keys left %v, want 90", s.Value())
	}
	got := sent(w)
	if last := got[len(got)-1]; last != (slid{90}) {
		t.Fatalf("last intent %v, want slid{90}", last)
	}
}

func TestTabsShowOnePageAtATime(t *testing.T) {
	a, b := &recorder{}, &recorder{}
	tabs := NewTabs([]string{"One", "Two"}, a, b)
	tabs.OnChange = func(i int) gunim.Intent { return tabbed{i} }
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	click(w, 150, 100)
	run(1)
	if len(a.events) == 0 || len(b.events) != 0 {
		t.Fatal("a click on the page reached the wrong one")
	}
	// The second title starts after the first, "One" and its padding.
	click(w, tabs.spans[1][0]+5, 10)
	run(60)
	if tabs.Selected() != 1 {
		t.Fatalf("a click on the second title left tab %d", tabs.Selected())
	}
	a.events, b.events = nil, nil
	click(w, 150, 100)
	run(1)
	if len(b.events) == 0 || len(a.events) != 0 {
		t.Fatal("after the switch, a click on the page reached the old one")
	}
	// The click on the page took focus from the titles; a click on the
	// chosen title gives it back.
	click(w, tabs.spans[1][0]+5, 10)
	w.Input(input.KeyPress{Key: input.KeyLeft})
	run(1)
	if tabs.Selected() != 0 {
		t.Fatal("Left on the focused titles did not go back a tab")
	}
	if got := sent(w); len(got) != 2 || got[0] != (tabbed{1}) || got[1] != (tabbed{0}) {
		t.Fatalf("intents %v, want tab 1 then 0", got)
	}
}

// recorder fills the space it is given and takes every pointer press.
type recorder struct{ events []input.Event }

func (r *recorder) Handle(e input.Event, _ *gunim.UI) bool {
	if _, ok := e.(input.PointerDown); ok {
		r.events = append(r.events, e)
		return true
	}
	return false
}

func (r *recorder) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (r *recorder) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

func TestTabsTakePagesMountedUnderThemAndSkipDisabledOnes(t *testing.T) {
	tabs := NewTabs([]string{"One", "Two", "Three"})
	tabs.Disabled = []bool{false, true}
	w := gunimtest.New(t, geom.Sz(600, 400), nil)
	gunim.RegisterView(w, "tabs", func(struct{}) *Tabs { return tabs }, nil)
	pages := []*spot{newSpot(10, 10), newSpot(10, 10), newSpot(10, 10)}
	for i, p := range pages {
		gunim.RegisterView(w, fmt.Sprint("page", i), func(struct{}) *spot { return p }, nil)
	}
	c := w.Client()
	if err := c.Mount(gunim.Root, "tabs", "tabs", nil); err != nil {
		t.Fatal(err)
	}
	for i := range pages {
		if err := c.Mount("tabs", gunim.ID(fmt.Sprint("page", i)), fmt.Sprint("page", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	if tabs.count != 3 {
		t.Fatalf("the tabs hold %d pages, want the 3 mounted", tabs.count)
	}
	two := tabs.spans[1]
	click(w, (two[0]+two[1])/2, 10)
	run(1)
	if tabs.Selected() != 0 {
		t.Fatalf("a click on a disabled tab chose tab %d", tabs.Selected())
	}
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	if tabs.Selected() != 2 {
		t.Fatalf("Right from the first tab chose tab %d, want 2, past the disabled one", tabs.Selected())
	}
}

func TestTabsCanBeSelectedBeforeTheirPagesArrive(t *testing.T) {
	tabs := NewTabs([]string{"One", "Two", "Three"})
	w := gunimtest.New(t, geom.Sz(600, 400), nil)
	gunim.RegisterView(w, "tabs", func(struct{}) *Tabs { return tabs }, func(tb *Tabs, _ struct{}, u *gunim.UI) { tb.Select(2, u) })
	page := newSpot(10, 10)
	gunim.RegisterView(w, "page", func(struct{}) *spot { return page }, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "tabs", "tabs", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	if tabs.Selected() != 2 {
		t.Fatalf("selecting a tab whose page has not arrived chose %d", tabs.Selected())
	}
}

func TestFaderRunsUpTheHeight(t *testing.T) {
	s := NewFader(0, 100)
	s.Snap = 1
	s.OnChange = func(v float32) gunim.Intent { return slid{v} }
	w, run := stage(t, &frame{child: s, size: geom.Sz(28, 218)})
	// The track runs from 9 at the top to 209 at the bottom, inside the
	// knob's half at each end, and the top of it is the maximum.
	w.Input(input.PointerDown{Pos: geom.Pt(14, 109), Clicks: 1})
	run(1)
	if s.Value() != 50 {
		t.Fatalf("a press mid-track set %v, want 50", s.Value())
	}
	w.Input(input.PointerMove{Pos: geom.Pt(14, 9)})
	w.Input(input.PointerUp{Pos: geom.Pt(14, 9)})
	run(60)
	if s.Value() != 100 || s.at.Value() != 1 {
		t.Fatalf("a drag to the top left %v with the knob at %v", s.Value(), s.at.Value())
	}
	// Up still raises and down still lowers, as on a horizontal one.
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if s.Value() != 99 {
		t.Fatalf("pressing down left %v, want 99", s.Value())
	}
}

func TestFaderFillsTheHeightItIsGiven(t *testing.T) {
	s := NewFader(0, 1)
	w, run := stage(t, &frame{child: s, size: geom.Sz(40, 300)})
	run(1)
	_ = w
	if s.size.H != 300 {
		t.Errorf("a fader in 300 of height took %v", s.size.H)
	}
	if s.size.W > 40 {
		t.Errorf("a fader took %v of width, more than it was given", s.size.W)
	}
}

func TestSliderTellsTheWindowAsItMoves(t *testing.T) {
	s := NewSlider(0, 100)
	s.Snap = 1
	var seen []float32
	s.OnMove(func(v float32, _ *gunim.UI) { seen = append(seen, v) })
	w, run := stage(t, &frame{child: s, size: geom.Sz(218, 28)})
	w.Input(input.PointerDown{Pos: geom.Pt(109, 14), Clicks: 1})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	if len(seen) != 2 || seen[0] != 50 || seen[1] != 51 {
		t.Errorf("OnMove saw %v, want 50 then 51", seen)
	}
}
