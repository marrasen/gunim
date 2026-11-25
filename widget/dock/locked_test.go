package dock

import (
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// lockable returns a dock with all the user can do to it turned on, and a layout of
// two panes side by side, a with a second tab c.
func lockable() (*Dock, Layout) {
	d, _ := hosting()
	d.Plus, d.Keys, d.Maximize = true, true, true
	d.Face = func(n *Node) Face { return Face{Title: n.ID + " on a machine", Closable: true} }
	return d, splitLayout()
}

// setLocked locks d or unlocks it, as an application does, and runs a frame.
func setLocked(t *testing.T, w *gunim.Window, run func(int), d *Dock, on bool) {
	t.Helper()
	onUIDock(t, w, run, func(u *gunim.UI) {
		d.Locked = on
		u.Invalidate()
	})
}

// TestALockedDockOnlyPicks: whatever the user does with the pointer, the layout stays
// as it was and nothing is asked for but a tab to show.
func TestALockedDockOnlyPicks(t *testing.T) {
	d, l := lockable()
	d.Locked = true
	menus, drops := 0, 0
	d.OnPlusMenu = func(*Bar, geom.Rect, *gunim.UI) gunim.Intent { menus++; return nil }
	d.Takes = func(any) bool { return true }
	d.OnDrop = func(string, Dropped, *gunim.UI) gunim.Intent { drops++; return nil }
	w, run, sent := stage(t, d, l)
	run(30)
	onlyShows := func(what string) {
		t.Helper()
		for _, in := range sent() {
			if _, ok := in.(Show); !ok {
				t.Errorf("%s asked for %#v", what, in)
			}
		}
		if got := d.Arrangement(); !reflect.DeepEqual(got.Windows[0].Root.Kids[1], l.Windows[0].Root.Kids[1]) ||
			got.Windows[0].Root.Share != l.Windows[0].Root.Share || !slices.Equal(got.Items(), l.Items()) {
			t.Errorf("%s left the layout %s", what, draw(got, 0))
		}
		if d.Maxed() != "" {
			t.Errorf("%s filled the window", what)
		}
	}
	a, b := tabAt(t, d, "a"), tabAt(t, d, "b")
	right := groupArea(t, d, l.GroupOf("b").ID)
	drag(w, run, a, b)
	onlyShows("a tab dragged to another bar")
	drag(w, run, a, geom.Pt(right.Center().X, right.Max.Y-10))
	onlyShows("a tab dragged to a pane's edge")
	drag(w, run, a, tabAt(t, d, "c").Add(geom.Pt(30, 0)))
	onlyShows("a tab dragged along its bar")
	drag(w, run, a, geom.Pt(1200, 900))
	onlyShows("a tab dragged out of the window")
	for _, bar := range d.allBars() {
		if bar.Carried() != "" {
			t.Errorf("tab %s is carried", bar.Carried())
		}
	}
	click(w, run, a, input.ButtonMiddle)
	onlyShows("the middle button")
	for _, g := range d.groups {
		for i := range g.bar.tabs {
			click(w, run, g.top.Min.Add(crossIn(g.bar.boxes[i], TabPadding.Default()).Center()), input.ButtonPrimary)
			onlyShows("a click where the × would be")
		}
		at := geom.Pt(g.bar.end+15, g.top.Center().Y)
		click(w, run, g.top.Min.Add(at), input.ButtonPrimary)
		click(w, run, g.top.Min.Add(at), input.ButtonSecondary)
		onlyShows("a click where the + would be")
		if !g.bar.PlusRect().Empty() {
			t.Errorf("a locked bar has a + at %v", g.bar.PlusRect())
		}
	}
	w.Input(input.PointerDown{Pos: b, Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: b, Button: input.ButtonPrimary})
	run(2)
	onlyShows("a double click on a tab")
	dv, ok := d.DividerRect(l.Windows[0].Root.ID)
	if !ok {
		t.Fatal("no divider")
	}
	if got := d.Cursor(dv.Center()); got != input.CursorArrow {
		t.Errorf("over a divider that can't be dragged the pointer is %v", got)
	}
	drag(w, run, dv.Center(), geom.Pt(150, dv.Center().Y))
	onlyShows("a divider dragged")
	w.Input(input.PointerDown{Pos: dv.Center(), Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: dv.Center(), Button: input.ButtonPrimary})
	run(2)
	onlyShows("a divider double clicked")
	if now, _ := d.DividerRect(l.Windows[0].Root.ID); now != dv {
		t.Errorf("the divider went from %v to %v", dv, now)
	}
	if menus != 0 || drops != 0 {
		t.Errorf("%d menus opened, and %d drops landed", menus, drops)
	}
}

// TestNothingLandsInALockedDock: a tab of another window's, on a bar or a pane, or
// something else the application takes on a bar. Unlocked, each of them lands.
func TestNothingLandsInALockedDock(t *testing.T) {
	for _, locked := range []bool{false, true} {
		for what, data := range map[string]any{"a tab": Drag{ID: "b", Win: 1}, "something else": "a session"} {
			for _, onBar := range []bool{true, false} {
				if _, tab := data.(Drag); !tab && !onBar {
					// Only bars take it.
					continue
				}
				d, l := lockable()
				d.Locked = locked
				d.Takes = func(data any) bool { return data == "a session" }
				drops := 0
				d.OnDrop = func(string, Dropped, *gunim.UI) gunim.Intent { drops++; return nil }
				w, run, sent := stage(t, d, l)
				run(30)
				at := tabAt(t, d, "a")
				if !onBar {
					at = groupArea(t, d, l.GroupOf("a").ID).Center()
				}
				var over, lit, took bool
				onUIDock(t, w, run, func(u *gunim.UI) {
					var n gunim.Handler = d
					local := at
					if onBar {
						n = d.groups[0].bar
						local = at.Sub(d.groups[0].top.Min)
					}
					over = n.Handle(input.DragOver{Pos: local, Data: data}, u)
					lit = d.groups[0].bar.landing >= 0 || d.dropLit.Target() > 0
					took = n.Handle(input.Drop{Pos: local, Data: data}, u)
				})
				landed := drops > 0
				for _, in := range sent() {
					_, move := in.(Move)
					landed = landed || move
				}
				if want := !locked; over != want || lit != want || took != want || landed != want {
					t.Errorf("locked %v, %s, on a bar %v: over it %v, lit %v, taken %v, landed %v", locked, what, onBar, over, lit, took, landed)
				}
			}
		}
	}
}

// TestALockedDockStillPicksAndFocuses: with the pointer, the keys and a screen reader.
func TestALockedDockStillPicksAndFocuses(t *testing.T) {
	d, l := lockable()
	d.Locked = true
	w, run, sent := stage(t, d, l)
	run(30)
	wantShow := func(what, id string) {
		t.Helper()
		if got := sent(); len(got) != 1 || got[0] != (Show{ID: id}) {
			t.Fatalf("%s sent %v, want Show %s", what, got, id)
		}
		if !slices.Contains(d.Shown(), id) {
			t.Fatalf("after %s, %v show", what, d.Shown())
		}
	}
	click(w, run, tabAt(t, d, "c"), input.ButtonPrimary)
	wantShow("a click on a tab", "c")
	// Its view fades in as ever, forward on every frame.
	last := float32(-1)
	for range 40 {
		v := d.showing["c"].on.Value()
		if v < last {
			t.Fatalf("the fade went back, from %v to %v", last, v)
		}
		last = v
		run(1)
	}
	if last < 0.99 {
		t.Errorf("the view came to %v", last)
	}
	w.Input(input.KeyPress{Key: input.KeyPageUp, Mods: input.ModControl})
	run(1)
	wantShow("Ctrl+Page Up", "a")
	w.Input(input.KeyPress{Key: input.KeyTab, Mods: input.ModControl})
	run(1)
	if d.Active() != "b" {
		t.Errorf("Ctrl+Tab went to %s", d.Active())
	}
	click(w, run, d.items["a"].Center(), input.ButtonPrimary)
	if d.Active() != "a" {
		t.Errorf("a press in a pane has the keys work in %s", d.Active())
	}
}

// TestALockedDockShowsTheApplicationsChangesInMotion: a new split slides open on every
// frame, and an item the application fills the window with has no button of the user's.
func TestALockedDockShowsTheApplicationsChangesInMotion(t *testing.T) {
	d, l := lockable()
	d.Locked = true
	w, run, _ := stage(t, d, l)
	run(30)
	l2 := l.Clone()
	l2.Add("d", Target{At: "b", Side: Bottom})
	if err := w.Client().Update("v", l2); err != nil {
		t.Fatal(err)
	}
	split := l2.Parent(l2.GroupOf("d").ID).ID
	last := float32(math.Inf(1))
	first := true
	for range 60 {
		run(1)
		r, ok := d.DividerRect(split)
		if !ok {
			t.Fatal("no divider for the new split")
		}
		if first && r.Min.Y < 400 {
			t.Fatalf("the new split starts open, its divider at %v", r)
		}
		first = false
		if r.Min.Y > last+0.5 {
			t.Fatalf("the divider went back, from %v to %v", last, r.Min.Y)
		}
		last = r.Min.Y
	}
	if math.Abs(float64(last-248)) > 4 {
		t.Errorf("the split came to %v", last)
	}
	onUIDock(t, w, run, func(u *gunim.UI) { d.ToggleMax("b", u) })
	for range 30 {
		run(1)
		if d.restoreShows() {
			t.Fatal("a locked dock has a button that puts the panes back")
		}
	}
	if d.Maxed() != "b" {
		t.Fatalf("the application filled the window with %q", d.Maxed())
	}
	onUIDock(t, w, run, func(u *gunim.UI) { d.ToggleMax("b", u) })
	if d.Maxed() != "" {
		t.Error("the application could not put the panes back")
	}
	run(60)
	if !w.Still() {
		t.Error("a locked dock at rest asks for frames")
	}
}

// TestTheControlsFadeAsTheDockLocks: on every frame, forward, the + where it was, and
// the tabs slide to their new room. Unlocked, they come back the same way.
func TestTheControlsFadeAsTheDockLocks(t *testing.T) {
	d, l := lockable()
	w, run, _ := stage(t, d, l)
	run(30)
	bar := d.groups[0].bar
	widest := func() float32 {
		r, _ := bar.drawnOf("a")
		return r.Size().W
	}
	free, plus, wide := bar.freeLit(), bar.plusAt.Value(), widest()
	widthWas, grips := wide, d.free.Value()
	if free != 1 || d.free.Value() != 1 {
		t.Fatalf("unlocked, the controls show %v, and the grips %v", free, d.free.Value())
	}
	onUIDock(t, w, func(int) {}, func(u *gunim.UI) { d.Locked = true; u.Invalidate() })
	for i := range 60 {
		run(1)
		now, w2 := bar.freeLit(), widest()
		if now > free+0.001 || free-now > 0.2 {
			t.Fatalf("frame %d: the controls went from %v to %v", i, free, now)
		}
		if w2 > wide+0.5 || wide-w2 > 3 {
			t.Fatalf("frame %d: the tab went from %v to %v wide", i, wide, w2)
		}
		if now > 0 && bar.plusAt.Value() != plus {
			t.Fatalf("frame %d: the + fades at %v, it was at %v", i, bar.plusAt.Value(), plus)
		}
		grips2 := max(d.free.Value(), 0)
		if grips2 > grips+0.001 || grips-grips2 > 0.2 {
			t.Fatalf("frame %d: the grips went from %v to %v", i, grips, grips2)
		}
		free, wide, grips = now, w2, grips2
	}
	if r := bar.boxes[0]; widthWas-wide < 10 || math.Abs(float64(r.Size().W-wide)) > 0.5 {
		t.Errorf("locked, the tab is %v wide, from %v, in room of %v", wide, widthWas, r.Size().W)
	}
	if free > 0.001 || d.free.Value() > 0.001 {
		t.Errorf("locked, the controls show %v, and the grips %v", free, d.free.Value())
	}
	if !w.Still() {
		t.Error("locked and at rest, the dock asks for frames")
	}
	onUIDock(t, w, func(int) {}, func(u *gunim.UI) { d.Locked = false; u.Invalidate() })
	for i := range 60 {
		run(1)
		now := bar.freeLit()
		if now < free-0.01 || now-free > 0.2 {
			t.Fatalf("frame %d: the controls went from %v to %v", i, free, now)
		}
		free = now
	}
	if free < 0.999 || bar.plusAt.Value() != plus || bar.PlusRect().Empty() {
		t.Errorf("unlocked, the controls show %v, the + at %v", free, bar.plusAt.Value())
	}
}

// TestADockLockedFromTheStartShowsNoControls: its first frame jumps, and unlocked later
// the + fades in where it belongs.
func TestADockLockedFromTheStartShowsNoControls(t *testing.T) {
	d, l := lockable()
	d.Locked = true
	w, run, _ := stage(t, d, l)
	bar := d.groups[0].bar
	if bar.freeLit() != 0 || d.free.Value() != 0 {
		t.Fatalf("on the first frames the controls show %v, and the grips %v", bar.freeLit(), d.free.Value())
	}
	setLocked(t, w, run, d, false)
	for i := range 30 {
		if at := bar.plusAt.Value(); at.Min.X < bar.boxes[0].Max.X-20 {
			t.Fatalf("frame %d: the + comes from %v", i, at)
		}
		run(1)
	}
	if bar.freeLit() < 0.999 {
		t.Errorf("the controls came to %v", bar.freeLit())
	}
}

// TestLockingEndsADividersDrag: in the middle of it, the divider glides back where it
// was, and nothing is asked for.
func TestLockingEndsADividersDrag(t *testing.T) {
	d, l := lockable()
	w, run, sent := stage(t, d, l)
	run(30)
	dv := d.dividers[0].rect.Center()
	w.Input(input.PointerMove{Pos: dv})
	w.Input(input.PointerDown{Pos: dv, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	w.Input(input.PointerMove{Pos: geom.Pt(200, dv.Y)})
	run(1)
	last := d.dividers[0].rect.Center().X
	onUIDock(t, w, func(int) {}, func(u *gunim.UI) { d.Locked = true; u.Invalidate() })
	for i := range 60 {
		run(1)
		// The pointer still down drags nothing.
		w.Input(input.PointerMove{Pos: geom.Pt(120+float32(i), dv.Y)})
		x := d.dividers[0].rect.Center().X
		if x < last-0.5 || x-last > 35 {
			t.Fatalf("frame %d: the divider went from %v to %v", i, last, x)
		}
		last = x
	}
	if d.Held() || math.Abs(float64(last-dv.X)) > 0.5 {
		t.Errorf("the divider came to %v, want %v; held %v", last, dv.X, d.Held())
	}
	w.Input(input.PointerUp{Pos: geom.Pt(120, dv.Y), Button: input.ButtonPrimary})
	run(2)
	if got := sent(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
}

// twoDocks shows two docks side by side, each in half the window.
type twoDocks struct{ docks [2]*Dock }

func (n *twoDocks) Children() []gunim.Node { return []gunim.Node{n.docks[0], n.docks[1]} }

func (n *twoDocks) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	i := 0
	for k := range kids.All {
		k.Layout(gunim.Tight(geom.Sz(c.Max.W/2, c.Max.H)))
		k.Place(geom.Pt(float32(i)*c.Max.W/2, 0))
		i++
	}
	return c.Max
}

func (n *twoDocks) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// TestLockingEndsATabsDrag: in the frame the dock locks the tab is back on its bar, and
// the pointer, still down, carries nothing: let go outside every window, over the panes,
// or over another dock that is not locked, it moves nothing.
func TestLockingEndsATabsDrag(t *testing.T) {
	for what, to := range map[string]geom.Point{"outside": geom.Pt(1200, 900), "over its own panes": geom.Pt(300, 480), "over another dock": geom.Pt(600, 250)} {
		for _, locks := range []bool{false, true} {
			d, l := lockable()
			d2, _ := hosting()
			d2.SetWin(1, nil)
			// The other dock knows every item, as the layout of one application does.
			whole := l.Clone()
			whole.AddWindow(true)
			whole.Add("z", Target{Win: 1, Index: -1})
			d2.SetLayout(whole, nil)
			root := &twoDocks{docks: [2]*Dock{d, d2}}
			w := gunimtest.New(t, geom.Sz(800, 500), nil)
			gunim.RegisterView(w, "v", func(Layout) *twoDocks { return root }, func(n *twoDocks, l Layout, u *gunim.UI) { n.docks[0].SetLayout(l, u) })
			gunim.RegisterPatch(w, "v", func(_ *twoDocks, call uiCall, u *gunim.UI) { call(u) })
			if err := w.Client().Mount(gunim.Root, "v", "v", whole, "v"); err != nil {
				t.Fatal(err)
			}
			run := func(k int) {
				for range k {
					w.Frame(time.Second / 60)
				}
			}
			run(30)
			from := tabAt(t, d, "c")
			w.Input(input.PointerMove{Pos: from})
			w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1})
			run(1)
			w.Input(input.PointerMove{Pos: from.Add(geom.Pt(30, 30))})
			run(1)
			w.Input(input.PointerMove{Pos: geom.Pt(300, 480)})
			run(2)
			bar := d.groups[0].bar
			if bar.Carried() != "c" || d.dropLit.Target() == 0 {
				t.Fatalf("%s: the bar carries %q, and the drag lights %v", what, bar.Carried(), d.dropLit.Target())
			}
			if locks {
				setLocked(t, w, run, d, true)
				if bar.Carried() != "" {
					t.Errorf("%s: locked, the bar still carries %q", what, bar.Carried())
				}
				run(1)
				if d.dropLit.Target() != 0 {
					t.Errorf("%s: locked, the drag still lights where it would land", what)
				}
			}
			w.Input(input.PointerMove{Pos: to})
			run(1)
			w.Input(input.PointerUp{Pos: to, Button: input.ButtonPrimary})
			run(2)
			asked := false
			for {
				select {
				case e := <-w.Client().Intents():
					switch e.Intent.(type) {
					case Move, Detach:
						asked = true
					}
					continue
				default:
				}
				break
			}
			if asked == locks {
				t.Errorf("%s, locking in between %v: something asked for, %v", what, locks, asked)
			}
			if locks && (bar.Dropped() != "" || bar.Carried() != "") {
				t.Errorf("%s: the bar says %q left it, and carries %q", what, bar.Dropped(), bar.Carried())
			}
		}
	}
}

// TestTheRestoreButtonFadesAsTheDockLocks: the user's way to put the panes back goes, on
// every frame, and comes back as the dock unlocks.
func TestTheRestoreButtonFadesAsTheDockLocks(t *testing.T) {
	d, l := lockable()
	w, run, _ := stage(t, d, l)
	onUIDock(t, w, run, func(u *gunim.UI) { d.ToggleMax("b", u) })
	run(60)
	at := d.restoreAt.Center()
	last := d.restoreLit.Value()
	onUIDock(t, w, func(int) {}, func(u *gunim.UI) { d.Locked = true; u.Invalidate() })
	for i := range 60 {
		run(1)
		v := max(d.restoreLit.Value(), 0)
		if v > last+0.001 || last-v > 0.2 {
			t.Fatalf("frame %d: the button went from %v to %v", i, last, v)
		}
		last = v
		if i == 1 {
			click(w, func(int) {}, at, input.ButtonPrimary)
		}
	}
	if d.Maxed() != "b" || d.restoreShows() {
		t.Errorf("locked, %q fills the window, and the button shows %v", d.Maxed(), d.restoreShows())
	}
	setLocked(t, w, run, d, false)
	run(60)
	click(w, run, d.restoreAt.Center(), input.ButtonPrimary)
	if d.Maxed() != "" {
		t.Error("unlocked, the button does not put the panes back")
	}
}

// TestALockedBarOnItsOwn: it picks, and asks for nothing else.
func TestALockedBarOnItsOwn(t *testing.T) {
	b := NewBar("g", 3)
	b.Plus, b.Locked = true, true
	w, run, sent := stageBar(t, b, threeTabs())
	click(w, run, barTabAt(t, b, "3"), input.ButtonMiddle)
	drag(w, run, barTabAt(t, b, "1"), barTabAt(t, b, "3").Add(geom.Pt(30, 0)))
	click(w, run, geom.Pt(b.end+15, 18), input.ButtonPrimary)
	if got := sent(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
	click(w, run, barTabAt(t, b, "2"), input.ButtonPrimary)
	if got := sent(); len(got) != 1 || got[0] != (Show{ID: "2"}) {
		t.Errorf("a click sent %v", got)
	}
}
