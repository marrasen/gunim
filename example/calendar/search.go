package main

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/marrasen/gunim/calendar"
)

// maxFound is how many events a search answers with.
const maxFound = 40

// search returns the events whose title, place or notes hold every word of query: for each, the next time it
// happens from now, or the last time for one that is over. Those still to come go first, soonest first.
func (a *app) search(query string, now time.Time) []FoundEvent {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	type hit struct {
		e     FoundEvent
		start time.Time
		past  bool
	}
	var hits []hit
	from, to := now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)
	for _, s := range a.series {
		text := strings.ToLower(s.title + " " + s.location + " " + s.notes)
		if !all(text, words) {
			continue
		}
		c, ok := a.calendarOf(s.cal)
		if !ok || s.answer == NotGoing {
			continue
		}
		times := s.times(from, to)
		if len(times) == 0 {
			continue
		}
		pick := times[len(times)-1]
		for _, o := range times {
			if o.end.After(now) {
				pick = o
				break
			}
		}
		ev := calendar.Event{Start: pick.start, End: pick.end, AllDay: s.allDay}
		w := when(ev)
		if s.repeat != Never {
			w = repeatNames[s.repeat] + " · next " + w
		}
		hits = append(hits, hit{e: FoundEvent{ID: occurrenceID(s, pick.day), Title: s.title, When: w, Color: c.Color},
			start: pick.start, past: !pick.end.After(now)})
	}
	slices.SortStableFunc(hits, func(x, y hit) int {
		if x.past != y.past {
			return map[bool]int{false: -1, true: 1}[x.past]
		}
		if x.past {
			return y.start.Compare(x.start)
		}
		return cmp.Compare(x.start.UnixNano(), y.start.UnixNano())
	})
	out := make([]FoundEvent, 0, min(len(hits), maxFound))
	for _, h := range hits[:min(len(hits), maxFound)] {
		out = append(out, h.e)
	}
	return out
}

// all reports whether text holds every word.
func all(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// show moves the view to the day of the time id names, and reports whether it is still there.
func (a *app) show(id string) bool {
	s, day, ok := a.find(id)
	if !ok {
		return false
	}
	for _, o := range s.times(day, calendar.AddDays(day, 1)) {
		if o.day.Equal(day) {
			a.day = calendar.Day(o.start)
			return true
		}
	}
	a.day = day
	return true
}
