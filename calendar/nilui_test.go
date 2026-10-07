package calendar

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
)

// Every setter that takes the UI also takes nil, as a view does when it builds a view before mounting it.
func TestEverySetterTakesANilUI(t *testing.T) {
	var u *gunim.UI
	day := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	ev := []Event{{ID: "a", Title: "Stand-up", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)}}
	sets := map[string]func(){
		"MiniMonth.SetMarked": func() { NewMiniMonth(day).SetMarked(day, day.AddDate(0, 0, 3), u) },
		"DateField.SetValue":  func() { NewDateField(day).SetValue(day.AddDate(0, 0, 1), u) },
		"TimeField.SetValue":  func() { NewTimeField(time.Hour).SetValue(2*time.Hour, u) },
		"Days.SetEvents":      func() { NewDays(day, 7).SetEvents(ev, u) },
		"Days.SetDays":        func() { NewDays(day, 7).SetDays(day.AddDate(0, 0, 7), 7, u) },
		"Days.SetSelected":    func() { d := NewDays(day, 7); d.SetEvents(ev, u); d.SetSelected("a", u) },
		"Month.SetSelected":   func() { m := NewMonth(day); m.SetEvents(ev, u); m.SetSelected("a", u) },
		"Month.SetMonth":      func() { NewMonth(day).SetMonth(day.AddDate(0, 1, 0), u) },
		"Month.SetEvents":     func() { NewMonth(day).SetEvents(ev, u) },
	}
	for name, set := range sets {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked with a nil UI: %v", r)
				}
			}()
			set()
		})
	}
}
