package plugui

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

const why = "A hit on Next preset plays Unit 7"

const how = "Drag up or down, or turn the wheel, with Shift for fine steps; double-click to reset"

// held is one control a program greys, as a test works it.
type held struct {
	name string
	node gunim.Node
	// grey greys the control or brings it back, with a reason; at is where a test rests the pointer on it.
	grey func(on bool, why string)
	at   func() geom.Point
	// tip is what its tooltip says enabled, and bare what it says greyed with no reason.
	tip, bare string
	// acted counts what it did for the user.
	acted *int
}

// helds returns one of each control of this package that a program greys.
func helds() []held {
	mid := func() geom.Point { return geom.Pt(50, 50) }
	acted := func() *int { n := 0; return &n }

	kn := acted()
	h := &host{values: map[uint32]float64{3: 100}}
	k := NewKnob(h, release)
	k.Tooltip = "How long it rings"
	k.OnTurn = func(float64, *gunim.UI) { *kn++ }

	sn := acted()
	sel := NewSelector("Stereo", []string{"Left", "Both"}, []string{"Only the left", "Both sides"})
	sel.OnPick = func(int, *gunim.UI) { *sn++ }

	pn := acted()
	pw := NewPower("gate")
	pw.OnChange = func(bool, *gunim.UI) gunim.Intent { *pn++; return nil }

	dn := acted()
	d := NewActionDial("Learn", nil)
	d.Tooltip = "Learn the voice's level"
	d.OnClick = func(*gunim.UI) { *dn++ }

	return []held{
		{"locked knob", k, func(on bool, w string) { k.Disabled, k.Locked, k.Why = on, on, w }, mid,
			"How long it rings. " + how, "How long it rings", kn},
		{"selector", sel, func(on bool, w string) { sel.Locked, sel.Why = on, w },
			func() geom.Point { return sel.cells[0].Center() }, "Left: Only the left", "Left: Only the left", sn},
		{"power lamp", pw, func(on bool, w string) { pw.Disabled, pw.Why = on, w },
			mid, "Gate: off. Click to turn on.", "Gate: off.", pn},
		{"action dial", d, func(on bool, w string) { d.Disabled, d.Why = on, w }, mid,
			"Learn the voice's level", "Learn the voice's level", dn},
	}
}

// shownTip is the text of the tooltip w shows, "" where it shows none.
func shownTip(w *gunim.Window) string {
	var find func(n *access.Node) string
	find = func(n *access.Node) string {
		if n.Role == access.RoleTooltip {
			return n.Name
		}
		for _, k := range n.Children {
			if s := find(k); s != "" {
				return s
			}
		}
		return ""
	}
	return find(w.Access().Root)
}

// rest mounts c, rests the pointer on it until its tooltip shows, and returns the window.
func rest(t *testing.T, c held) *gunim.Window {
	t.Helper()
	w := stageNode(t, c.node)
	w.Input(input.PointerMove{Pos: c.at(), Time: time.Now()})
	frames(w, 60)
	return w
}

// A greyed control with a reason shows the reason alone as its tooltip, and with none it shows its tooltip without
// how to work it; enabled, it says what it said before, to the letter.
func TestAGreyedControlsTooltipIsItsReason(t *testing.T) {
	for _, c := range []struct {
		on   bool
		why  string
		want func(held) string
	}{
		{false, why, func(c held) string { return c.tip }},
		{true, why, func(held) string { return why }},
		{true, "", func(c held) string { return c.bare }},
	} {
		for _, h := range helds() {
			h.grey(c.on, c.why)
			if got := shownTip(rest(t, h)); got != c.want(h) {
				t.Errorf("resting on the %s, greyed %v with reason %q, the tooltip says %q, want %q", h.name, c.on,
					c.why, got, c.want(h))
			}
		}
	}
}

// A knob greyed but still turning says its reason, and how to turn it.
func TestAGreyedKnobThatTurnsSaysItsReasonAndHowToTurnIt(t *testing.T) {
	k := NewKnob(&host{values: map[uint32]float64{3: 100}}, release)
	k.Tooltip, k.Disabled = "How long it rings", true
	if got := k.tooltip(); got != "How long it rings. "+how {
		t.Errorf("greyed with no reason, the knob says %q", got)
	}
	k.Why = why
	if got := k.tooltip(); got != why+". "+how {
		t.Errorf("greyed with a reason, the knob says %q", got)
	}
}

// actions lists the actions info and its parts offer.
func actions(info access.Info) []string {
	out := slices.Clone(info.Actions)
	for _, p := range info.Parts {
		out = append(out, actions(p)...)
	}
	return out
}

// A greyed control reads as disabled, offers nothing to do, keeps its name and value, and says its reason as its
// description; enabled, a reason changes nothing it says.
func TestAScreenReaderHearsWhyAControlIsGreyed(t *testing.T) {
	for _, h := range helds() {
		a := h.node.(gunim.Accessible)
		before := a.Access()
		h.grey(false, why)
		if got := a.Access(); !reflect.DeepEqual(got, before) {
			t.Errorf("enabled with a reason, the %s reads as %+v, want %+v as before", h.name, got, before)
		}
		for _, reason := range []string{why, ""} {
			h.grey(true, reason)
			got := a.Access()
			if !got.State.Has(access.StateDisabled) || got.Name != before.Name || got.Value != before.Value {
				t.Errorf("greyed, the %s reads as %q = %q, state %v; want %q = %q, disabled", h.name, got.Name,
					got.Value, got.State, before.Name, before.Value)
			}
			if acts := actions(got); len(acts) != 0 {
				t.Errorf("greyed, the %s offers %v", h.name, acts)
			}
			want := reason
			if want == "" {
				want = before.Description
			}
			if got.Description != want {
				t.Errorf("greyed with reason %q, the %s's description is %q, want %q", reason, h.name,
					got.Description, want)
			}
		}
	}
}

// A locked knob keeps its value for a screen reader, and reads as one whose value takes no change.
func TestALockedKnobReadsAsReadOnly(t *testing.T) {
	k := NewKnob(&host{values: map[uint32]float64{3: 100}}, release)
	if k.Access().State.Has(access.StateReadOnly) {
		t.Error("a knob that turns reads as read only")
	}
	k.Disabled = true
	if k.Access().State.Has(access.StateReadOnly) {
		t.Error("a knob greyed but turning reads as read only")
	}
	k.Locked = true
	if info := k.Access(); !info.State.Has(access.StateReadOnly) || info.Range == nil || info.Range.Value != 100 {
		t.Errorf("a locked knob reads as %+v, want read only at 100", info)
	}
}

// A tooltip that shows turns over to a greyed control's new reason, with the pointer still on it.
func TestATooltipFollowsAReasonAsItChanges(t *testing.T) {
	for _, h := range helds() {
		h.grey(true, why)
		w := rest(t, h)
		if got := shownTip(w); got != why {
			t.Fatalf("resting on the greyed %s, the tooltip says %q, want %q", h.name, got, why)
		}
		h.grey(true, "Go to the song's start")
		frames(w, 2)
		if got := shownTip(w); got != "Go to the song's start" {
			t.Errorf("the %s's reason changed under the pointer, and its tooltip says %q", h.name, got)
		}
	}
}

// A greyed control, with or without a reason, takes no press, double click, wheel, key or screen reader's act, even
// with the keyboard on it as it greyed.
func TestAGreyedControlTakesNothing(t *testing.T) {
	for _, reason := range []string{why, ""} {
		for _, h := range helds() {
			w := stageNode(t, h.node)
			w.Input(input.KeyPress{Key: input.KeyTab})
			frames(w, 2)
			h.grey(true, reason)
			frames(w, 2)
			at := h.at()
			for _, clicks := range []int{1, 2} {
				w.Input(input.PointerMove{Pos: at, Time: time.Now()})
				w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: clicks, Time: time.Now()})
				w.Input(input.PointerMove{Pos: at.Add(geom.Pt(0, -30)), Time: time.Now()})
				w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
				frames(w, 1)
			}
			w.Input(input.Scroll{Pos: at, Delta: geom.Pt(0, 40), Notches: geom.Pt(0, 1)})
			for _, k := range []input.Key{input.KeySpace, input.KeyEnter, input.KeyUp, input.KeyRight, input.KeyEnd} {
				w.Input(input.KeyPress{Key: k})
			}
			frames(w, 30)
			if a, ok := h.node.(gunim.AccessActor); ok {
				for _, r := range []access.Request{{Action: access.ActionPress}, {Action: access.ActionPress, Part: 1},
					{SetValue: true, Value: 500}} {
					if a.AccessAct(r, nil) {
						t.Errorf("the greyed %s (reason %q) took a screen reader's %+v", h.name, reason, r)
					}
				}
			}
			if *h.acted != 0 {
				t.Errorf("the greyed %s (reason %q) acted %d times", h.name, reason, *h.acted)
			}
		}
	}
}

// A power lamp laid out disabled starts faint; disabled after that, it fades frame by frame and lets go of a press.
func TestAPowerLampFadesAsItIsDisabled(t *testing.T) {
	p := NewPower("gate")
	p.Disabled = true
	w := stageNode(t, p)
	if got := p.grey.dim.Value(); got != 1 {
		t.Fatalf("laid out disabled, the lamp starts %v faint", got)
	}
	p.Disabled = false
	frames(w, 30)
	if got := p.grey.dim.Value(); got > 0.001 {
		t.Fatalf("enabled, the lamp settles %v faint", got)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(50, 50), Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	frames(w, 1)
	p.Disabled = true
	last := float32(0)
	for f := range 40 {
		frames(w, 1)
		got := p.grey.dim.Value()
		// A spring may bounce a little past where it rests.
		if got < last-0.02 || (f == 1 && (got <= 0.001 || got >= 0.999)) {
			t.Fatalf("frame %d after it was disabled, the lamp went from %v to %v faint", f+1, last, got)
		}
		last = got
	}
	if last < 0.99 || p.held || p.squash.Value() > 0.01 {
		t.Fatalf("settled disabled, the lamp is %v faint, held %v, pressed in %v", last, p.held, p.squash.Value())
	}
	w.Input(input.PointerUp{Pos: geom.Pt(50, 50), Button: input.ButtonPrimary, Time: time.Now()})
	frames(w, 1)
	if p.Checked() {
		t.Fatal("a press let go after the lamp was disabled switched it")
	}
	if p.Step(time.Second / 60) {
		t.Error("settled, the lamp still asks for frames")
	}
}

// A selector with no tips and a dial with no tooltip, freed while they show their reason, close the tooltip, and
// show none after the pointer leaves.
func TestAReasonsTooltipClosesAsTheControlIsFreed(t *testing.T) {
	sel := NewSelector("Stereo", []string{"Left", "Both"}, nil)
	d := NewActionDial("Learn", nil)
	for _, h := range []held{
		{"selector", sel, func(on bool, w string) { sel.Locked, sel.Why = on, w },
			func() geom.Point { return sel.cells[0].Center() }, "", "", nil},
		{"action dial", d, func(on bool, w string) { d.Disabled, d.Why = on, w },
			func() geom.Point { return geom.Pt(50, 50) }, "", "", nil},
	} {
		h.grey(true, why)
		w := rest(t, h)
		if got := shownTip(w); got != why {
			t.Fatalf("resting on the greyed %s, the tooltip says %q, want %q", h.name, got, why)
		}
		h.grey(false, "")
		frames(w, 30)
		if got := shownTip(w); got != "" {
			t.Errorf("freed under the pointer, the %s's tooltip still says %q", h.name, got)
		}
		w.Input(input.PointerMove{Pos: geom.Pt(400, 400), Time: time.Now()})
		w.Input(input.PointerLeave{Time: time.Now()})
		frames(w, 60)
		if got := shownTip(w); got != "" {
			t.Errorf("after the pointer left the freed %s, a tooltip says %q", h.name, got)
		}
	}
}

// A knob greyed but still turning reads as what it is: enabled and turning, with its reason as its description.
func TestAGreyedKnobThatTurnsReadsAsEnabled(t *testing.T) {
	k := NewKnob(&host{values: map[uint32]float64{3: 100}}, release)
	k.Disabled, k.Why = true, why
	info := k.Access()
	if info.State.Has(access.StateDisabled) || info.State.Has(access.StateReadOnly) || info.Description != why {
		t.Errorf("greyed but turning, the knob reads as state %v, described %q; want enabled, described %q",
			info.State, info.Description, why)
	}
	if !k.Focusable() || !k.AccessAct(access.Request{SetValue: true, Value: 500}, nil) {
		t.Error("greyed but turning, the knob takes no keyboard or screen reader's value")
	}
}

// A power lamp reads as a switch that can be checked, on or off, enabled or disabled, so a screen reader can read
// it and, while it is enabled, press it.
func TestAPowerLampReadsAsCheckable(t *testing.T) {
	p := NewPower("gate")
	for _, c := range []struct{ on, disabled bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		p.SetChecked(c.on, nil)
		p.Disabled = c.disabled
		info := p.Access()
		if !info.State.Has(access.StateCheckable) || info.State.Has(access.StateChecked) != c.on {
			t.Errorf("on %v, disabled %v, the lamp reads as state %v, want checkable and checked %v", c.on,
				c.disabled, info.State, c.on)
		}
	}
}
