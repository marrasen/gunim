package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// The application half, on its own: each lesson's Handle changes the
// state, and State hands the lesson what it shows.
func TestTheLessonsHandleTheirIntents(t *testing.T) {
	a := newApp(0)
	steps := []struct {
		in   gunim.Intent
		want func() bool
	}{
		{Clicked{}, func() bool { return a.clicks == 1 }},
		{Clicked{}, func() bool { return a.clicks == 2 }},
		{Reset{}, func() bool { return a.clicks == 0 }},
		{Added{Text: "Four"}, func() bool { return len(a.items) == 4 && a.items[3].Text == "Four" }},
		{Added{Text: ""}, func() bool { return len(a.items) == 4 }},
		{Removed{ID: a.items[0].ID}, func() bool { return len(a.items) == 3 && a.items[0].Text == "Add an item" }},
		{Reversed{}, func() bool { return a.items[0].Text == "Four" }},
		{Emptied{}, func() bool { return a.trash == 0 }},
		{Restored{}, func() bool { return a.trash == 12 }},
	}
	for _, st := range steps {
		took := false
		for _, l := range lessons {
			if l.Handle != nil && l.Handle(a, gunim.Client{}, st.in) {
				took = true
				break
			}
		}
		if !took {
			t.Fatalf("no lesson took %T", st.in)
		}
		if !st.want() {
			t.Fatalf("after %#v the state is %+v", st.in, *a)
		}
	}
	if s := lesson3.State(a).(Todo); len(s.Items) != 3 || &s.Items[0] == &a.items[0] {
		t.Fatalf("lesson 3's state is %+v, want a clone of the three items", s)
	}
}

// stage opens the tutorial offscreen, with the application half run by
// hand: serve's loop is a select, so the test drives its steps itself.
func stage(t *testing.T) (w *gunim.Window, c gunim.Client, a *app, run func(int)) {
	t.Helper()
	w = gunimtest.New(t, geom.Sz(1120, 720), widget.NewSurface())
	registerViews(w)
	c = w.Client()
	a = newApp(0)
	if err := c.Mount(gunim.Root, "shell", "shell", Shell{Lesson: a.lesson}); err != nil {
		t.Fatal(err)
	}
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, c, a, run
}

// intents returns what the window has sent so far.
func intents(w *gunim.Window) []gunim.Intent {
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

// Every lesson opens, animates in over the one before, and takes its
// state, with no command failing along the way.
func TestEveryLessonOpensInTurn(t *testing.T) {
	w, c, a, run := stage(t)
	for i := range lessons {
		a.lesson = i
		if err := c.Update("shell", Shell{Lesson: i}); err != nil {
			t.Fatal(err)
		}
		if err := a.open(c); err != nil {
			t.Fatal(err)
		}
		// The page before is still leaving as the new one arrives.
		run(3)
		if err := c.Update("page", lessons[i].State(a)); err != nil {
			t.Fatal(err)
		}
		run(60)
		for _, in := range intents(w) {
			if f, ok := in.(gunim.CommandFailed); ok {
				t.Fatalf("opening lesson %d, %s failed: %s", i+1, f.Command, f.Reason)
			}
		}
	}
}

// click presses and releases the primary button at p.
func click(w *gunim.Window, p geom.Point) {
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
}

// A click on a lesson in the list sends Chose with that lesson.
func TestTheListChoosesALesson(t *testing.T) {
	w, _, _, run := stage(t)
	// The buttons sit under the heading in the sidebar's column: the
	// margin, the heading, a gap, and a button each 36 high with a gap
	// of 8 between.
	click(w, geom.Pt(sidebarWidth/2, 16+30+8+36+8+36/2))
	run(2)
	var got []Chose
	for _, in := range intents(w) {
		if ch, ok := in.(Chose); ok {
			got = append(got, ch)
		}
	}
	if len(got) != 1 || got[0].Lesson != 1 {
		t.Fatalf("the click sent %+v, want Chose{Lesson: 1}", got)
	}
}

// Lesson 2's button sends Clicked, and the count shows what comes back.
func TestTheCounterRoundTrip(t *testing.T) {
	w, c, a, run := stage(t)
	a.lesson = 1
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run(60)
	page := pageOf[*counterPage](t, w, run, "lesson2", "page")
	if page == nil {
		t.Fatal("lesson 2's page is missing")
	}
	if page.count.Text != "Press it and see" {
		t.Fatalf("before a press the count reads %q", page.count.Text)
	}
	if err := c.Update("page", Counter{Clicks: 3}); err != nil {
		t.Fatal(err)
	}
	run(1)
	if page.count.Text != "Pressed 3 times" {
		t.Fatalf("with 3 clicks the count reads %q", page.count.Text)
	}
}

// Lesson 3's list grows and shrinks with the state, a frame at a time.
func TestTheListFollowsTheState(t *testing.T) {
	w, c, a, run := stage(t)
	a.lesson = 2
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run(60)
	page := pageOf[*todoPage](t, w, run, "lesson3", "page")
	if page == nil {
		t.Fatal("lesson 3's page is missing")
	}
	if n := page.list.Len(); n != 3 {
		t.Fatalf("the list has %d rows, want the 3 seeded", n)
	}
	a.add("Four")
	if err := c.Update("page", lesson3.State(a)); err != nil {
		t.Fatal(err)
	}
	run(1)
	if n := page.list.Len(); n != 4 {
		t.Fatalf("after an add the list has %d rows", n)
	}
	a.items = a.items[1:]
	if err := c.Update("page", lesson3.State(a)); err != nil {
		t.Fatal(err)
	}
	// The row leaving is still in the list while it collapses.
	for range 60 {
		run(1)
		if n := page.list.Len(); n < 3 || n > 4 {
			t.Fatalf("while a row leaves the list has %d rows", n)
		}
	}
	if n := page.list.Len(); n != 3 {
		t.Fatalf("once the row has left the list has %d rows", n)
	}
}

// Lesson 6's dialog mounts over the page, and closes itself on Escape,
// sending Kept.
func TestTheDialogMountsAndDismisses(t *testing.T) {
	w, c, a, run := stage(t)
	a.lesson = 5
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run(60)
	if !lesson6.Handle(a, c, EmptyAsked{}) {
		t.Fatal("lesson 6 left EmptyAsked alone")
	}
	run(30)
	if pageOf[*widget.Dialog](t, w, run, "confirm", "confirm") == nil {
		t.Fatal("the dialog is missing after EmptyAsked")
	}
	intents(w)
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(2)
	var kept bool
	for _, in := range intents(w) {
		if _, ok := in.(Kept); ok {
			kept = true
		}
	}
	if !kept {
		t.Fatal("Escape sent no Kept")
	}
	run(90)
	// Once the dialog has left, its ID is free and a patch to it fails.
	if err := c.Patch("confirm", probe{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	var failed bool
	for _, in := range intents(w) {
		if f, ok := in.(gunim.CommandFailed); ok && f.Key == "confirm" {
			failed = true
		}
	}
	if !failed {
		t.Fatal("the dialog is still in the tree after its exit")
	}
}

// probe is a patch that hands a test the view's node.
type probe struct{}

// pageOf returns the node of the view mounted as id, registered under
// name, or the zero N when it is gone. It reaches the node through a
// patch, which is how a test gets at a view's nodes: they belong to the
// window's goroutine, and the patch runs there.
func pageOf[N gunim.Node](t *testing.T, w *gunim.Window, run func(int), name string, id gunim.ID) N {
	t.Helper()
	var got N
	gunim.RegisterPatch(w, name, func(n N, _ probe, _ *gunim.UI) { got = n })
	if err := w.Client().Patch(string(id), probe{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return got
}

// Lesson 2's count bumps when it changes, and settles back to size.
func TestTheCountBumpsOnAChange(t *testing.T) {
	w, c, a, run := stage(t)
	a.lesson = 1
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run(60)
	page := pageOf[*counterPage](t, w, run, "lesson2", "page")
	if s := page.bump.scale.Value(); s != 1 {
		t.Fatalf("at rest the count is scaled %v", s)
	}
	if err := c.Update("page", Counter{Clicks: 1}); err != nil {
		t.Fatal(err)
	}
	run(1)
	if s := page.bump.scale.Value(); s <= 1 {
		t.Fatalf("a frame after the change the count is scaled %v, want over 1", s)
	}
	run(120)
	if s := page.bump.scale.Value(); s != 1 || page.bump.scale.Active() {
		t.Fatalf("two seconds on the count is scaled %v and still moving: %v", s, page.bump.scale.Active())
	}
}

// A click in lesson 4's field sends the ball there, starts a ripple
// from the click, and blends the ball to its next colour.
func TestAClickMovesTheBall(t *testing.T) {
	w, c, a, run := stage(t)
	a.lesson = 3
	if err := a.open(c); err != nil {
		t.Fatal(err)
	}
	run(60)
	page := pageOf[*page](t, w, run, "lesson4", "page")
	field := page.body.(*widget.Pad).Children()[0].(*widget.Flex).Children()[2].(*widget.Card).Children()[0].(*ballField)
	was := field.at.Value()
	// The click lands in the field's own space, so aim from where the
	// field sits in the window, relative to the ball in its middle.
	b, ok := boundsOf(t, w, run, "lesson4", "page", field)
	if !ok {
		t.Fatal("the field has no bounds")
	}
	at := geom.Pt(b.Min.X+was.X-100, b.Min.Y+was.Y+40)
	click(w, at)
	run(1)
	want := geom.Pt(was.X-100, was.Y+40)
	if field.ripple.Value() >= 1 || !near(field.from, want) {
		t.Fatalf("a frame after the click the ripple is at %v from %v, want from %v", field.ripple.Value(), field.from, want)
	}
	if field.tint.Target() != hueColors[1] || !field.at.Active() {
		t.Fatalf("the click left the ball heading for %v in %v", field.at.Target(), field.tint.Target())
	}
	run(120)
	if got := field.at.Value(); !near(got, want) {
		t.Fatalf("two seconds on the ball is at %v, want where the click was, %v", got, want)
	}
}

// near reports whether a and b are within a hundredth of a pixel, the
// slack a click's position picks up crossing the tree's transforms.
func near(a, b geom.Point) bool {
	return abs32(a.X-b.X) < 0.01 && abs32(a.Y-b.Y) < 0.01
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// boundsOf returns n's bounds in the window, through a patch on the
// view named name mounted as id.
func boundsOf(t *testing.T, w *gunim.Window, run func(int), name string, id gunim.ID, n gunim.Node) (geom.Rect, bool) {
	t.Helper()
	var r geom.Rect
	var ok bool
	gunim.RegisterPatch(w, name, func(_ *page, _ bounds, u *gunim.UI) { r, ok = u.Bounds(n) })
	if err := w.Client().Patch(string(id), bounds{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return r, ok
}

type bounds struct{}
