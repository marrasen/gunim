package calendar

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
)

// A day or a month says it takes Escape while an event is chosen, for a phone's Back.
func TestAViewWithAnEventChosenTakesEscape(t *testing.T) {
	day := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	ev := []Event{{ID: "a", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)}}
	d, m := NewDays(day, 7), NewMonth(day)
	d.SetEvents(ev, nil)
	m.SetEvents(ev, nil)
	for name, v := range map[string]interface {
		gunim.EscapeTaker
		SetSelected(id string, u *gunim.UI)
	}{"days": d, "month": m} {
		if v.TakesEscape(nil) {
			t.Errorf("%s with none chosen takes Escape", name)
		}
		v.SetSelected("a", nil)
		if !v.TakesEscape(nil) {
			t.Errorf("%s with one chosen takes no Escape", name)
		}
	}
	m.SetSelected("", nil)
	m.drag = &monthDrag{}
	if !m.TakesEscape(nil) {
		t.Error("a month with an event dragged takes no Escape")
	}
}
