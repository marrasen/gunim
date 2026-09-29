package main

import (
	"strings"
	"testing"
	"time"
)

func TestASteadyStreamStillShowsHeadings(t *testing.T) {
	c := &conv{ID: "c", byID: map[string]*msg{}}
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.Local)
	// One person, a message every 30 seconds for 20 minutes.
	for i := range 40 {
		m := &msg{ID: "m" + string(rune('a'+i)), Author: "Johan Ek", At: start.Add(time.Duration(i) * 30 * time.Second)}
		c.msgs = append(c.msgs, m)
		c.byID[m.ID] = m
	}
	var headed []time.Time
	for _, it := range timeline(c, start) {
		if it.Day == "" && !it.Continued {
			headed = append(headed, it.At)
		}
	}
	if len(headed) < 4 {
		t.Fatalf("%d headings in 20 minutes of one person's messages, want one at least every %v", len(headed), groupFor)
	}
	for i := 1; i < len(headed); i++ {
		if gap := headed[i].Sub(headed[i-1]); gap > groupFor {
			t.Fatalf("headings %v apart, want at most %v", gap, groupFor)
		}
	}
}

func TestAColleagueEditsAMessageOnce(t *testing.T) {
	h := newHarness(t)
	c := h.a.current
	who := c.people[0]
	m := h.a.add(c, who, "Build times:\n\n| a | b |\n|--|--|\n| 1 | 2 |", time.Now())
	for range 200 {
		h.a.colleague()
	}
	if n := strings.Count(m.Body, "\n\n"); n > 2 {
		t.Fatalf("the message was edited %d times, want once at most:\n%s", n-1, m.Body)
	}
	if m.Edited && !strings.Contains(m.Body, "|\n\n") {
		t.Fatalf("the edit did not start a paragraph of its own:\n%s", m.Body)
	}
}
