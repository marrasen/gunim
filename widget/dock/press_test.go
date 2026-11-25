package dock

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// hosted is a window's content round a dock of a and b beside each other: with views, the views are the dock's, and
// else they lie beneath it, as a program's own. over lies over the dock where it is set, and card is where.
type hosted struct {
	d    *Dock
	own  map[string]*handles
	over gunim.Node
	card geom.Rect
}

// handles is a pane that takes every press, and drags by touch while drags says.
type handles struct {
	pane
	drags bool
}

func (*handles) Handle(e input.Event, _ *gunim.UI) bool {
	_, ok := e.(input.PointerDown)
	return ok
}

func (h *handles) DragsTouch() bool { return h.drags }

func (h *hosted) Children() []gunim.Node {
	var kids []gunim.Node
	if h.d.Content == nil {
		kids = append(kids, h.own["a"], h.own["b"])
	}
	kids = append(kids, h.d)
	if h.over != nil {
		kids = append(kids, h.over)
	}
	return kids
}

func (h *hosted) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for k := range kids.All {
		r := geom.Rect{Max: c.Max.Point()}
		switch {
		case k.Node() == h.over:
			r = h.card
		case k.Node() == gunim.Node(h.d):
		default:
			for id, v := range h.own {
				if k.Node() == gunim.Node(v) {
					r, _ = h.d.Rect(id)
				}
			}
		}
		k.Layout(gunim.Tight(r.Size()))
		k.Place(r.Min)
	}
	return c.Max
}

func (h *hosted) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// host shows a dock whose a is the pane the keys work in, and returns where b's middle is. With over, it lies over b.
func host(t *testing.T, content bool, over gunim.Node) (h *hosted, w *gunim.Window, run func(int), b geom.Point) {
	t.Helper()
	h = &hosted{d: NewDock(0), own: map[string]*handles{"a": {}, "b": {}}, over: over, card: geom.Rc(420, 100, 360, 300)}
	if content {
		h.d.Content = func(item string) gunim.Node { return h.own[item] }
	}
	l := NewLayout(true, "a")
	l.Add("b", Target{At: "a", Side: Right})
	w = gunimtest.New(t, geom.Sz(800, 500), nil)
	w.PaintWhenLooked(false)
	w.TimeByFrames(true)
	gunim.RegisterView(w, "v", func(Layout) *hosted { return h }, func(h *hosted, l Layout, u *gunim.UI) { h.d.SetLayout(l, u) })
	gunim.RegisterPatch(w, "v", func(_ *hosted, call uiCall, u *gunim.UI) { call(u) })
	if err := w.Client().Mount(gunim.Root, "v", "v", l, "v"); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(60)
	onUIDock(t, w, run, func(u *gunim.UI) { h.d.SetActive("a", u) })
	run(30)
	r, ok := h.d.Rect("b")
	if !ok || h.d.Active() != "a" {
		t.Fatalf("b shows %v, and the keys work in %s", ok, h.d.Active())
	}
	return h, w, run, r.Center()
}

// press presses and lets go at p with button b.
func press(w *gunim.Window, p geom.Point, b input.Button) {
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Button: b, Clicks: 1})
	w.Input(input.PointerUp{Pos: p, Button: b})
}

// TestAPressOverAPaneIsNotItsOwn: in a dialog, in a plain view over the dock and in a popup, with the views the dock's
// and the program's. A press that reaches the pane is.
func TestAPressOverAPaneIsNotItsOwn(t *testing.T) {
	for _, content := range []bool{true, false} {
		for _, button := range []input.Button{input.ButtonPrimary, input.ButtonSecondary, input.ButtonMiddle} {
			for name, over := range map[string]gunim.Node{"a dialog": widget.NewDialog("Sure?"), "a view": &handles{}, "a view that takes no press": &pane{}} {
				h, w, run, b := host(t, content, over)
				run(60)
				press(w, b, button)
				run(3)
				if got := h.d.Active(); got != "a" {
					t.Errorf("views in the dock %v, button %v: a press in %s over b has the keys work in %s", content, button, name, got)
				}
				// Beside what lies over it.
				if _, modal := over.(gunim.Modal); !modal {
					press(w, geom.Pt(b.X, 60), button)
					run(3)
					if got := h.d.Active(); got != "b" {
						t.Errorf("views in the dock %v, button %v: a press in b beside %s has the keys work in %s", content, button, name, got)
					}
				}
			}
			h, w, run, b := host(t, content, nil)
			press(w, b, button)
			if got := h.d.Active(); got != "b" {
				t.Errorf("views in the dock %v, button %v: a press in b has the keys work in %s", content, button, got)
			}
			// A popup of a's over b.
			var popup *gunim.Popup
			onUIDock(t, w, run, func(u *gunim.UI) {
				h.d.SetActive("a", u)
				popup = u.OpenPopup(h.own["a"], &handles{}, gunim.PopupOptions{Anchor: geom.Rc(0, 0, 10, 10), Max: geom.Sz(300, 300)})
			})
			run(5)
			popup.Input(input.PointerDown{Pos: geom.Pt(5, 5), Button: button, Clicks: 1})
			popup.Input(input.PointerUp{Pos: geom.Pt(5, 5), Button: button})
			run(3)
			if got := h.d.Active(); got != "a" {
				t.Errorf("views in the dock %v, button %v: a press in a popup has the keys work in %s", content, button, got)
			}
		}
	}
}

// TestAFingerThatScrollsLeavesThePaneTheKeysWorkIn: one that taps, that is held, or that drags what it pressed, has
// them work in the pane it pressed, and not before that shows.
func TestAFingerThatScrollsLeavesThePaneTheKeysWorkIn(t *testing.T) {
	for _, content := range []bool{true, false} {
		h, w, run, b := host(t, content, nil)
		at := time.Now()
		down := func() {
			w.Input(input.PointerMove{Pos: b, Touch: true, Time: at})
			w.Input(input.PointerDown{Pos: b, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at})
		}
		move := func(n int) {
			for i := range n {
				at = at.Add(16 * time.Millisecond)
				w.Input(input.PointerMove{Pos: b.Add(geom.Pt(0, -float32(i+1)*6)), Touch: true, Time: at})
				run(1)
			}
		}
		up := func(p geom.Point) {
			at = at.Add(16 * time.Millisecond)
			w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Touch: true, Time: at})
			w.Input(input.PointerLeave{Time: at})
			run(2)
		}
		again := func() {
			h.own["b"].drags = false
			at = at.Add(time.Second)
			run(60)
			onUIDock(t, w, run, func(u *gunim.UI) { h.d.SetActive("a", u) })
		}
		want := func(step, pane string) {
			t.Helper()
			if got := h.d.Active(); got != pane {
				t.Errorf("views in the dock %v, %s: the keys work in %s, want %s", content, step, got, pane)
			}
		}
		down()
		want("as a finger comes down", "a")
		run(2)
		move(8)
		want("as it scrolls", "a")
		up(b.Add(geom.Pt(0, -48)))
		run(60)
		want("as it lifts from a scroll", "a")

		again()
		down()
		up(input.Away)
		run(60)
		want("called off", "a")

		again()
		down()
		run(3)
		up(b)
		want("as it taps", "b")

		again()
		down()
		run(10)
		want("held a while", "a")
		run(30)
		want("held for long", "b")
		up(b)
		want("let go after that", "b")

		again()
		h.own["b"].drags = true
		down()
		want("on what drags", "a")
		move(4)
		want("as it drags", "b")
		up(b.Add(geom.Pt(0, -24)))
		want("as it lifts from a drag", "b")

		// One that goes on to scroll says nothing of the tap after it, elsewhere.
		again()
		down()
		move(8)
		up(b)
		run(60)
		r, _ := h.d.Rect("a")
		w.Input(input.PointerDown{Pos: r.Center(), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at})
		w.Input(input.PointerUp{Pos: r.Center(), Button: input.ButtonPrimary, Touch: true, Time: at.Add(time.Millisecond)})
		run(2)
		want("tapping a after it", "a")
	}
}

// TestAPaneGoneAsAFingerLiftsIsNotThePaneTheKeysWorkIn: taken out of the layout under the finger.
func TestAPaneGoneAsAFingerLiftsIsNotThePaneTheKeysWorkIn(t *testing.T) {
	h, w, run, b := host(t, true, nil)
	w.Input(input.PointerDown{Pos: b, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: time.Now()})
	onUIDock(t, w, run, func(u *gunim.UI) { h.d.SetLayout(NewLayout(true, "a"), u) })
	run(3)
	w.Input(input.PointerUp{Pos: b, Button: input.ButtonPrimary, Touch: true, Time: time.Now()})
	run(3)
	if h.d.active == "b" {
		t.Error("the keys work in a pane that is gone")
	}
}
