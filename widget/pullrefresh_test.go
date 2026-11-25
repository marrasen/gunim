package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

type refreshAsked struct{ N int }

// pullStage mounts a long list that refreshes as it is pulled, in a phone's window.
func pullStage(t *testing.T) (*PullRefresh, *Scroll, *gunim.Window, *grab, func(int)) {
	t.Helper()
	list := NewScroll(&pullContent{size: geom.Sz(390, 3000)})
	r := NewPullRefresh(list)
	r.OnRefresh = func(n int, _ *gunim.UI) gunim.Intent { return refreshAsked{n} }
	w, g, run := phoneStage(t, geom.Sz(390, 800), r)
	return r, list, w, g, run
}

// pullContent is content of a size that takes no input.
type pullContent struct{ size geom.Size }

func (b *pullContent) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(b.size)
}

func (b *pullContent) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

// pullDown drags a finger down the list by by, slowly, and lets go.
func pullDown(w *gunim.Window, run func(int), by float32) {
	fingerDrag(w, run, geom.Pt(200, 200), geom.Pt(200, 200+by), 12, 600*time.Millisecond, 150*time.Millisecond)
	run(1)
}

// A pull far enough down refreshes: the disc comes down with the finger on every frame, rests spinning while the
// program works, and slides away once the program says it is done.
func TestAPullFarEnoughRefreshes(t *testing.T) {
	r, _, w, g, run := pullStage(t)
	t0 := time.Now()
	w.Input(input.PointerMove{Pos: geom.Pt(200, 200), Touch: true, Time: t0})
	w.Input(input.PointerDown{Pos: geom.Pt(200, 200), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: t0})
	run(1)
	last := r.pull.Value()
	for i := 1; i <= 12; i++ {
		w.Input(input.PointerMove{Pos: geom.Pt(200, 200+float32(i)*25), Touch: true, Time: t0.Add(time.Duration(i) * 50 * time.Millisecond)})
		run(1)
		v := r.pull.Value()
		if v < last || v-last > 25 {
			t.Fatalf("move %d: the disc went from %v to %v", i, last, v)
		}
		last = v
	}
	if last < PullReach.Get(g.ui.Theme()) || r.Refreshing() {
		t.Fatalf("pulled 300 the disc stands %v down, refreshing %v", last, r.Refreshing())
	}
	end := t0.Add(800 * time.Millisecond)
	w.Input(input.PointerUp{Pos: geom.Pt(200, 500), Button: input.ButtonPrimary, Touch: true, Time: end})
	w.Input(input.PointerLeave{Time: end})
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (refreshAsked{1}) || r.Current() != 1 {
		t.Fatalf("let go past the reach, the pull sent %v, refresh %d under way", got, r.Current())
	}
	run(90)
	if v := r.pull.Value(); v < 63 || v > 65 {
		t.Fatalf("while refreshing the disc rests %v down, want 64", v)
	}
	if !r.Step(time.Second / 60) {
		t.Fatal("the spinner asks for no frames while it turns")
	}
	r.Done(1, g.ui)
	run(120)
	if r.Refreshing() || r.pull.Value() > 0.5 {
		t.Fatalf("done, the disc stands %v down, refreshing %v", r.pull.Value(), r.Refreshing())
	}
	if r.Step(time.Second / 60) {
		t.Fatal("at rest the pull asks for frames")
	}
}

// A short pull springs back and refreshes nothing; a pull on a list scrolled down scrolls it.
func TestAShortPullSpringsBack(t *testing.T) {
	r, list, w, _, run := pullStage(t)
	pullDown(w, run, 60)
	run(90)
	if got := sent(w); len(got) != 0 || r.Refreshing() || r.pull.Value() > 0.5 {
		t.Fatalf("a short pull sent %v, the disc %v down", got, r.pull.Value())
	}
	fingerDrag(w, run, geom.Pt(200, 600), geom.Pt(200, 200), 12, 600*time.Millisecond, 150*time.Millisecond)
	run(60)
	scrolled := list.Offset()
	pullDown(w, run, 200)
	run(60)
	if got := sent(w); len(got) != 0 || list.Offset() >= scrolled || r.pull.Value() > 0.5 {
		t.Fatalf("a drag down on a list scrolled %v down sent %v, left it at %v and the disc %v down", scrolled, got,
			list.Offset(), r.pull.Value())
	}
}

// A pull while a refresh is under way starts no other, and the answer to a refresh gone ends no later one.
func TestTwoRefreshesKeepApart(t *testing.T) {
	r, _, w, g, run := pullStage(t)
	pullDown(w, run, 300)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("the first pull sent %v", got)
	}
	pullDown(w, run, 300)
	w.Input(input.KeyPress{Key: input.KeyF5})
	run(1)
	if got := sent(w); len(got) != 0 || r.Current() != 1 {
		t.Fatalf("a pull and F5 during a refresh sent %v, refresh %d under way", got, r.Current())
	}
	r.Done(1, g.ui)
	run(60)
	pullDown(w, run, 300)
	if got := sent(w); len(got) != 1 || got[0] != (refreshAsked{2}) {
		t.Fatalf("the pull after the first refresh sent %v", got)
	}
	r.Done(1, g.ui)
	r.Fail(1, "late", g.ui)
	if r.Current() != 2 {
		t.Fatal("an answer to the first refresh ended the second")
	}
}

// A failed refresh shows its warning for a moment, then the disc slides away; one never answered fails once
// Timeout has passed.
func TestARefreshThatFailsOrNeverEnds(t *testing.T) {
	r, _, w, g, run := pullStage(t)
	r.Timeout = time.Second
	pullDown(w, run, 300)
	r.Fail(1, "No network", g.ui)
	run(30)
	if !r.failed || r.pull.Value() < 60 {
		t.Fatalf("failed, the disc stands %v down, showing the warning %v", r.pull.Value(), r.failed)
	}
	run(150)
	if r.failed || r.pull.Value() > 0.5 {
		t.Fatalf("after the warning the disc stands %v down", r.pull.Value())
	}
	sent(w)
	pullDown(w, run, 300)
	run(90)
	if r.Current() != 0 || !r.failed {
		t.Fatalf("a second past the timeout, refresh %d is still under way", r.Current())
	}
}

// The mouse pulls from a part of the list that takes no press, F5 and Ctrl+R refresh, and a screen reader has a
// Refresh action and hears that a refresh is under way.
func TestTheMouseKeysAndAScreenReaderRefresh(t *testing.T) {
	r, _, w, g, run := pullStage(t)
	drag(w, 100, 400, 300*time.Millisecond)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (refreshAsked{1}) {
		t.Fatalf("a drag of the mouse 300 down sent %v", got)
	}
	r.Done(1, g.ui)
	run(60)
	click(w, 100, 100)
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a click sent %v", got)
	}
	w.Input(input.KeyPress{Key: input.KeyR, Mods: input.ModControl})
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (refreshAsked{2}) {
		t.Fatalf("Ctrl+R sent %v", got)
	}
	if info := r.Access(); info.Description != "Refreshing" || len(info.Actions) != 0 {
		t.Fatalf("refreshing, the list reads %q with actions %v", info.Description, info.Actions)
	}
	r.Done(2, g.ui)
	if info := r.Access(); len(info.Actions) != 1 || info.Actions[0] != "Refresh" {
		t.Fatalf("at rest, the list's actions are %v", info.Actions)
	}
	if !r.AccessAct(access.Request{Part: -1, Action: "Refresh"}, g.ui) {
		t.Fatal("the screen reader's Refresh was refused")
	}
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (refreshAsked{3}) {
		t.Fatalf("the screen reader's Refresh sent %v", got)
	}
}

// With less motion the spinner stands still and asks for no frames, and the disc is a few quads, all inside the
// box.
func TestTheSpinnerIsCheapAndStillUnderLessMotion(t *testing.T) {
	r := NewPullRefresh(NewScroll(newSpot(10, 10)))
	for _, still := range []bool{false, true} {
		th := theme.NewLive(Dark())
		if still {
			th = theme.NewLive(Dark().With(theme.Set(theme.Scale, 0)))
		}
		r.th = th
		r.current = 1
		r.pull.Jump(64)
		var p paint.Painter
		box := geom.Sz(390, 600)
		r.paintDisc(&p, th, box, time.Now())
		if n := len(p.Ops()); n > 8 {
			t.Fatalf("the disc takes %d ops", n)
		}
		if bad := spills(p.Ops(), box, 0); len(bad) > 0 {
			t.Fatalf("the disc draws outside the box: %v", bad)
		}
		if r.Step(time.Second/60) == still {
			t.Fatalf("less motion %v: the spinner asks for frames %v", still, !still)
		}
	}
}
