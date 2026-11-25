package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

type barWent struct{ I int }

// barPlaces are three places with icons, the second with a count.
func barPlaces() []BarItem {
	return []BarItem{
		{Key: "home", Label: "Home", Icon: icon.House},
		{Key: "mail", Label: "Mail", Icon: icon.Mail, Count: 4, CountLabel: "4 unread"},
		{Key: "you", Label: "You", Icon: icon.User},
	}
}

// barStage mounts a bar of three places over three pages in a window of size.
func placesStage(t *testing.T, size geom.Size) (*BottomBar, []*spot, *gunim.Window, *grab, func(int)) {
	t.Helper()
	pages := []*spot{newSpot(10, 10), newSpot(10, 10), newSpot(10, 10)}
	b := NewBottomBar(barPlaces(), pages[0], pages[1], pages[2])
	b.Label = "Places"
	b.OnSelect = func(i int, _ *gunim.UI) gunim.Intent { return barWent{i} }
	w, g, run := phoneStage(t, size, b)
	return b, pages, w, g, run
}

// placeAt is the middle of place i's icon, in the window, as the bar last stood.
func placeAt(t *testing.T, b *BottomBar, g *grab, i int) geom.Point {
	t.Helper()
	r, ok := g.ui.Bounds(b.strip)
	if !ok {
		t.Fatal("the bar was not drawn")
	}
	return pillRect(b.strip.slot(float32(i), b.strip.th), b.strip.th).Center().Add(r.Min)
}

// On a phone the bar runs along the bottom and the page fills what it leaves; a tap goes to a place, tells the
// program, and the pill glides there on every frame while the new page fades in.
func TestATapOnAPlaceGoesThere(t *testing.T) {
	b, pages, w, g, run := placesStage(t, geom.Sz(390, 800))
	if b.Rail() {
		t.Fatal("a bar 390 wide stands as a rail")
	}
	strip, _ := g.ui.Bounds(b.strip)
	if strip.Max.Y != 800 || strip.Min.Y < 700 || pages[0].box.H != strip.Min.Y {
		t.Fatalf("the bar stands at %v and the page is %v, want the bar along the bottom and the page above it",
			strip, pages[0].box)
	}
	tap(w, run, placeAt(t, b, g, 2))
	if b.Selected() != 2 {
		t.Fatalf("a tap on the third place left place %d current", b.Selected())
	}
	if got := sent(w); len(got) != 1 || got[0] != (barWent{2}) {
		t.Fatalf("going to the third place sent %v", got)
	}
	last := b.strip.pill.Value()
	for i := range 60 {
		run(1)
		v := b.strip.pill.Value()
		if v < last-0.05 || v > 2.1 {
			t.Fatalf("frame %d: the pill went from %v to %v on its way to place 2", i, last, v)
		}
		last = v
	}
	if last < 1.99 || b.prev != -1 {
		t.Fatalf("the pill rests at %v, the old page %d still fading", last, b.prev)
	}
	// A tap on the current place sends nothing more.
	tap(w, run, placeAt(t, b, g, 2))
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a tap on the current place sent %v", got)
	}
}

// With the keyboard on the bar, the arrows move the ring along it and Enter goes to the place with the ring; the
// pages hidden keep their state, as they stay in the tree.
func TestTheKeysWorkTheBar(t *testing.T) {
	b, pages, w, g, run := placesStage(t, geom.Sz(390, 800))
	g.ui.Focus(b.strip)
	run(1)
	for _, k := range []input.Key{input.KeyRight, input.KeyRight, input.KeyRight} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	if b.Cursor() != 2 || b.Selected() != 0 {
		t.Fatalf("three Rights put the ring on %d and the current place on %d, want 2 and 0", b.Cursor(), b.Selected())
	}
	w.Input(input.KeyPress{Key: input.KeyHome})
	w.Input(input.KeyPress{Key: input.KeyRight})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(90)
	if b.Selected() != 1 {
		t.Fatalf("Home, Right and Enter went to place %d, want 1", b.Selected())
	}
	if got := sent(w); len(got) != 1 || got[0] != (barWent{1}) {
		t.Fatalf("Enter sent %v", got)
	}
	if !g.ui.Drawn(pages[1]) || g.ui.Drawn(pages[0]) {
		t.Fatalf("page 1 drawn %v, page 0 drawn %v, want only the current place's", g.ui.Drawn(pages[1]), g.ui.Drawn(pages[0]))
	}
	if g.ui.Presence(pages[0]) == gunim.Exiting {
		t.Fatal("the page left hid by leaving the tree, and loses its state")
	}
}

// In a window wide enough the bar stands as a rail down the left. As the window narrows it slides out and the bar
// slides in along the bottom, the page's room changing a little on every frame.
func TestTheBarTurnsToARailWhereThereIsRoom(t *testing.T) {
	b, pages, w, g, run := placesStage(t, geom.Sz(800, 600))
	if !b.Rail() {
		t.Fatal("a bar 800 wide stands along the bottom")
	}
	r, _ := g.ui.Bounds(b.strip)
	if r.Min.X != 0 || r.Size().W != 80 || pages[0].at.X != 80 || pages[0].box.H != 600 {
		t.Fatalf("the rail stands at %v and the page at %v, %v", r, pages[0].at, pages[0].box)
	}
	// A tap on the rail's second place goes there.
	tap(w, run, placeAt(t, b, g, 1))
	if b.Selected() != 1 {
		t.Fatalf("a tap on the rail's second place left %d current", b.Selected())
	}
	run(60)
	lastX, lastH := pages[1].at.X, pages[1].box.H
	w.Offscreen().Resize(geom.Sz(390, 600))
	w.Input(driver.Redraw{})
	for i := range 90 {
		run(1)
		x, h := pages[1].at.X, pages[1].box.H
		if x > lastX+0.01 || h > lastH+0.01 || x-lastX < -40 || h-lastH < -40 {
			t.Fatalf("frame %d: the page went from %v, %v tall to %v, %v tall", i, lastX, lastH, x, h)
		}
		lastX, lastH = x, h
	}
	if b.Rail() {
		t.Fatal("narrowed to 390 the bar still heads for the rail")
	}
	strip, _ := g.ui.Bounds(b.strip)
	if lastX != 0 || strip.Max.Y != 600 || lastH != strip.Min.Y {
		t.Fatalf("narrowed, the page stands at x %v, %v tall, the bar at %v", lastX, lastH, strip)
	}
	// Up and Down work the rail, Left and Right the bar.
	g.ui.Focus(b.strip)
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	if b.Cursor() != 2 {
		t.Fatalf("Right along the bar put the ring on %d", b.Cursor())
	}
}

// A screen reader hears a tab list with a tab for each place, the current one selected and the count as its
// description, and can go to a place.
func TestAScreenReaderHearsThePlacesAsTabs(t *testing.T) {
	b, _, _, g, run := placesStage(t, geom.Sz(390, 800))
	info := b.strip.Access()
	if info.Role != access.RoleTabList || info.Name != "Places" || len(info.Parts) != 3 {
		t.Fatalf("the bar reads as %v %q with %d parts", info.Role, info.Name, len(info.Parts))
	}
	for i, p := range info.Parts {
		if p.Role != access.RoleTab || p.State.Has(access.StateSelected) != (i == 0) {
			t.Fatalf("place %d reads as %v, %v", i, p.Role, p.State)
		}
	}
	if info.Parts[1].Description != "4 unread" {
		t.Fatalf("the count reads as %q", info.Parts[1].Description)
	}
	if !b.strip.AccessAct(access.Request{Part: 2, Action: access.ActionPress}, g.ui) {
		t.Fatal("the screen reader's press was refused")
	}
	run(1)
	if b.Selected() != 2 || !b.strip.Access().Parts[2].State.Has(access.StateSelected) {
		t.Fatalf("the screen reader's press left %d current", b.Selected())
	}
}

// The current place and the ring stay on their places by Key as the places change, and a place that goes leaves
// the current place in its spot.
func TestThePlacesKeepTheirKeys(t *testing.T) {
	b, _, _, g, run := placesStage(t, geom.Sz(390, 800))
	b.SetSelected(1, g.ui)
	run(30)
	items := barPlaces()
	items[0], items[2] = items[2], items[0]
	b.SetItems(items, g.ui)
	if b.Selected() != 1 {
		t.Fatalf("Mail moved nowhere, and the current place went to %d", b.Selected())
	}
	b.SetItems([]BarItem{items[1], items[0]}, g.ui)
	if b.Items()[b.Selected()].Key != "mail" {
		t.Fatalf("the current place went to %q", b.Items()[b.Selected()].Key)
	}
	b.SetItems([]BarItem{items[0]}, g.ui)
	run(30)
	if b.Selected() != 0 || b.Cursor() != 0 {
		t.Fatalf("one place left, the current place is %d and the ring on %d", b.Selected(), b.Cursor())
	}
}

// A bar of no places draws nothing and leaves the page the whole window; a bar of one place, or of long labels, or
// under text twice as large, draws inside its box, cuts its labels and grows taller.
func TestTheBarFitsWhatItHolds(t *testing.T) {
	page := newSpot(10, 10)
	b := NewBottomBar(nil, page)
	_, _, run := phoneStage(t, geom.Sz(390, 800), b)
	run(1)
	if page.box != geom.Sz(390, 800) {
		t.Fatalf("with no places the page is %v, want the whole window", page.box)
	}
	long := strings.Repeat("Notifications ", 4)
	for _, items := range [][]BarItem{
		{{Label: "Only"}},
		{{Label: long, Icon: icon.Bell, Count: 120}, {Label: long, Dot: true}, {Label: long}, {Label: long}, {Label: long}},
	} {
		for _, scale := range []float32{1, 2} {
			th := theme.NewLive(theme.ScaleText(Dark(), scale))
			b := NewBottomBar(items)
			s := b.strip
			s.laid, s.th = true, th
			s.size = geom.Sz(390, s.barHeight(th))
			var p paint.Painter
			s.Paint(&p, gunim.Frame{Scale: 1, Theme: th}, s.size, gunim.Children{})
			if bad := spills(p.Ops(), s.size, 3); len(bad) > 0 {
				t.Fatalf("%d places at text %v: drawn outside the bar: %v", len(items), scale, bad)
			}
			if scale == 2 && s.barHeight(th) <= s.barHeight(theme.NewLive(Dark())) {
				t.Fatal("the bar keeps its height under text twice as large")
			}
			if len(items) > 1 && s.tipText(0) != long {
				t.Fatalf("a label cut short gives %q as its tooltip", s.tipText(0))
			}
		}
	}
}

// The bar keeps its places clear of a phone's navigation bar, and its ground runs on under it.
func TestTheBarRunsUnderThePhonesBar(t *testing.T) {
	b, pages, w, g, run := placesStage(t, geom.Sz(390, 800))
	w.Offscreen().SetSafeArea(geom.Insets{Top: 24, Bottom: 32})
	w.Input(driver.Redraw{})
	run(3)
	strip, _ := g.ui.Bounds(b.strip)
	if strip.Max.Y != 800-32 || pages[0].box.H != strip.Min.Y-24 {
		t.Fatalf("with a bar of 32 the bar stands at %v and the page is %v tall", strip, pages[0].box.H)
	}
	var reach float32
	fill := MenubarFill.Get(b.strip.th)
	for _, op := range w.Offscreen().Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Fill.Solid == fill {
			reach = max(reach, r.Transform.Apply(r.Rect.Max).Y)
		}
	}
	if reach < 799 {
		t.Fatalf("the bar's ground reaches %v down, want under the phone's bar to the window's edge", reach)
	}
}
