package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type sheetShut struct{}

// sheetStage mounts a page with a button that has the keyboard in a window of size, opens a sheet holding body over
// it, and runs the frames it takes to spring up.
func sheetStage(t *testing.T, size geom.Size, body gunim.Node) (*BottomSheet, *Button, *gunim.Window, *grab, func(int)) {
	t.Helper()
	b := NewButton("Open")
	w, g, run := phoneStage(t, size, Column(b))
	g.ui.Focus(b)
	run(1)
	s := NewBottomSheet("Share", body)
	s.OnDismiss = Sends(sheetShut{})
	g.ui.Insert(g.ui.Root(), s)
	run(1)
	return s, b, w, g, run
}

// sheetRests runs frames until the sheet rests, and fails the test where a frame moves it further than a spring would.
func sheetRests(t *testing.T, s *BottomSheet, run func(int)) {
	t.Helper()
	last := s.pos.Value()
	for i := range 120 {
		run(1)
		v := s.pos.Value()
		if d := v - last; d > 120 || d < -120 {
			t.Fatalf("frame %d: the sheet jumped from %v to %v", i, last, v)
		}
		last = v
	}
}

// A sheet springs up half open over the page, takes the keyboard, closes on Escape, tells the program, and gives
// the keyboard back.
func TestASheetSpringsUpAndClosesOnEscape(t *testing.T) {
	s, b, w, g, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 2000))
	sheetRests(t, s, run)
	if h := s.pos.Value(); h < 399 || h > 401 {
		t.Fatalf("the sheet rests %v up, want half the window", h)
	}
	if g.ui.Focused() != gunim.Node(s) {
		t.Fatalf("the keyboard is on %v, want the sheet", g.ui.Focused())
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (sheetShut{}) {
		t.Fatalf("Escape sent %v", got)
	}
	last := s.pos.Value()
	for i := range 60 {
		run(1)
		if v := s.pos.Value(); v > last+0.5 {
			t.Fatalf("frame %d: the sheet going rose from %v to %v", i, last, v)
		} else {
			last = v
		}
	}
	if g.ui.Presence(s) != gunim.Exiting || g.ui.Drawn(s) {
		t.Fatal("the sheet stays after Escape")
	}
	if !g.ui.HasFocus(b) {
		t.Fatalf("the keyboard went to %v, want back to the button", g.ui.Focused())
	}
}

// A tap on the page above the sheet closes it, and a tap on the sheet does not.
func TestATapAboveTheSheetClosesIt(t *testing.T) {
	s, _, w, g, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 100))
	sheetRests(t, s, run)
	tap(w, run, geom.Pt(200, s.panel.Min.Y+20))
	if got := sent(w); len(got) != 0 || g.ui.Presence(s) == gunim.Exiting {
		t.Fatalf("a tap on the sheet's title sent %v and closed it", got)
	}
	fingerDrag(w, run, geom.Pt(200, 100), geom.Pt(200, 300), 8, 300*time.Millisecond, 150*time.Millisecond)
	sheetRests(t, s, run)
	if got := sent(w); len(got) != 0 || s.pos.Value() < s.half-0.5 {
		t.Fatalf("a drag on the page above the sheet sent %v and took the sheet to %v", got, s.pos.Value())
	}
	tap(w, run, geom.Pt(200, 100))
	if got := sent(w); len(got) != 1 || g.ui.Presence(s) != gunim.Exiting {
		t.Fatalf("a tap above the sheet sent %v, its presence %v", got, g.ui.Presence(s))
	}
}

// A drag on the top moves the sheet with the finger: dragged up it opens full, let go slowly a little lower it
// stays full, flung down it goes back to half, and flung down from half it closes.
func TestDraggingTheTopMovesTheSheet(t *testing.T) {
	s, _, w, _, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 2000))
	sheetRests(t, s, run)
	top := s.panel.Min.Y + 10
	fingerDrag(w, run, geom.Pt(200, top), geom.Pt(200, top-300), 10, 400*time.Millisecond, 150*time.Millisecond)
	if !s.Expanded() {
		t.Fatalf("dragged up 300 the sheet stands %v up and heads for half", s.pos.Value())
	}
	sheetRests(t, s, run)
	if h := s.pos.Value(); h < 775 || h > 777 {
		t.Fatalf("full, the sheet stands %v up, want the window less the room above it, 776", h)
	}
	top = s.panel.Min.Y + 10
	fingerDrag(w, run, geom.Pt(200, top), geom.Pt(200, top+60), 6, 400*time.Millisecond, 150*time.Millisecond)
	sheetRests(t, s, run)
	if !s.Expanded() || s.pos.Value() < 775 {
		t.Fatalf("let go slowly 60 down, the sheet stands %v up", s.pos.Value())
	}
	fingerDrag(w, run, geom.Pt(200, top), geom.Pt(200, top+120), 4, 60*time.Millisecond, 0)
	sheetRests(t, s, run)
	if s.Expanded() || s.pos.Value() > 401 {
		t.Fatalf("flung down from full the sheet stands %v up", s.pos.Value())
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("moving the sheet sent %v", got)
	}
	top = s.panel.Min.Y + 10
	fingerDrag(w, run, geom.Pt(200, top), geom.Pt(200, top+120), 4, 60*time.Millisecond, 0)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (sheetShut{}) {
		t.Fatalf("flung down from half the sheet sent %v", got)
	}
}

// A drag on the body scrolls the body first. Once the body is back at its top, the rest of a drag down takes the
// sheet down, and let go slowly it springs back.
func TestTheBodyScrollsBeforeTheSheetMoves(t *testing.T) {
	s, _, w, _, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 2000))
	sheetRests(t, s, run)
	body, ok := s.held().(*Scroll)
	if !ok {
		t.Fatal("the sheet holds its body in no scroll")
	}
	fingerDrag(w, run, geom.Pt(200, 700), geom.Pt(200, 500), 10, 500*time.Millisecond, 150*time.Millisecond)
	sheetRests(t, s, run)
	scrolled := body.Offset()
	if scrolled < 150 || s.pos.Value() < 399 || s.pos.Value() > 401 {
		t.Fatalf("a drag up on the body scrolled it %v and moved the sheet to %v", scrolled, s.pos.Value())
	}
	// Down by more than the body scrolled: the body goes back to its top, and the sheet down by the rest.
	t0 := time.Now()
	w.Input(input.PointerMove{Pos: geom.Pt(200, 500), Touch: true, Time: t0})
	w.Input(input.PointerDown{Pos: geom.Pt(200, 500), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: t0})
	run(1)
	lowest := s.pos.Value()
	for i := 1; i <= 15; i++ {
		y := 500 + float32(i)*20
		w.Input(input.PointerMove{Pos: geom.Pt(200, y), Touch: true, Time: t0.Add(time.Duration(i) * 40 * time.Millisecond)})
		run(1)
		lowest = min(lowest, s.pos.Value())
	}
	run(20)
	if body.Offset() > 0.5 || lowest > 380 {
		t.Fatalf("dragged 300 down, the body is at %v and the sheet came down to %v", body.Offset(), lowest)
	}
	end := t0.Add(800 * time.Millisecond)
	w.Input(input.PointerUp{Pos: geom.Pt(200, 800), Button: input.ButtonPrimary, Touch: true, Time: end})
	w.Input(input.PointerLeave{Time: end})
	sheetRests(t, s, run)
	if h := s.pos.Value(); h < 399 || h > 401 {
		t.Fatalf("let go slowly, the sheet rests %v up, want back at half", h)
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("the drags sent %v", got)
	}
}

// A sheet as tall as its body where that is shorter than half, and no taller than the window, which a wide
// window stands centred in, and the keyboard pushes up.
func TestASheetFitsItsBodyAndTheWindow(t *testing.T) {
	s, _, _, g, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 100))
	sheetRests(t, s, run)
	if want := s.header + 100 + s.pad; s.pos.Value() < want-0.5 || s.pos.Value() > want+0.5 {
		t.Fatalf("a short body's sheet stands %v up, want %v", s.pos.Value(), want)
	}
	s.SetExpanded(true, g.ui)
	sheetRests(t, s, run)
	if want := s.header + 100 + s.pad; s.pos.Value() > want+0.5 {
		t.Fatalf("opened full, a short body's sheet stands %v up, want %v", s.pos.Value(), want)
	}
	if s.Access().State.Has(access.StateExpandable) {
		t.Fatal("a sheet with one height reads as expandable")
	}

	tall, _, w2, g2, run2 := sheetStage(t, geom.Sz(1000, 700), newSpot(390, 2000))
	sheetRests(t, tall, run2)
	if tall.panel.Min.X != 180 || tall.panel.Size().W != 640 {
		t.Fatalf("in a wide window the sheet stands at %v", tall.panel)
	}
	w2.Offscreen().SetKeyboardCover(300)
	w2.Input(driver.Redraw{})
	sheetRests(t, tall, run2)
	if bottom := tall.panel.Min.Y + tall.pos.Value(); bottom < 399 || bottom > 401 {
		t.Fatalf("with the keyboard 300 up the sheet's bottom stands at %v, want on the keyboard at 400", bottom)
	}
	if h := tall.pos.Value(); h < 199 || h > 201 {
		t.Fatalf("with the keyboard up the half sheet stands %v up, want half of what the keyboard leaves", h)
	}
	tall.SetExpanded(true, g2.ui)
	sheetRests(t, tall, run2)
	if h := tall.pos.Value(); h < 375 || h > 377 {
		t.Fatalf("full over the keyboard, the sheet stands %v up, want 376", h)
	}
}

// A screen reader hears a modal dialog named by the title, expandable, and can open it full and close it. With the
// keyboard on the sheet, Up opens it full and Down takes it back.
func TestAScreenReaderWorksTheSheet(t *testing.T) {
	s, _, w, g, run := sheetStage(t, geom.Sz(390, 800), newSpot(390, 2000))
	sheetRests(t, s, run)
	info := s.Access()
	if info.Role != access.RoleDialog || info.Name != "Share" || !info.State.Has(access.StateModal|access.StateExpandable) ||
		info.State.Has(access.StateExpanded) {
		t.Fatalf("the sheet reads as %v %q, %v", info.Role, info.Name, info.State)
	}
	w.Input(input.KeyPress{Key: input.KeyUp})
	sheetRests(t, s, run)
	if !s.Expanded() || !s.Access().State.Has(access.StateExpanded) {
		t.Fatal("Up left the sheet at half")
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	sheetRests(t, s, run)
	if s.Expanded() {
		t.Fatal("Down left the sheet full")
	}
	if !s.AccessAct(access.Request{Part: -1, Action: access.ActionOpen}, g.ui) || !s.Expanded() {
		t.Fatal("the screen reader's Open left the sheet at half")
	}
	if !s.AccessAct(access.Request{Part: -1, Action: access.ActionClose}, g.ui) {
		t.Fatal("the screen reader's Close was refused")
	}
	run(1)
	if got := sent(w); len(got) != 1 || g.ui.Presence(s) != gunim.Exiting {
		t.Fatalf("the screen reader's Close sent %v", got)
	}
}
