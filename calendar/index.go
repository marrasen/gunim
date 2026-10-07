package calendar

import (
	"slices"
	"time"
)

// shortSpan is the longest an event an index files by its day may last; the index keeps longer ones apart.
const shortSpan = 7 * 24 * time.Hour

// index finds the events that take up some of a span of time, looking only at those that start near it. It is
// built for one slice of events, and serves until the views are given another.
type index struct {
	src []Event
	// byDay holds the events lasting up to shortSpan by the day they start on, counted in whole days of Unix time,
	// each day's in the order given; long holds the rest.
	byDay map[int64][]int
	long  []int
	// byID finds an event by its ID, once something has asked.
	byID map[string]int
}

// unixDay returns the day of Unix time t falls on.
func unixDay(t time.Time) int64 {
	s := t.Unix()
	d := s / 86400
	if s%86400 < 0 {
		d--
	}
	return d
}

// indexOf returns x when it was built for evs, and a new index for evs otherwise.
func indexOf(x *index, evs []Event) *index {
	if x != nil && len(x.src) == len(evs) && (len(evs) == 0 || &x.src[0] == &evs[0]) {
		return x
	}
	x = &index{src: evs, byDay: map[int64][]int{}}
	for i, e := range evs {
		if e.End.Sub(e.Start) > shortSpan {
			x.long = append(x.long, i)
			continue
		}
		d := unixDay(e.Start)
		x.byDay[d] = append(x.byDay[d], i)
	}
	return x
}

// within returns the events that take up some of the time between from and to, by their place in the slice the
// index was built for, in that order.
func (x *index) within(from, to time.Time) []int {
	var out []int
	// An event filed by its day starts at most shortSpan before from.
	for d := unixDay(from.Add(-shortSpan)); d <= unixDay(to); d++ {
		for _, i := range x.byDay[d] {
			if overlaps(x.src[i], from, to) {
				out = append(out, i)
			}
		}
	}
	for _, i := range x.long {
		if overlaps(x.src[i], from, to) {
			out = append(out, i)
		}
	}
	slices.Sort(out)
	return out
}

// find returns the place of the first event with id in the slice the index was built for, and false when none has
// it.
func (x *index) find(id string) (int, bool) {
	if x.byID == nil {
		x.byID = make(map[string]int, len(x.src))
		for i, e := range x.src {
			if _, ok := x.byID[e.ID]; !ok {
				x.byID[e.ID] = i
			}
		}
	}
	i, ok := x.byID[id]
	return i, ok
}

// overlaps reports whether e takes up some of the time between from and to, as [Event.covers] does for a day.
func overlaps(e Event, from, to time.Time) bool {
	if e.End.Equal(e.Start) {
		return !e.Start.Before(from) && e.Start.Before(to)
	}
	return e.Start.Before(to) && e.End.After(from)
}
