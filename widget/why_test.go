package widget

import (
	"image/color"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

const why = "Works only with the Echo up"

// heldFamily returns the shared family and the other controls of the base that a program greys.
func heldFamily() []member {
	rg := NewRadioGroup(RadioItem{Label: "Low"}, RadioItem{Label: "High"})
	ss, hb := NewStatusSwitch("Sync"), NewHoldButton("Erase")
	cb := NewColorButton(color.NRGBA{R: 0xff, A: 0xff})
	return append(family(), member{"radio group", rg, &rg.Control}, member{"status switch", ss, &ss.Control},
		member{"hold button", hb, &hb.Control}, member{"colour button", cb, &cb.Control})
}

// shownTip is the text of the tooltip c shows, "" where it shows none.
func shownTip(c *Control) string {
	if c.tip.card == nil {
		return ""
	}
	return c.tip.card.text
}

// A disabled control with a reason shows the reason alone as its tooltip; with none it shows its tooltip; an
// enabled one shows its tooltip whatever reason it holds.
func TestADisabledControlsTooltipIsItsReason(t *testing.T) {
	for _, c := range []struct {
		disabled bool
		why      string
		want     func(m member) string
	}{
		{false, why, func(m member) string { return m.c.Tooltip }},
		{true, why, func(member) string { return why }},
		{true, "", func(m member) string { return m.c.Tooltip }},
	} {
		for _, m := range heldFamily() {
			m.c.Tooltip = "What " + m.name + " does"
			m.c.Disabled, m.c.Why = c.disabled, c.why
			w, run := stage(t, &frame{child: Column(m.node), size: geom.Sz(400, 300)})
			w.Input(input.PointerMove{Pos: geom.Pt(6, 6), Time: time.Now()})
			run(60)
			if got := shownTip(m.c); got != c.want(m) {
				t.Errorf("resting on the %s, disabled %v with reason %q, the tooltip says %q, want %q", m.name,
					c.disabled, c.why, got, c.want(m))
			}
		}
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

// A disabled control reads as disabled, offers nothing to do, keeps its name and value, and says its reason as its
// description; enabled, a reason changes nothing it says.
func TestAScreenReaderHearsWhyAControlIsDisabled(t *testing.T) {
	for _, m := range heldFamily() {
		a := m.node.(gunim.Accessible)
		before := a.Access()
		m.c.Why = why
		if got := a.Access(); !reflect.DeepEqual(got, before) {
			t.Errorf("enabled with a reason, the %s reads as %+v, want %+v as before", m.name, got, before)
		}
		for _, reason := range []string{why, ""} {
			m.c.Disabled, m.c.Why = true, reason
			got := a.Access()
			if !got.State.Has(access.StateDisabled) || got.Name != before.Name || got.Value != before.Value {
				t.Errorf("disabled, the %s reads as %q = %q, state %v; want %q = %q, disabled", m.name, got.Name,
					got.Value, got.State, before.Name, before.Value)
			}
			if acts := actions(got); len(acts) != 0 {
				t.Errorf("disabled, the %s offers %v", m.name, acts)
			}
			want := reason
			if want == "" {
				want = before.Description
			}
			if got.Description != want {
				t.Errorf("disabled with reason %q, the %s's description is %q, want %q", reason, m.name,
					got.Description, want)
			}
		}
		m.c.Disabled, m.c.Why = false, ""
	}
}

// The published tree carries a disabled control's reason, and follows it as it changes.
func TestTheTreeFollowsAReasonAsItChanges(t *testing.T) {
	b := NewButton("Save")
	b.Disabled, b.Why = true, why
	w, run := stage(t, &frame{child: Column(b), size: geom.Sz(400, 300)})
	w.Offscreen().ListenForAccess()
	described := func() string {
		t.Helper()
		run(2)
		var walk func(n *access.Node) *access.Node
		walk = func(n *access.Node) *access.Node {
			if n.Role == access.RoleButton && n.Name == "Save" {
				return n
			}
			for _, k := range n.Children {
				if f := walk(k); f != nil {
					return f
				}
			}
			return nil
		}
		tree := w.Offscreen().AccessTree()
		if tree == nil {
			t.Fatal("nothing published to a listening screen reader")
		}
		n := walk(tree.Root)
		if n == nil {
			t.Fatal("the tree holds no button Save")
		}
		if !n.State.Has(access.StateDisabled) || len(n.Actions) != 0 {
			t.Fatalf("the button reads as state %v with actions %v, want disabled with none", n.State, n.Actions)
		}
		return n.Description
	}
	if d := described(); d != why {
		t.Fatalf("the tree describes the button as %q, want %q", d, why)
	}
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, s setWhy, u *gunim.UI) {
		b.Why = s.Why
		u.Invalidate()
	})
	if err := w.Client().Patch("stage", setWhy{"Go to the song's start"}); err != nil {
		t.Fatal(err)
	}
	if d := described(); d != "Go to the song's start" {
		t.Fatalf("after the reason changed, the tree describes the button as %q", d)
	}
}

type setWhy struct{ Why string }

// A tooltip that shows turns over to a disabled control's new reason, with the pointer still on it.
func TestATooltipFollowsAReasonAsItChanges(t *testing.T) {
	for _, m := range heldFamily() {
		m.c.Tooltip = "What " + m.name + " does"
		m.c.Disabled, m.c.Why = true, why
		w, run := stage(t, &frame{child: Column(m.node), size: geom.Sz(400, 300)})
		gunim.RegisterPatch(w, "stage", func(_ gunim.Node, s setWhy, u *gunim.UI) {
			m.c.Why = s.Why
			u.Invalidate()
		})
		w.Input(input.PointerMove{Pos: geom.Pt(6, 6), Time: time.Now()})
		run(60)
		if got := shownTip(m.c); got != why {
			t.Fatalf("resting on the disabled %s, the tooltip says %q, want %q", m.name, got, why)
		}
		for _, next := range []string{"Go to the song's start", ""} {
			if err := w.Client().Patch("stage", setWhy{next}); err != nil {
				t.Fatal(err)
			}
			run(2)
			want := next
			if want == "" {
				want = m.c.Tooltip
			}
			if got := shownTip(m.c); got != want {
				t.Errorf("the %s's reason changed to %q under the pointer, and its tooltip says %q, want %q",
					m.name, next, got, want)
			}
		}
	}
}

// listen sets every On… callback of n to one that counts its calls and sends an intent, so a test sees any act.
func listen(n gunim.Node, calls *int) {
	v := reflect.ValueOf(n).Elem()
	intent := reflect.TypeFor[gunim.Intent]()
	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		if f.Kind() != reflect.Func || !sf.IsExported() || len(sf.Name) < 3 || sf.Name[:2] != "On" {
			continue
		}
		ft := f.Type()
		f.Set(reflect.MakeFunc(ft, func([]reflect.Value) []reflect.Value {
			*calls++
			out := make([]reflect.Value, ft.NumOut())
			for j := range out {
				out[j] = reflect.Zero(ft.Out(j))
				if ft.Out(j) == intent {
					out[j] = reflect.ValueOf(gunim.Intent(acted{}))
				}
			}
			return out
		}))
	}
}

type acted struct{}

// A disabled control, with or without a reason, takes no press, double click, wheel, key, typing or screen reader's
// act, even with the keyboard on it as it turned disabled.
func TestADisabledControlTakesNothing(t *testing.T) {
	for _, reason := range []string{why, ""} {
		for _, m := range heldFamily() {
			calls := 0
			listen(m.node, &calls)
			w, run, disable := familyStage(t, []member{m})
			tab(w, run, 0)
			disable(true)
			m.c.Why = reason
			run(2)
			sent(w)
			calls = 0
			click(w, 6, 6)
			doubleClick(w, 6, 6)
			w.Input(input.Scroll{Pos: geom.Pt(6, 6), Delta: geom.Pt(0, 40), Notches: geom.Pt(0, 1)})
			for _, k := range []input.Key{input.KeySpace, input.KeyEnter, input.KeyUp, input.KeyRight, input.KeyEnd,
				input.KeyBackspace} {
				w.Input(input.KeyPress{Key: k})
			}
			w.Input(input.TextInput{Text: "x"})
			run(30)
			if a, ok := m.node.(gunim.AccessActor); ok {
				u := stageUI(t, w, run)
				for _, r := range []access.Request{{Action: access.ActionPress}, {Action: access.ActionPress, Part: 1},
					{SetValue: true, Value: 5}} {
					if a.AccessAct(r, u) {
						t.Errorf("the disabled %s (reason %q) took a screen reader's %+v", m.name, reason, r)
					}
				}
			}
			run(2)
			if n := len(sent(w)); n != 0 || calls != 0 {
				t.Errorf("the disabled %s (reason %q) sent %d intents and ran its callbacks %d times", m.name, reason,
					n, calls)
			}
		}
	}
}

// A screen reader's request to focus a disabled control leaves the keyboard where it was.
func TestAScreenReaderCannotFocusADisabledControl(t *testing.T) {
	for _, m := range heldFamily() {
		m.c.Disabled, m.c.Why = true, why
		w, run := stage(t, &frame{child: Column(m.node), size: geom.Sz(400, 300)})
		w.Offscreen().ListenForAccess()
		run(2)
		var held *access.Node
		var walk func(n *access.Node)
		walk = func(n *access.Node) {
			if held == nil && n.State.Has(access.StateDisabled) && n.Description == why {
				held = n
			}
			for _, k := range n.Children {
				walk(k)
			}
		}
		walk(w.Offscreen().AccessTree().Root)
		if held == nil {
			t.Fatalf("the disabled %s is not in the tree", m.name)
		}
		focused := focusProbe(t, w, run)
		w.Input(access.Request{ID: held.ID, Focus: true})
		run(2)
		if f := focused(); f != nil {
			t.Errorf("a screen reader focused the disabled %s, and the keyboard is on %T", m.name, f)
		}
	}
}
