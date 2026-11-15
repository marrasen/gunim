package calendar

import (
	"reflect"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

const why = "Booked days cannot move"

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

// A date field and a time field disabled with a reason show it as their tooltip, and a screen reader reads it as
// their description; disabled, they offer nothing to do; enabled, the reason changes nothing.
func TestADisabledDayOrTimeFieldSaysWhy(t *testing.T) {
	df, tf := NewDateField(monday), NewTimeField(9*time.Hour)
	for _, c := range []struct {
		name string
		node gunim.Node
		set  func(on bool, why string)
	}{
		{"date field", df, func(on bool, w string) { df.Disabled, df.Why = on, w }},
		{"time field", tf, func(on bool, w string) { tf.Disabled, tf.Why = on, w }},
	} {
		a := c.node.(gunim.Accessible)
		before := a.Access()
		c.set(false, why)
		if got := a.Access(); !reflect.DeepEqual(got, before) {
			t.Errorf("enabled with a reason, the %s reads as %+v, want %+v as before", c.name, got, before)
		}
		c.set(true, why)
		info := a.Access()
		if !info.State.Has(access.StateDisabled) || len(info.Actions) != 0 || info.Description != why ||
			info.Name != before.Name || info.Value != before.Value {
			t.Errorf("disabled with a reason, the %s reads as %+v", c.name, info)
		}
		w, run, _ := stage(t, &frame{child: c.node, size: geom.Sz(200, 32)})
		w.Input(input.PointerMove{Pos: geom.Pt(20, 16), Time: time.Now()})
		run(60)
		if got := shownTip(w); got != why {
			t.Errorf("resting on the disabled %s, the tooltip says %q, want %q", c.name, got, why)
		}
		c.set(false, why)
		run(30)
		if got := shownTip(w); got != "" {
			t.Errorf("enabled again under the pointer, the %s shows a tooltip %q", c.name, got)
		}
	}
}
