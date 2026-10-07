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
)

// member is one control of the family, as a test works it.
type member struct {
	name string
	node gunim.Node
	c    *Control
}

// family returns one of each control that embeds the shared base, in a column.
func family() []member {
	b := NewButton("Save")
	ib := NewIconButton(nil, "Refresh")
	box, sw := NewCheckbox("Wrap"), NewSwitch("Wi-Fi")
	sl := NewSlider(0, 10)
	dd := NewDropdown([]MenuItem{{Label: "One"}, {Label: "Two"}})
	seg := NewSegmented("Day", "Week")
	mb := NewMenuButton("Sort", []MenuItem{{Label: "Name"}, {Label: "Date"}})
	l := NewLink("Show more")
	ch := NewChip("Type", "Image")
	tf, ta, nf := NewTextField(), NewTextArea(), NewNumberField(0, 10)
	ta.Rows = 2
	return []member{
		{"button", b, &b.Control}, {"icon button", ib, &ib.Control}, {"checkbox", box, &box.Control},
		{"switch", sw, &sw.Control}, {"slider", sl, &sl.Control}, {"drop-down", dd, &dd.Control},
		{"segmented", seg, &seg.Control}, {"menu button", mb, &mb.Control}, {"link", l, &l.Control},
		{"chip", ch, &ch.Control}, {"text field", tf, &tf.Control}, {"text area", ta, &ta.Control},
		{"number field", nf, &nf.Control},
	}
}

// familyStage lays out the family in a column, and returns what sets every member's Disabled through a patch, as an
// application's update would.
func familyStage(t *testing.T, ms []member) (w *gunim.Window, run func(int), disable func(bool)) {
	t.Helper()
	nodes := make([]gunim.Node, len(ms))
	for i, m := range ms {
		nodes[i] = m.node
	}
	w, run = stage(t, &frame{child: Column(nodes...), size: geom.Sz(400, 800)})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, d setDisabled, u *gunim.UI) {
		for _, m := range ms {
			m.c.Disabled = d.On
		}
		u.Invalidate()
	})
	disable = func(on bool) {
		t.Helper()
		if err := w.Client().Patch("stage", setDisabled{on}); err != nil {
			t.Fatal(err)
		}
	}
	return w, run, disable
}

type setDisabled struct{ On bool }

// opacity is how much of n shows as it paints itself into box: the opacity of the layer it opens first, or 1.
func opacity(n gunim.Node, box geom.Size) float32 {
	for _, op := range painted(n, box) {
		if l, ok := op.(*paint.LayerOp); ok && !l.Opts.Clip {
			return l.Opts.Opacity
		}
	}
	return 1
}

// Disabling a control fades it over frames, each fainter than the one before, and enabling it fades it back: none
// of them snaps.
func TestEveryControlFadesAsItIsDisabledAndBack(t *testing.T) {
	ms := family()
	w, run, disable := familyStage(t, ms)
	u := stageUI(t, w, run)
	box := func(m member) geom.Size {
		r, _ := u.Bounds(m.node)
		return r.Size()
	}
	for _, m := range ms {
		if o := opacity(m.node, box(m)); o != 1 {
			t.Fatalf("the enabled %s paints at opacity %v, want 1", m.name, o)
		}
	}
	for _, on := range []bool{true, false} {
		disable(on)
		last := map[string]float32{}
		for _, m := range ms {
			last[m.name] = map[bool]float32{true: 1, false: faintOpacity}[on]
		}
		for f := range 40 {
			run(1)
			for _, m := range ms {
				o := opacity(m.node, box(m))
				if (on && o > last[m.name]+1e-4) || (!on && o < last[m.name]-1e-4) {
					t.Fatalf("frame %d after Disabled=%v, the %s went from opacity %v to %v, the wrong way", f+1, on,
						m.name, last[m.name], o)
				}
				if f == 1 && (o <= faintOpacity+1e-3 || o >= 1-1e-3) {
					t.Fatalf("two frames after Disabled=%v, the %s paints at opacity %v, want part way", on, m.name, o)
				}
				last[m.name] = o
			}
		}
		for _, m := range ms {
			want := map[bool]float32{true: faintOpacity, false: 1}[on]
			if o := last[m.name]; o < want-1e-3 || o > want+1e-3 {
				t.Fatalf("settled with Disabled=%v, the %s paints at opacity %v, want %v", on, m.name, o, want)
			}
		}
	}
}

// A control disabled with the pointer on it lets its hover go, frame by frame; enabled again with the pointer still
// there, it lights again.
func TestAControlDisabledMidHoverFadesItsHoverAndLightsAgain(t *testing.T) {
	b, l, mb := NewButton("Save"), NewLink("More"), NewMenuButton("Sort", nil)
	ms := []member{{"button", b, &b.Control}, {"link", l, &l.Control}, {"menu button", mb, &mb.Control}}
	for _, m := range ms {
		w, run, disable := familyStage(t, []member{m})
		w.Input(input.PointerMove{Pos: geom.Pt(4, 4), Time: time.Now()})
		run(40)
		if h := m.c.hover.Value(); h < 0.99 {
			t.Fatalf("under the pointer the %s's hover is %v, want 1", m.name, h)
		}
		disable(true)
		last := float32(1)
		for f := range 40 {
			run(1)
			h := m.c.hover.Value()
			// A spring may bounce a little past where it rests.
			if h > last+0.02 || h < -0.1 || (f == 1 && h >= 1) {
				t.Fatalf("frame %d after disabling, the %s's hover went from %v to %v", f+1, m.name, last, h)
			}
			last = h
		}
		if last > 0.01 {
			t.Fatalf("disabled under the pointer, the %s's hover stays at %v", m.name, last)
		}
		disable(false)
		for f := range 40 {
			run(1)
			h := m.c.hover.Value()
			if h < last-0.02 || h > 1.1 {
				t.Fatalf("frame %d after enabling, the %s's hover went back from %v to %v", f+1, m.name, last, h)
			}
			last = h
		}
		if last < 0.99 {
			t.Fatalf("enabled again under the pointer, the %s's hover is %v, want 1", m.name, last)
		}
	}
}

// A control disabled with the keyboard on it fades its ring with it, and enabled again with the keyboard still there,
// its ring comes back; neither jumps.
func TestAControlDisabledWhileFocusedFadesItsRingAndBringsItBack(t *testing.T) {
	for _, m := range family() {
		w, run, disable := familyStage(t, []member{m})
		tab(w, run, 0)
		run(40)
		ring := func() float32 { return m.c.ring.Value() * (1 - min(max(m.c.dim.Value(), 0), 1)) }
		if m.name == "text field" || m.name == "text area" || m.name == "number field" {
			// A field shows the keyboard by its border, and draws no ring.
			continue
		}
		if r := ring(); r < 0.99 {
			t.Fatalf("Tab left the %s's ring at %v", m.name, r)
		}
		disable(true)
		last := float32(1)
		for f := range 40 {
			run(1)
			r := ring()
			if r > last+1e-4 || (f == 1 && (r >= 0.999 || r <= 0.001)) {
				t.Fatalf("frame %d after disabling, the focused %s's ring went from %v to %v", f+1, m.name, last, r)
			}
			last = r
		}
		disable(false)
		for f := range 40 {
			run(1)
			r := ring()
			if r < last-1e-4 || (f == 1 && r >= 0.999) {
				t.Fatalf("frame %d after enabling, the focused %s's ring went from %v to %v", f+1, m.name, last, r)
			}
			last = r
		}
		if last < 0.99 {
			t.Fatalf("enabled again with the keyboard on it, the %s's ring is %v, want 1", m.name, last)
		}
	}
}

// A disabled control takes no keyboard, lets a right click pass to what is round it, and keeps a click to itself.
func TestADisabledControlTakesNoFocusAndPassesARightClick(t *testing.T) {
	for _, m := range family() {
		rights, primaries := 0, 0
		fr := &frame{child: Column(m.node), size: geom.Sz(400, 300), handle: func(e input.Event, _ *gunim.UI) bool {
			if d, ok := e.(input.PointerDown); ok {
				if d.Button == input.ButtonSecondary {
					rights++
				} else {
					primaries++
				}
				return true
			}
			return false
		}}
		m.c.Disabled = true
		w, run := stage(t, fr)
		focused := focusProbe(t, w, run)
		tab(w, run, 0)
		if f := focused(); f != nil {
			t.Fatalf("Tab put the keyboard on %T past the disabled %s", f, m.name)
		}
		rightClick(w, 6, 6)
		click(w, 6, 6)
		run(1)
		if rights != 1 || primaries != 0 {
			t.Fatalf("on the disabled %s, a right click reached the frame %d times and a click %d, want 1 and 0",
				m.name, rights, primaries)
		}
		if n := len(sent(w)); n != 0 {
			t.Fatalf("the disabled %s sent %d intents", m.name, n)
		}
	}
}

// Every control says it is disabled to a screen reader.
func TestAScreenReaderHearsEveryDisabledControl(t *testing.T) {
	ms := family()
	w, run, disable := familyStage(t, ms)
	w.Offscreen().ListenForAccess()
	disable(true)
	run(2)
	tree := w.Offscreen().AccessTree()
	if tree == nil {
		t.Fatal("nothing published to a listening screen reader")
	}
	var disabled []string
	var walk func(n *access.Node)
	walk = func(n *access.Node) {
		if n.State&access.StateDisabled != 0 {
			disabled = append(disabled, n.Role.String())
		}
		for _, k := range n.Children {
			walk(k)
		}
	}
	walk(tree.Root)
	if len(disabled) < len(ms) {
		t.Fatalf("only %d nodes read as disabled (%s), want the %d controls", len(disabled),
			strings.Join(disabled, ", "), len(ms))
	}
}

// A slider is named by its label, or by the row it is in.
func TestASliderHasAName(t *testing.T) {
	s := NewSlider(0, 1)
	s.Label = "Volume"
	if n := s.Access().Name; n != "Volume" {
		t.Fatalf("a slider labelled Volume reads as %q", n)
	}
	r := NewSliderRow("Contrast", NewSlider(-1, 1))
	w, run := stage(t, &frame{child: r, size: geom.Sz(300, 28)})
	_ = w
	run(1)
	if n := r.Slider.Access().Name; n != "Contrast" {
		t.Fatalf("the slider of the row Contrast reads as %q", n)
	}
}

// Every control shows its tooltip once the pointer rests on it, disabled or not.
func TestEveryControlShowsItsTooltip(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		for _, m := range family() {
			m.c.Tooltip = "What " + m.name + " does"
			m.c.Disabled = disabled
			w, run := stage(t, &frame{child: Column(m.node), size: geom.Sz(400, 300)})
			w.Input(input.PointerMove{Pos: geom.Pt(6, 6), Time: time.Now()})
			run(60)
			if m.c.tip.popup == nil {
				t.Fatalf("resting on the %s (disabled %v), no tooltip showed", m.name, disabled)
			}
		}
	}
}
