package calendar

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// The events' corners round as the theme says, in the week and in the month.
func TestEventsRoundAsTheThemeSays(t *testing.T) {
	th := theme.NewLive(theme.Make("test", theme.Set(EventRadius, 9), theme.Set(MonthEventRadius, 3)))
	month := NewMonth(monday)
	month.events = []Event{{ID: "a", Title: "Away", Start: monday, End: monday.Add(48 * time.Hour), AllDay: true}}
	for name, c := range map[string]struct {
		n    gunim.Node
		want float32
	}{"week": {newWeek(), 9}, "month": {month, 3}} {
		_, run, _ := stage(t, c.n)
		run(30)
		var p paint.Painter
		c.n.Paint(&p, gunim.Frame{Scale: 1, Theme: th}, geom.Sz(800, 600), gunim.Children{})
		found := false
		for _, op := range p.Ops() {
			if r, ok := op.(*paint.RRectOp); ok && r.Radius == c.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no event rounded %v, as the theme says", name, c.want)
		}
	}
}
