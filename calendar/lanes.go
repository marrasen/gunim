package calendar

import (
	"cmp"
	"slices"
	"time"
)

// placed is where an event sits across its day's column: from lane to lane+span of lanes lanes, each lane an equal
// part of the column's width.
type placed struct {
	i                 int
	lane, span, lanes int
}

// lanes sets side by side the events that overlap in time, given by their starts and ends. Events that overlap
// share a group, and each takes the leftmost lane free when it starts; it then widens to the right over lanes that
// stay free for all of its time. It returns one place for each event, in the order given.
func lanes(starts, ends []time.Time) []placed {
	n := len(starts)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if c := starts[a].Compare(starts[b]); c != 0 {
			return c
		}
		// The longer first, so it takes the lane on the left.
		return cmp.Compare(ends[b].Sub(starts[b]), ends[a].Sub(starts[a]))
	})
	out := make([]placed, n)
	var group []int
	var laneEnds []time.Time
	var groupEnd time.Time
	flush := func() {
		for _, i := range group {
			out[i].lanes = len(laneEnds)
			out[i].span = 1
			// Widen over the lanes to the right that nothing in the group uses while this event runs.
			for next := out[i].lane + 1; next < len(laneEnds); next++ {
				if slices.ContainsFunc(group, func(j int) bool {
					return out[j].lane == next && starts[j].Before(ends[i]) && ends[j].After(starts[i])
				}) {
					break
				}
				out[i].span++
			}
		}
		group, laneEnds = group[:0], laneEnds[:0]
	}
	for _, i := range order {
		start, end := starts[i], ends[i]
		if !end.After(start) {
			// A moment still takes a sliver of room.
			end = start.Add(time.Minute)
		}
		if len(group) > 0 && !start.Before(groupEnd) {
			flush()
		}
		lane := slices.IndexFunc(laneEnds, func(t time.Time) bool { return !t.After(start) })
		if lane < 0 {
			lane = len(laneEnds)
			laneEnds = append(laneEnds, end)
		} else {
			laneEnds[lane] = end
		}
		out[i] = placed{i: i, lane: lane}
		group = append(group, i)
		if end.After(groupEnd) || len(group) == 1 {
			groupEnd = end
		}
	}
	flush()
	return out
}
