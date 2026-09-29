package calendar

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 9, 29, h, m, 0, 0, time.UTC) }

func TestLanesKeepOverlappingEventsReadable(t *testing.T) {
	type ev struct{ from, to [2]int }
	cases := []struct {
		name string
		evs  []ev
		want []placed
	}{
		{"one alone takes the whole column", []ev{{[2]int{9, 0}, [2]int{10, 0}}},
			[]placed{{0, 0, 0, 1, 1}}},
		{"one after another each take it whole", []ev{{[2]int{9, 0}, [2]int{10, 0}}, {[2]int{10, 0}, [2]int{11, 0}}},
			[]placed{{0, 0, 0, 1, 1}, {1, 0, 0, 1, 1}}},
		{"two starting close together sit side by side", []ev{{[2]int{9, 0}, [2]int{11, 0}}, {[2]int{9, 15}, [2]int{12, 0}}},
			[]placed{{0, 0, 0, 1, 2}, {1, 0, 1, 1, 2}}},
		{"a later one sits on top, indented", []ev{{[2]int{9, 0}, [2]int{11, 0}}, {[2]int{10, 0}, [2]int{12, 0}}},
			[]placed{{0, 0, 0, 1, 1}, {1, 1, 0, 1, 1}}},
		{"each later one indents a step further", []ev{
			{[2]int{9, 0}, [2]int{13, 0}}, {[2]int{10, 0}, [2]int{12, 0}}, {[2]int{11, 0}, [2]int{11, 30}}},
			[]placed{{0, 0, 0, 1, 1}, {1, 1, 0, 1, 1}, {2, 2, 0, 1, 1}}},
		{"two close together on top share the indent", []ev{
			{[2]int{9, 0}, [2]int{12, 0}}, {[2]int{10, 0}, [2]int{11, 0}}, {[2]int{10, 15}, [2]int{11, 0}}},
			[]placed{{0, 0, 0, 1, 1}, {1, 1, 0, 1, 2}, {2, 1, 1, 1, 2}}},
	}
	for _, c := range cases {
		var starts, ends []time.Time
		for _, e := range c.evs {
			starts = append(starts, at(e.from[0], e.from[1]))
			ends = append(ends, at(e.to[0], e.to[1]))
		}
		got, order := lanes(starts, ends)
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: event %d at %+v, want %+v", c.name, i, got[i], c.want[i])
			}
		}
		for k := 1; k < len(order); k++ {
			if starts[order[k]].Before(starts[order[k-1]]) {
				t.Errorf("%s: drawn in the order %v, want the earliest first", c.name, order)
			}
		}
	}
}

func TestDaysCountByTheCalendarAcrossAClockChange(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Skip("no time zone data")
	}
	sat := time.Date(2026, 10, 24, 0, 0, 0, 0, loc)
	if got := AddDays(sat, 2); got.Day() != 26 || got.Hour() != 0 {
		t.Fatalf("two days after Saturday is %v, want Monday at midnight", got)
	}
	if got := WeekStart(time.Date(2026, 10, 25, 15, 0, 0, 0, loc), time.Monday); got.Day() != 19 {
		t.Fatalf("the week of Sunday the 25th starts %v, want Monday the 19th", got)
	}
}

func TestParseClockReadsTheWaysPeopleWriteATime(t *testing.T) {
	ok := map[string]time.Duration{
		"9": 9 * time.Hour, "09": 9 * time.Hour, "930": 9*time.Hour + 30*time.Minute, "0930": 9*time.Hour + 30*time.Minute,
		"9:30": 9*time.Hour + 30*time.Minute, "9.30": 9*time.Hour + 30*time.Minute, " 23:59 ": 23*time.Hour + 59*time.Minute,
		"0:00": 0,
	}
	for s, want := range ok {
		if got, good := ParseClock(s); !good || got != want {
			t.Errorf("%q reads as %v, %v; want %v", s, got, good, want)
		}
	}
	for _, s := range []string{"", "24", "9:3", "9:60", "abc", "12345", ":30"} {
		if got, good := ParseClock(s); good {
			t.Errorf("%q reads as %v, want no time", s, got)
		}
	}
}
