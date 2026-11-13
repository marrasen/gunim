package widget

import (
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// grab holds a node and keeps the UI its frames are laid out in, for a test to call setters with.
type grab struct {
	child gunim.Node
	ui    *gunim.UI
}

func (g *grab) Children() []gunim.Node { return []gunim.Node{g.child} }

func (g *grab) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	g.ui = f.UI()
	k := kids.At(0)
	s := k.Layout(c)
	k.Place(geom.Point{})
	return s
}

func (g *grab) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// touchStage mounts root under the touch theme in a window that keeps time by its frames, and returns the window,
// the UI and a function that runs n frames of 1/60 s.
func touchStage(t *testing.T, root gunim.Node, size geom.Size) (*gunim.Window, *grab, func(int)) {
	t.Helper()
	g := &grab{child: NewThemed(root, Dark().With(Touch...))}
	w, run := stage(t, &frame{child: g, size: size})
	w.TimeByFrames(true)
	run(1)
	return w, g, run
}

// touchDown and touchUp land a finger at p and lift it, as a touch driver sends them: a move to the finger before
// it presses, and the pointer leaving as it lifts.
func touchDown(w *gunim.Window, p geom.Point) {
	w.Input(input.PointerMove{Pos: p, Touch: true, Time: time.Now()})
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: time.Now()})
}

func touchUp(w *gunim.Window, p geom.Point) {
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Touch: true, Time: time.Now()})
	w.Input(input.PointerLeave{Time: time.Now()})
}

// The touch theme sets text in Inter, makes the targets at least 48 tall and turns tooltips off.
func TestTheTouchThemeFollowsTheDesignLanguage(t *testing.T) {
	th := theme.NewLive(Dark().With(Touch...))
	if Font.Get(th) != text.Inter(false) || BoldFont.Get(th) != text.Inter(true) {
		t.Fatal("the touch theme sets text in another face than Inter")
	}
	for _, tok := range []theme.Token[float32]{ButtonHeight, FieldHeight, ControlHeight, MenuRowHeight, TabHeight,
		SegmentedHeight, ChipHeight, NavRowHeight, PinKeyLeast, KeyboardKey} {
		if v := tok.Get(th); v < 48 {
			t.Errorf("%s is %v under the touch theme, want 48 or more", tok.Key(), v)
		}
	}
	if Tooltips.Get(th) {
		t.Error("the touch theme leaves tooltips on")
	}
	if got := [4]float32{FigureSize.Get(th), HeadingSize.Get(th), TextSize.Get(th), CaptionSize.Get(th)}; got != [4]float32{40, 24, 17, 14} {
		t.Errorf("the four sizes of text are %v, want 40, 24, 17 and 14", got)
	}
}

// A finger lights and squashes a button on the frame it lands, and a glow spreads from it; lifted, the light fades.
func TestAFingerLightsAButtonOnTheFrameItLands(t *testing.T) {
	b := NewButton("Water now")
	w, _, run := touchStage(t, Row(b), geom.Sz(400, 100))
	at := geom.Pt(20, 20)
	touchDown(w, at)
	run(1)
	if b.hover.Value() <= 0 || b.squash.Value() <= 0 || b.touch.shine.Value() <= 0 {
		t.Fatalf("one frame after the finger landed: light %v, squash %v, glow %v; want all on their way up",
			b.hover.Value(), b.squash.Value(), b.touch.shine.Value())
	}
	if b.touch.at != at {
		t.Fatalf("the glow spreads from %v, want where the finger landed, %v", b.touch.at, at)
	}
	touchUp(w, at)
	run(60)
	if b.hover.Value() > 0.01 || b.touch.shine.Value() > 0.01 {
		t.Fatalf("lifted, the light is at %v and the glow at %v, want both gone", b.hover.Value(), b.touch.shine.Value())
	}
}

// A mouse's press squashes a button, as before, and spreads no glow.
func TestAMousePressSpreadsNoGlow(t *testing.T) {
	b := NewButton("Water now")
	w, _, run := touchStage(t, Row(b), geom.Sz(400, 100))
	w.Input(input.PointerDown{Pos: geom.Pt(20, 20), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	run(3)
	if b.squash.Value() <= 0 || b.touch.shine.Target() != 0 {
		t.Fatalf("a mouse press: squash %v, glow heading to %v; want a squash and no glow", b.squash.Value(), b.touch.shine.Target())
	}
}

// longColumn returns a column of buttons too tall for a window of 300, and its first button.
func longColumn() (gunim.Node, *Button) {
	kids := make([]gunim.Node, 0, 20)
	var first *Button
	for i := range 20 {
		b := NewButton("Row")
		if i == 0 {
			first = b
		}
		kids = append(kids, b)
	}
	return NewScroll(Column(kids...)), first
}

// Inside something that scrolls, a finger's press waits up to 80 ms before it shows, and one that goes on to scroll
// never lights the button it started on, on any frame.
func TestAFingerThatScrollsLightsNothing(t *testing.T) {
	col, b := longColumn()
	w, _, run := touchStage(t, col, geom.Sz(300, 300))
	at := geom.Pt(30, 20)
	touchDown(w, at)
	for i := range 3 {
		run(1)
		if b.hover.Value() != 0 || b.squash.Value() != 0 {
			t.Fatalf("frame %d, %d ms after landing in a scroll: light %v, squash %v; want none yet", i, (i+1)*16,
				b.hover.Value(), b.squash.Value())
		}
	}
	for i := range 6 {
		w.Input(input.PointerMove{Pos: geom.Pt(at.X, at.Y+float32(i+1)*10), Touch: true, Time: time.Now()})
		run(1)
		if b.hover.Value() != 0 || b.squash.Value() != 0 || b.touch.shine.Value() != 0 {
			t.Fatalf("frame %d of the scroll: light %v, squash %v, glow %v; want none", i, b.hover.Value(),
				b.squash.Value(), b.touch.shine.Value())
		}
	}
	touchUp(w, geom.Pt(at.X, at.Y+60))
	for i := range 30 {
		run(1)
		if b.hover.Value() != 0 || b.touch.shine.Value() != 0 {
			t.Fatalf("frame %d after the scroll: light %v, glow %v; want none", i, b.hover.Value(), b.touch.shine.Value())
		}
	}
}

// Inside something that scrolls, a finger held still lights the button once the wait is over, and a tap quicker
// than the wait still glows on it for a moment, and clicks it.
func TestAFingerHeldStillInAScrollLightsAfterTheWait(t *testing.T) {
	col, b := longColumn()
	clicks := 0
	b.OnClick = func(*gunim.UI) gunim.Intent { clicks++; return nil }
	w, _, run := touchStage(t, col, geom.Sz(300, 300))
	at := geom.Pt(30, 20)
	touchDown(w, at)
	run(7)
	if b.hover.Target() != 1 || b.squash.Target() != 1 {
		t.Fatalf("held still past the wait: light heading to %v, squash to %v; want both lit", b.hover.Target(), b.squash.Target())
	}
	touchUp(w, at)
	run(60)

	touchDown(w, at)
	run(1)
	touchUp(w, at)
	lit := false
	for range 50 {
		run(1)
		lit = lit || b.touch.shine.Value() > 0.3
	}
	if !lit {
		t.Fatal("a quick tap in a scroll never lit the button")
	}
	if b.hover.Value() > 0.01 || b.touch.shine.Value() > 0.01 {
		t.Fatalf("shortly after a quick tap the light is at %v and the glow at %v, want both gone",
			b.hover.Value(), b.touch.shine.Value())
	}
	if clicks != 2 {
		t.Fatalf("two taps clicked %d times, want 2", clicks)
	}
}

// Under the touch theme a pointer resting on a control opens no tooltip.
func TestTheTouchThemeShowsNoTooltip(t *testing.T) {
	b := NewButton("Water")
	b.Tooltip = "Waters the bed now"
	w, _, run := touchStage(t, Row(b), geom.Sz(300, 100))
	w.Input(input.PointerMove{Pos: geom.Pt(20, 20), Time: time.Now()})
	run(90)
	if b.tip.popup != nil || b.tip.stop != nil {
		t.Fatal("a tooltip opened, or waits to, under the touch theme")
	}
}

// A hold button acts once it has been held for Hold, and only once however long it is held.
func TestAHoldButtonActsOnceHeldLongEnough(t *testing.T) {
	b := NewHoldButton("Revoke laptop")
	b.OnClick = Sends(pressed{1})
	w, _, run := touchStage(t, Row(b), geom.Sz(400, 100))
	at := geom.Pt(30, 24)
	touchDown(w, at)
	last := float32(0)
	for i := range 59 {
		run(1)
		if p := b.Progress(); p < last-1e-4 {
			t.Fatalf("frame %d: the ring went back from %v to %v while held", i, last, p)
		}
		last = b.Progress()
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("held just short of a second, it sent %v", got)
	}
	run(60)
	if got := sent(w); len(got) != 1 || got[0] != (pressed{1}) {
		t.Fatalf("held two seconds, it sent %v, want one intent", got)
	}
	touchUp(w, at)
	run(90)
	if b.Progress() > 0.01 {
		t.Fatalf("let go, the ring stays at %v", b.Progress())
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("letting go sent %v", got)
	}
}

// Let go early, a hold button's ring drains back on every frame and nothing happens; pressed again before it has
// drained, it fills on from where it is, and acts sooner than a full hold.
func TestAHoldLetGoEarlyDrainsAndResumes(t *testing.T) {
	b := NewHoldButton("Remove guest")
	b.OnClick = Sends(pressed{2})
	w, _, run := touchStage(t, Row(b), geom.Sz(400, 100))
	at := geom.Pt(30, 24)
	touchDown(w, at)
	run(36)
	half := b.Progress()
	if half < 0.5 || half > 0.7 {
		t.Fatalf("after 0.6 s the ring is at %v, want about 0.6", half)
	}
	touchUp(w, at)
	last := b.Progress()
	for i := range 6 {
		run(1)
		if p := b.Progress(); p > last+1e-4 {
			t.Fatalf("frame %d after letting go: the ring grew from %v to %v", i, last, p)
		}
		last = b.Progress()
	}
	if last <= 0.05 {
		t.Fatalf("a tenth of a second after letting go the ring is at %v, want it still draining", last)
	}
	touchDown(w, at)
	frames := 0
	for len(sent(w)) == 0 && frames < 120 {
		run(1)
		frames++
	}
	if frames >= 60 {
		t.Fatalf("pressed again from %v, it took %d frames to act, want fewer than a full second's", last, frames)
	}
	touchUp(w, at)
	run(1)
}

// Space held down holds a hold button from the keyboard; let go early, nothing happens.
func TestAHoldButtonHoldsFromTheKeyboard(t *testing.T) {
	b := NewHoldButton("Revoke laptop")
	b.OnClick = Sends(pressed{3})
	w, g, run := touchStage(t, Row(b), geom.Sz(400, 100))
	g.ui.Focus(b)
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(30)
	w.Input(input.KeyRelease{Key: input.KeySpace})
	run(30)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("Space held half a second sent %v", got)
	}
	w.Input(input.KeyPress{Key: input.KeySpace})
	for range 20 {
		w.Input(input.KeyPress{Key: input.KeySpace, Repeat: true})
		run(4)
	}
	w.Input(input.KeyRelease{Key: input.KeySpace})
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("Space held over a second sent %v, want one intent", got)
	}
}

// A hold button tells a screen reader it is a button, its name, and that it asks to be held; a press from one acts.
func TestAHoldButtonSpeaksToAScreenReader(t *testing.T) {
	b := NewHoldButton("Revoke laptop")
	b.OnClick = Sends(pressed{4})
	w, g, run := touchStage(t, Row(b), geom.Sz(400, 100))
	info := b.Access()
	if info.Role != access.RoleButton || info.Name != "Revoke laptop" || info.Description == "" {
		t.Fatalf("access %+v, want a button named Revoke laptop that says to hold it", info)
	}
	if !b.AccessAct(access.Request{Part: -1, Action: access.ActionPress}, g.ui) {
		t.Fatal("a screen reader's press was refused")
	}
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a screen reader's press sent %v, want one intent", got)
	}
	b.Disabled = true
	run(2)
	if b.Access().State&access.StateDisabled == 0 || b.AccessAct(access.Request{Part: -1, Action: access.ActionPress}, g.ui) {
		t.Fatal("a disabled hold button reads as enabled, or acts")
	}
}

// pinStage mounts a PIN pad of six digits and returns it with its window, UI and frames.
func pinStage(t *testing.T) (*PinPad, *gunim.Window, *grab, func(int)) {
	t.Helper()
	p := NewPinPad(6, "Unlock")
	w, g, run := touchStage(t, p, geom.Sz(360, 420))
	return p, w, g, run
}

// tapKey taps the PIN pad's key i with a finger.
func tapKey(w *gunim.Window, run func(int), p *PinPad, i int) {
	c := p.keys[i].Center()
	touchDown(w, c)
	run(1)
	touchUp(w, c)
	run(1)
}

// pinKey is the index of the key that types digit d.
func pinKey(d int) int {
	if d == 0 {
		return pinZero
	}
	return d - 1
}

// Tapped digits fill dots, delete takes the last one off, and the commit key hands the PIN over once and clears it.
func TestAPinPadHandsThePinOverOnceAndClearsIt(t *testing.T) {
	p, w, _, run := pinStage(t)
	var got []string
	p.OnCommit = func(pin string, _ *gunim.UI) gunim.Intent { got = append(got, pin); return nil }
	for _, d := range []int{9, 7, 3, 1, 5} {
		tapKey(w, run, p, pinKey(d))
	}
	tapKey(w, run, p, pinDelete)
	if p.Len() != 4 {
		t.Fatalf("five digits and a delete leave %d digits, want 4", p.Len())
	}
	run(30)
	for i, d := range p.dots {
		if want := float32(0); i < 4 {
			if v := d.Value(); v < 0.95 {
				t.Fatalf("dot %d is %v full with a digit typed, want full", i, v)
			}
		} else if v := d.Value(); v > want+0.01 {
			t.Fatalf("dot %d is %v full with no digit, want empty", i, v)
		}
	}
	tapKey(w, run, p, pinEnter)
	if len(got) != 1 || got[0] != "9731" {
		t.Fatalf("the pad handed over %q, want 9731 once", got)
	}
	if p.Len() != 0 {
		t.Fatalf("after handing it over the pad holds %d digits, want none", p.Len())
	}
	tapKey(w, run, p, pinEnter)
	if len(got) != 1 {
		t.Fatalf("an empty pad handed over %q", got[1:])
	}
}

// The pad takes no more digits than its Length.
func TestAPinPadStopsAtItsLength(t *testing.T) {
	p, w, _, run := pinStage(t)
	for range 9 {
		tapKey(w, run, p, pinKey(4))
	}
	if p.Len() != 6 {
		t.Fatalf("nine taps typed %d digits on a pad of 6", p.Len())
	}
}

// A keyboard's digits, Backspace and Enter work the pad, from the number row and the number pad.
func TestAPinPadTakesAKeyboard(t *testing.T) {
	p, w, g, run := pinStage(t)
	var got string
	p.OnCommit = func(pin string, _ *gunim.UI) gunim.Intent { got = pin; return nil }
	g.ui.Focus(p)
	for _, k := range []input.Key{input.Key2, input.KeyKP0, input.Key8, input.KeyBackspace, input.Key6} {
		key(w, run, k, 0)
	}
	key(w, run, input.KeyKPEnter, 0)
	if got != "206" {
		t.Fatalf("the keys handed over %q, want 206", got)
	}
}

// The digits typed never reach a screen reader: it hears how many there are, and a part for each key.
func TestAPinPadKeepsItsDigitsFromAScreenReader(t *testing.T) {
	p, w, _, run := pinStage(t)
	for _, d := range []int{9, 7, 3, 1} {
		tapKey(w, run, p, pinKey(d))
	}
	info := p.Access()
	heard := make([]string, 0, 3+2*len(info.Parts))
	heard = append(heard, info.Name, info.Value, info.Description)
	for _, part := range info.Parts {
		heard = append(heard, part.Value, part.Description)
	}
	all := strings.Join(heard, "|")
	if strings.Contains(all, "9731") || strings.Contains(all, "973") {
		t.Fatalf("a screen reader hears the PIN in %q", all)
	}
	if info.Value != "4/6" || len(info.Parts) != pinKeys {
		t.Fatalf("access value %q with %d parts, want 4/6 and a part for each of the %d keys", info.Value, len(info.Parts), pinKeys)
	}
	if info.Parts[pinEnter].Name != "Unlock" || info.Parts[pinDelete].Role != access.RoleButton {
		t.Fatalf("parts %+v, want buttons named by their keys", info.Parts)
	}
}

// After a wrong PIN the dots shake and settle, the pad takes no digits while the ring drains, and takes them again
// once it has.
func TestAPinPadWaitsAfterAWrongPin(t *testing.T) {
	p, w, _, run := pinStage(t)
	p.OnCommit = func(_ string, u *gunim.UI) gunim.Intent { p.SetWaiting(500*time.Millisecond, u); return nil }
	for _, d := range []int{1, 2, 3, 4} {
		tapKey(w, run, p, pinKey(d))
	}
	tapKey(w, run, p, pinEnter)
	if !p.Waiting() {
		t.Fatal("the pad does not wait after a wrong PIN")
	}
	moved, last := false, p.wait.Value()
	for i := range 20 {
		run(1)
		if s := p.shake.Value(); s < -1.5 || s > 1.5 {
			t.Fatalf("frame %d: the dots swing %v dots' widths, want at most 1.5", i, s)
		} else if s > 0.2 || s < -0.2 {
			moved = true
		}
		if v := p.wait.Value(); v > last+1e-4 {
			t.Fatalf("frame %d: the wait's ring grew from %v to %v", i, last, v)
		}
		last = p.wait.Value()
	}
	if !moved {
		t.Fatal("the dots never shook")
	}
	tapKey(w, run, p, pinKey(5))
	if p.Len() != 0 {
		t.Fatal("the pad took a digit while it waited")
	}
	run(40)
	if p.Waiting() {
		t.Fatal("the pad still waits once the wait is over")
	}
	if s := p.shake.Value(); s > 0.01 || s < -0.01 {
		t.Fatalf("the dots rest %v off their place", s)
	}
	tapKey(w, run, p, pinKey(5))
	if p.Len() != 1 {
		t.Fatal("the pad takes no digit once the wait is over")
	}
}

// A wait set before the pad is mounted holds it from the first frame.
func TestAPinPadWaitSetBeforeMountingHolds(t *testing.T) {
	p := NewPinPad(4, "OK")
	p.SetWaiting(time.Second, nil)
	w, _, run := touchStage(t, p, geom.Sz(360, 420))
	if !p.Waiting() || p.dim.Value() != 1 {
		t.Fatalf("waiting %v, keys dimmed %v; want a wait shown from the first frame", p.Waiting(), p.dim.Value())
	}
	tapKey(w, run, p, pinKey(1))
	if p.Len() != 0 {
		t.Fatal("the pad took a digit while it waited")
	}
	run(70)
	if p.Waiting() {
		t.Fatal("the wait set before mounting never ended")
	}
}

// stackStage mounts a stack of one labelled screen in a window 400 wide, and returns it with its window, UI and
// frames.
func stackStage(t *testing.T) (*Stack, *gunim.Window, *grab, func(int)) {
	t.Helper()
	s := NewStack(NewLabel("Home"))
	w, g, run := touchStage(t, s, geom.Sz(400, 300))
	return s, w, g, run
}

// A screen pushed slides in from the right on every frame, and popped slides back out and leaves the tree.
func TestAStackSlidesScreensInAndOut(t *testing.T) {
	s, _, g, run := stackStage(t)
	next := NewLabel("Plants")
	s.Push(next, g.ui)
	run(1)
	top := s.front()
	last := top.off.Value()
	for i := range 60 {
		run(1)
		v := top.off.Value()
		if v > last+1e-4 || v < -0.02 {
			t.Fatalf("frame %d: the screen coming in went from %v to %v", i, last, v)
		}
		last = v
	}
	if last > 0.001 || s.Top() != next {
		t.Fatalf("the screen pushed rests %v of the width off, top %v", last, s.Top())
	}
	if !s.Pop(g.ui) || s.Len() != 1 {
		t.Fatal("Pop left the stack with more than its first screen")
	}
	last = top.off.Value()
	for i := range 90 {
		run(1)
		v := top.off.Value()
		if v < last-1e-4 {
			t.Fatalf("frame %d: the screen going came back from %v to %v", i, last, v)
		}
		last = v
	}
	if _, ok := g.ui.Bounds(next); ok {
		t.Fatal("the screen popped is still in the tree once it has slid away")
	}
	if s.Pop(g.ui) {
		t.Fatal("Pop took the last screen")
	}
}

// swipe drags from x0 to x1 across the stack in steps over d, with a finger or the mouse.
func swipe(w *gunim.Window, run func(int), x0, x1 float32, steps int, d time.Duration, touch bool) {
	t0 := time.Now()
	y := float32(150)
	if touch {
		w.Input(input.PointerMove{Pos: geom.Pt(x0, y), Touch: true, Time: t0})
	}
	w.Input(input.PointerDown{Pos: geom.Pt(x0, y), Button: input.ButtonPrimary, Clicks: 1, Touch: touch, Time: t0})
	run(1)
	for i := 1; i <= steps; i++ {
		x := x0 + (x1-x0)*float32(i)/float32(steps)
		w.Input(input.PointerMove{Pos: geom.Pt(x, y), Touch: touch, Time: t0.Add(d * time.Duration(i) / time.Duration(steps))})
		run(1)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(x1, y), Button: input.ButtonPrimary, Touch: touch, Time: t0.Add(d + time.Millisecond)})
	if touch {
		w.Input(input.PointerLeave{Time: t0.Add(d + time.Millisecond)})
	}
}

// A slow swipe from the edge less than a third of the way springs back; one past a third goes back and runs OnPop.
func TestASwipeFromTheEdgeGoesBackPastAThird(t *testing.T) {
	for _, touch := range []bool{true, false} {
		s, w, g, run := stackStage(t)
		s.OnPop = Sends(pressed{5})
		s.Push(NewLabel("Plants"), g.ui)
		run(60)
		top := s.front()
		swipe(w, run, 6, 106, 10, time.Second, touch)
		run(60)
		if s.Len() != 2 || top.off.Value() > 0.01 {
			t.Fatalf("touch %v: a slow swipe a quarter of the way left %d screens, the top %v off", touch, s.Len(), top.off.Value())
		}
		if got := sent(w); len(got) != 0 {
			t.Fatalf("touch %v: a swipe that sprang back sent %v", touch, got)
		}
		swipe(w, run, 6, 226, 10, time.Second, touch)
		run(1)
		if s.Len() != 1 {
			t.Fatalf("touch %v: a slow swipe past half way left %d screens", touch, s.Len())
		}
		if got := sent(w); len(got) != 1 || got[0] != (pressed{5}) {
			t.Fatalf("touch %v: going back by a swipe sent %v, want OnPop's intent", touch, got)
		}
		run(90)
	}
}

// A quick flick from the edge goes back though it went a short way, and the screen goes on at the finger's speed.
func TestAFlickFromTheEdgeGoesBack(t *testing.T) {
	s, w, g, run := stackStage(t)
	s.Push(NewLabel("Plants"), g.ui)
	run(60)
	top := s.front()
	swipe(w, run, 6, 86, 4, 60*time.Millisecond, true)
	before := top.off.Value()
	run(1)
	if s.Len() != 1 {
		t.Fatalf("a quick flick a fifth of the way left %d screens", s.Len())
	}
	if top.off.Value()-before < 0.02 {
		t.Fatalf("the flicked screen moved %v of the width in a frame, want it going on at speed", top.off.Value()-before)
	}
}

// A drag that starts away from the edge moves no screen.
func TestADragAwayFromTheEdgeLeavesTheStack(t *testing.T) {
	s, w, g, run := stackStage(t)
	s.Push(NewLabel("Plants"), g.ui)
	run(60)
	swipe(w, run, 120, 380, 10, 300*time.Millisecond, false)
	run(30)
	if s.Len() != 2 || s.front().off.Value() != 0 {
		t.Fatalf("a drag from the middle left %d screens, the top %v off", s.Len(), s.front().off.Value())
	}
}

// Alt+Left goes back, and a screen reader's close does too.
func TestAStackGoesBackFromKeysAndAScreenReader(t *testing.T) {
	s, w, g, run := stackStage(t)
	b := NewButton("Bed one")
	s.Push(b, g.ui)
	s.Push(NewLabel("Deeper"), g.ui)
	run(60)
	g.ui.Focus(b)
	key(w, run, input.KeyLeft, input.ModAlt)
	if s.Len() != 2 {
		t.Fatalf("Alt+Left left %d screens, want 2", s.Len())
	}
	if info := s.Access(); len(info.Actions) != 1 || info.Actions[0] != access.ActionClose {
		t.Fatalf("access %+v, want an action that goes back", info)
	}
	if !s.AccessAct(access.Request{Part: -1, Action: access.ActionClose}, g.ui) || s.Len() != 1 {
		t.Fatal("a screen reader's close did not go back")
	}
	run(60)
	if len(s.Access().Actions) != 0 {
		t.Fatal("with one screen left the stack still offers to go back")
	}
}

// keyboardStage mounts a text field over a keyboard, and returns them with the window, UI and frames.
func keyboardStage(t *testing.T) (*Keyboard, *TextField, *gunim.Window, *grab, func(int)) {
	t.Helper()
	f := NewTextField()
	k := NewKeyboard(Column(f))
	w, g, run := touchStage(t, k, geom.Sz(600, 400))
	return k, f, w, g, run
}

// kbIndex returns the index of the key that types s, or of the first key of kind.
func kbIndex(t *testing.T, k *Keyboard, s string, kind keyKind) int {
	t.Helper()
	for i, key := range k.keys {
		if key.kind == kind && (kind != keyChar || key.text == s) {
			return i
		}
	}
	t.Fatalf("no key %q of kind %d", s, kind)
	return -1
}

// tapKb taps the keyboard's key i with a finger.
func tapKb(w *gunim.Window, run func(int), k *Keyboard, i int) {
	c := k.rects[i].Center()
	touchDown(w, c)
	run(1)
	touchUp(w, c)
	run(1)
}

// The keyboard slides up on every frame as a field takes the focus, and down as it leaves.
func TestAKeyboardSlidesUpForAField(t *testing.T) {
	k, f, _, g, run := keyboardStage(t)
	if k.Shown() || k.top != 400 {
		t.Fatalf("with nothing focused the keyboard shows, its top at %v", k.top)
	}
	g.ui.Focus(f)
	last := k.top
	for i := range 60 {
		run(1)
		if k.top > last+0.01 {
			t.Fatalf("frame %d: the keyboard went down from %v to %v as it opened", i, last, k.top)
		}
		last = k.top
	}
	if !k.Shown() || last > 400-k.height+0.5 {
		t.Fatalf("open, the keyboard's top is at %v, want %v", last, 400-k.height)
	}
	for i, r := range k.rects {
		if r.Min.X < 0 || r.Max.X > 600 || r.Max.Y > 400 {
			t.Fatalf("key %d lies at %v, outside the keyboard's 600 by 400", i, r)
		}
		if r.Size().H < KeyboardKey.Get(theme.NewLive(Dark().With(Touch...))) {
			t.Fatalf("key %d is %v tall", i, r.Size().H)
		}
	}
	g.ui.Focus(nil)
	run(60)
	if k.Shown() || k.top < 399.5 {
		t.Fatalf("with the focus gone the keyboard's top is at %v", k.top)
	}
}

// Keys type into the focused field through its text edits: Shift makes a capital for one letter, Delete takes the
// last character off, and the second page types digits.
func TestAKeyboardTypesIntoTheField(t *testing.T) {
	k, f, w, g, run := keyboardStage(t)
	var committed string
	f.OnCommit = func(s string, _ *gunim.UI) gunim.Intent { committed = s; return nil }
	g.ui.Focus(f)
	run(60)
	tapKb(w, run, k, kbIndex(t, k, "", keyShift))
	for _, s := range []string{"h", "i", "x"} {
		tapKb(w, run, k, kbIndex(t, k, s, keyChar))
	}
	tapKb(w, run, k, kbIndex(t, k, "", keyDelete))
	tapKb(w, run, k, kbIndex(t, k, " ", keySpace))
	tapKb(w, run, k, kbIndex(t, k, "", keyPage))
	tapKb(w, run, k, kbIndex(t, k, "4", keyChar))
	tapKb(w, run, k, kbIndex(t, k, "2", keyChar))
	if got := f.Text(); got != "Hi 42" {
		t.Fatalf("the field holds %q, want %q", got, "Hi 42")
	}
	if g.ui.Focused() != gunim.Node(f) {
		t.Fatal("typing on the keyboard took the focus from the field")
	}
	tapKb(w, run, k, kbIndex(t, k, "", keyEnter))
	if committed != "Hi 42" {
		t.Fatalf("Enter committed %q, want the field's text", committed)
	}
}

// The hide key takes the keyboard down, and the focus with it.
func TestAKeyboardHides(t *testing.T) {
	k, f, w, g, run := keyboardStage(t)
	g.ui.Focus(f)
	run(60)
	tapKb(w, run, k, kbIndex(t, k, "", keyHide))
	run(60)
	if k.Shown() || g.ui.Focused() != nil {
		t.Fatalf("after the hide key the keyboard shows %v, the focus is on %v", k.Shown(), g.ui.Focused())
	}
}

// A screen reader hears a part for each key while the keyboard is up, named for what it does, and presses them.
func TestAKeyboardSpeaksToAScreenReader(t *testing.T) {
	k, f, _, g, run := keyboardStage(t)
	if parts := k.Access().Parts; len(parts) != 0 {
		t.Fatalf("down, the keyboard offers %d keys", len(parts))
	}
	g.ui.Focus(f)
	run(60)
	info := k.Access()
	if len(info.Parts) != len(k.keys) {
		t.Fatalf("up, the keyboard offers %d keys, want %d", len(info.Parts), len(k.keys))
	}
	names := map[string]bool{}
	for _, p := range info.Parts {
		names[p.Name] = true
	}
	for _, want := range []string{"q", "Shift", "Delete", "Enter", "Space", "Hide keyboard"} {
		if !names[want] {
			t.Errorf("no key is named %q", want)
		}
	}
	if !k.AccessAct(access.Request{Part: kbIndex(t, k, "q", keyChar), Action: access.ActionPress}, g.ui) {
		t.Fatal("a screen reader's press on q was refused")
	}
	run(1)
	if f.Text() != "q" {
		t.Fatalf("a screen reader's press on q typed %q", f.Text())
	}
}

// The kit's widgets draw inside the box they were laid out at, with a focus ring's room round it, while they move:
// a hold half full, a PIN pad shaking, a keyboard part way up, and a screen sliding into a stack.
func TestTheTouchKitDrawsInsideItsBox(t *testing.T) {
	hold := NewHoldButton("Revoke laptop")
	hold.Icon = nil
	pad := NewPinPad(6, "Unlock")
	f := NewTextField()
	kb := NewKeyboard(Column(f))
	st := NewStack(NewLabel("Home"))
	w, g, run := touchStage(t, Column(Row(hold), pad, NewSized(kb, 600, 320), NewSized(st, 600, 100)), geom.Sz(600, 1000))
	st.Push(NewLabel("Plants"), g.ui)
	th := theme.NewLive(Dark().With(Touch...))
	touchDown(w, geom.Pt(20, 20))
	pad.OnCommit = func(_ string, u *gunim.UI) gunim.Intent { pad.SetWaiting(time.Second, u); return nil }
	g.ui.Focus(f)
	for i := range 40 {
		run(1)
		if i == 5 {
			pad.press(0, g.ui)
			pad.press(pinEnter, g.ui)
		}
		for name, n := range map[string]gunim.Node{"hold button": hold, "PIN pad": pad, "keyboard": kb, "stack": st} {
			b, ok := g.ui.Bounds(n)
			if !ok {
				t.Fatalf("the %s is not laid out", name)
			}
			box := b.Size()
			if out := spills(paintedIn(n, box, th), box, 3); len(out) > 0 {
				t.Fatalf("frame %d: the %s draws outside its box %v: %v", i, name, box, out)
			}
		}
	}
	touchUp(w, geom.Pt(20, 20))
	run(1)
}
