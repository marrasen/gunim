package calendar

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// within reports whether r lies inside box.
func within(r, box geom.Rect) bool {
	return r.Min.X >= box.Min.X && r.Min.Y >= box.Min.Y && r.Max.X <= box.Max.X && r.Max.Y <= box.Max.Y
}

// busyDay returns events for the day at cell 30 of the tests' month, Wednesday the 30th: all-day ones, then timed
// ones an hour apart from nine.
func busyDay(allDay, timed int) []Event {
	wed := monday.Add(2 * 24 * time.Hour)
	out := make([]Event, 0, allDay+timed)
	for i := range allDay {
		out = append(out, Event{ID: "all" + strconv.Itoa(i), Title: "All day", Start: wed, End: wed.Add(24 * time.Hour),
			AllDay: true})
	}
	for i := range timed {
		s := wed.Add(time.Duration(9+i) * time.Hour)
		out = append(out, Event{ID: "t" + strconv.Itoa(i), Title: "Meeting", Start: s, End: s.Add(30 * time.Minute)})
	}
	return out
}

// moreOn returns the line saying how many more cell has, and false when it has none.
func moreOn(m *Month, cell int) (chip, bool) {
	for _, ch := range m.more {
		if ch.cell == cell {
			return ch, true
		}
	}
	return chip{}, false
}

// drawnOnWednesday returns how many events show on Wednesday the 30th, as drawn.
func drawnOnWednesday(m *Month) int {
	const cell = 30
	n := 0
	for _, k := range m.order {
		if s := m.sprites[k]; s != nil && !s.gone && m.cell(cell).Contains(s.rect.Value().Center()) {
			n++
		}
	}
	return n
}

func TestAllDayBarsLeaveRoomToSayHowManyMore(t *testing.T) {
	m := NewMonth(monday)
	m.events = busyDay(2, 5)
	var listed time.Time
	m.More = func(day time.Time, _ geom.Rect, _ *gunim.UI) { listed = day }
	w, run, _ := stage(t, m)
	for f := range 20 {
		ch, ok := moreOn(m, 30)
		if !ok {
			t.Fatalf("frame %d: Wednesday shows %d of its 7 events and no line saying how many more", f, drawnOnWednesday(m))
		}
		if got := drawnOnWednesday(m) + ch.more; got != 7 {
			t.Fatalf("frame %d: Wednesday shows %d events and says %d more, want 7 in all", f, drawnOnWednesday(m), ch.more)
		}
		if !within(ch.box, m.cell(30)) {
			t.Fatalf("frame %d: the line saying how many more is at %v, outside its day at %v", f, ch.box, m.cell(30))
		}
		run(1)
	}
	ch, _ := moreOn(m, 30)
	w.Input(input.PointerDown{Pos: ch.box.Center(), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: ch.box.Center(), Button: input.ButtonPrimary})
	run(1)
	if !listed.Equal(m.day(30)) {
		t.Fatalf("a click on how many more listed %v, want Wednesday", listed)
	}
}

func TestAShortMonthKeepsEventsInTheirDays(t *testing.T) {
	for _, h := range []float32{120, 200, 250, 300, 330, 420} {
		m := NewMonth(monday)
		m.events = busyDay(1, 2)
		w, run, _ := stageAt(t, m, geom.Sz(800, h))
		for f := range 20 {
			for _, k := range m.order {
				s := m.sprites[k]
				if s == nil || s.gone {
					continue
				}
				r := s.rect.Value()
				c := m.cellAt(r.Center())
				// A bar may run across days; it keeps to its week's row of cells.
				row := geom.Rect{Min: geom.Pt(0, m.cell(c).Min.Y), Max: geom.Pt(800, m.cell(c).Max.Y)}
				if c < 0 || !within(r, row) {
					t.Fatalf("%v tall, frame %d: %s drawn at %v, outside its day at %v", h, f, s.e.ID, r, m.cell(c))
				}
			}
			if n := drawnOnWednesday(m); n < 3 {
				ch, ok := moreOn(m, 30)
				if !ok || ch.more+n != 3 || !within(ch.box, m.cell(30)) {
					t.Fatalf("%v tall, frame %d: Wednesday shows %d of 3 events, and says %d more at %v (shown %v)",
						h, f, n, ch.more, ch.box, ok)
				}
			}
			run(1)
		}
		// A click on the next week's day number picks that day.
		next := m.cell(37)
		var picked time.Time
		m.OnDay = func(day time.Time) gunim.Intent { picked = day; return nil }
		at := geom.Pt(next.Min.X+10, next.Min.Y+min(dayNumH, next.Size().H)/2)
		w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
		w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
		run(1)
		if !picked.Equal(m.day(37)) {
			t.Fatalf("%v tall: a click on the number of the day under Wednesday picked %v", h, picked)
		}
	}
}
