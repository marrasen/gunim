package plugui

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// count is how many times what appears in log.
func count(log []string, what string) int {
	n := 0
	for _, s := range log {
		if s == what {
			n++
		}
	}
	return n
}

// pass moves the pointer across the window and back with no button down.
func pass(w *gunim.Window) {
	for _, y := range []float32{90, 50, 10, 50, 90} {
		w.Input(input.PointerMove{Pos: geom.Pt(50, y), Time: time.Now()})
		frames(w, 1)
	}
}

// A knob locked in the middle of a drag ends the drag at once: one end, and no more sets, then or after.
func TestAKnobLockedMidDragEndsTheDrag(t *testing.T) {
	h := &host{values: map[uint32]float64{3: 100}}
	k := NewKnob(h, release)
	w := stageNode(t, k)
	w.Input(input.PointerMove{Pos: geom.Pt(50, 50), Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: geom.Pt(50, 30), Time: time.Now()})
	frames(w, 1)
	sets := count(h.log, "set")
	if count(h.log, "begin") != 1 || sets == 0 {
		t.Fatalf("the drag sent %v, want a begin and sets", h.log)
	}
	k.Locked = true
	frames(w, 1)
	if count(h.log, "end") != 1 || k.held {
		t.Fatalf("a frame after it was locked mid-drag, the knob sent %v and is held %v, want the gesture ended",
			h.log, k.held)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(50, 10), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(50, 10), Button: input.ButtonPrimary, Time: time.Now()})
	frames(w, 1)
	k.Locked = false
	frames(w, 1)
	pass(w)
	if !slices.Equal(h.log[len(h.log)-1:], []string{"end"}) || count(h.log, "set") != sets ||
		count(h.log, "begin") != 1 || count(h.log, "end") != 1 {
		t.Fatalf("after it was locked mid-drag, let go and passed over freed, the knob sent %v", h.log)
	}
}

// A knob locked in the middle of a run of the wheel ends the run at once: one end, and no more sets.
func TestAKnobLockedMidWheelEndsTheRun(t *testing.T) {
	h := &host{values: map[uint32]float64{3: 100}}
	k := NewKnob(h, release)
	w := stageNode(t, k)
	w.Input(input.PointerMove{Pos: geom.Pt(50, 50), Time: time.Now()})
	w.Input(input.Scroll{Pos: geom.Pt(50, 50), Delta: geom.Pt(0, 40), Notches: geom.Pt(0, 1)})
	frames(w, 1)
	sets := count(h.log, "set")
	if count(h.log, "begin") != 1 || sets == 0 || count(h.log, "end") != 0 {
		t.Fatalf("a notch of the wheel sent %v, want a begin and a set, the run still open", h.log)
	}
	k.Locked = true
	frames(w, 1)
	if count(h.log, "end") != 1 {
		t.Fatalf("a frame after it was locked mid-run, the knob sent %v, want the run ended", h.log)
	}
	w.Input(input.Scroll{Pos: geom.Pt(50, 50), Delta: geom.Pt(0, 40), Notches: geom.Pt(0, 1)})
	frames(w, 60)
	k.Locked = false
	frames(w, 1)
	pass(w)
	if count(h.log, "set") != sets || count(h.log, "begin") != 1 || count(h.log, "end") != 1 {
		t.Fatalf("after it was locked mid-run, the knob sent %v", h.log)
	}
}

// A selector locked while a lamp is pressed lets go of the press, and picks nothing as it is let go or after.
func TestASelectorLockedMidPressLetsGo(t *testing.T) {
	picked := 0
	s := NewSelector("Stereo", []string{"Left", "Both"}, nil)
	s.OnPick = func(int, *gunim.UI) { picked++ }
	w := stageNode(t, s)
	at := s.cells[1].Center()
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	frames(w, 1)
	s.Locked = true
	frames(w, 1)
	if s.held || s.press.Target() != 0 {
		t.Fatalf("a frame after it was locked mid-press, the selector is held %v, pressed in to %v", s.held,
			s.press.Target())
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	frames(w, 1)
	s.Locked = false
	frames(w, 1)
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	pass(w)
	if picked != 0 || s.Selected() != 0 {
		t.Fatalf("locked mid-press, the selector picked %d times, and stands at %d", picked, s.Selected())
	}
}

// An action dial disabled while pressed lets go of the press, and clicks on no release after.
func TestAnActionDialDisabledMidPressLetsGo(t *testing.T) {
	clicked := 0
	d := NewActionDial("Learn", nil)
	d.OnClick = func(*gunim.UI) { clicked++ }
	w := stageNode(t, d)
	at := geom.Pt(50, 50)
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	frames(w, 1)
	d.Disabled = true
	frames(w, 1)
	if d.held || d.squash.Target() != 0 {
		t.Fatalf("a frame after it was disabled mid-press, the dial is held %v, squashed to %v", d.held,
			d.squash.Target())
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	frames(w, 1)
	d.Disabled = false
	frames(w, 1)
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	pass(w)
	if clicked != 0 {
		t.Fatalf("disabled mid-press, the dial clicked %d times", clicked)
	}
}

// A power lamp disabled while pressed switches on no release after, nor as the pointer passes once it is enabled.
func TestAPowerLampDisabledMidPressSwitchesNothing(t *testing.T) {
	switched := 0
	p := NewPower("gate")
	p.OnChange = func(bool, *gunim.UI) gunim.Intent { switched++; return nil }
	w := stageNode(t, p)
	at := geom.Pt(50, 50)
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	frames(w, 1)
	p.Disabled = true
	frames(w, 1)
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	frames(w, 1)
	p.Disabled = false
	frames(w, 1)
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	pass(w)
	if switched != 0 || p.held || p.Checked() {
		t.Fatalf("disabled mid-press, the lamp switched %d times, held %v, on %v", switched, p.held, p.Checked())
	}
}

// faded is how faint n paints itself into box: one less the opacity of the first layer it opens, or 0.
func faded(n gunim.Node, box geom.Size) float32 {
	var p paint.Painter
	n.Paint(&p, gunim.Frame{Scale: 1, Theme: theme.NewLive(widget.Dark())}, box, gunim.Children{})
	for _, op := range p.Ops() {
		if l, ok := op.(*paint.LayerOp); ok && !l.Opts.Clip {
			return 1 - l.Opts.Opacity
		}
	}
	return 0
}

// A selector and an action dial laid out greyed start grey; greyed after that, they fade frame by frame, and back.
func TestASelectorAndADialGlideToGrey(t *testing.T) {
	sel := NewSelector("Stereo", []string{"Left", "Both"}, nil)
	d := NewActionDial("Learn", nil)
	for _, c := range []struct {
		name string
		node gunim.Node
		grey func(bool)
	}{
		{"selector", sel, func(on bool) { sel.Locked = on }},
		{"action dial", d, func(on bool) { d.Disabled = on }},
	} {
		c.grey(true)
		w := stageNode(t, c.node)
		box := geom.Sz(100, 100)
		full := faded(c.node, box)
		if full < 0.3 {
			t.Fatalf("laid out greyed, the %s paints %v faint", c.name, full)
		}
		c.grey(false)
		frames(w, 40)
		if f := faded(c.node, box); f > 0.001 {
			t.Fatalf("freed, the %s settles %v faint", c.name, f)
		}
		for _, on := range []bool{true, false} {
			c.grey(on)
			last := map[bool]float32{true: 0, false: full}[on]
			for f := range 40 {
				frames(w, 1)
				got := faded(c.node, box)
				// A spring may bounce a little past where it rests.
				if (on && got < last-0.02) || (!on && got > last+0.02) || (f == 1 && (got < 0.01 || got > full-0.01)) {
					t.Fatalf("frame %d after greyed %v, the %s went from %v to %v faint", f+1, on, c.name, last, got)
				}
				last = got
			}
		}
	}
}

// A dial's reason that goes for a frame and comes back under a resting pointer shows again after the delay.
func TestADialsReasonThatComesBackShowsAgain(t *testing.T) {
	d := NewActionDial("Learn", nil)
	d.Disabled, d.Why = true, why
	w := stageNode(t, d)
	w.Input(input.PointerMove{Pos: geom.Pt(50, 50), Time: time.Now()})
	frames(w, 60)
	if got := shownTip(w); got != why {
		t.Fatalf("resting on the disabled dial, the tooltip says %q", got)
	}
	d.Disabled = false
	frames(w, 1)
	d.Disabled = true
	frames(w, 2)
	if got := shownTip(w); got != "" {
		t.Fatalf("enabled for a frame, the dial's tooltip still says %q", got)
	}
	frames(w, 60)
	if got := shownTip(w); got != why {
		t.Fatalf("disabled again under the resting pointer, the dial's tooltip says %q, want %q", got, why)
	}
}
