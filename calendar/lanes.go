package calendar

import (
	"cmp"
	"slices"
	"time"
)

// cascadeAfter is how much later than an event another must start to sit on top of it, indented, rather than
// beside it.
const cascadeAfter = 30 * time.Minute

// placed is where an event sits across its day's column: indented by indent steps, then from lane to lane+span of
// lanes lanes, each lane an equal part of the width the indent leaves.
type placed struct {
	i                 int
	indent            int
	lane, span, lanes int
}

// lanes places the events of a day's column, given by their starts and ends, so each can be read. Events that start
// within cascadeAfter of each other and overlap sit side by side, each widening over lanes left free for all of its
// time. An event that starts later than that, over one already placed, sits on top of it, indented a step further,
// and keeps most of the width. Each event counts as lasting at least least, the time its smallest box covers, so
// short events in a row sit side by side, each where it can be read. It returns one place for each event, in the
// order given, and the order to draw them in, earliest first, so a later event is drawn over the one it sits on.
func lanes(starts, ends []time.Time, least time.Duration) (places []placed, order []int) {
	n := len(starts)
	order = make([]int, n)
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
	least = max(least, time.Minute)
	end := func(i int) time.Time {
		// A moment, or an event shorter than its box, takes the room its box does.
		return maxTime(ends[i], starts[i].Add(least))
	}
	overlap := func(a, b int) bool { return starts[a].Before(end(b)) && end(a).After(starts[b]) }
	out := make([]placed, n)
	done := make([]bool, n)
	// Indent first: an event sits one step in from the deepest event it overlaps that started well before it.
	for _, i := range order {
		for _, j := range order {
			if j == i {
				break
			}
			if overlap(i, j) && starts[i].Sub(starts[j]) >= cascadeAfter {
				out[i].indent = max(out[i].indent, out[j].indent+1)
			}
		}
	}
	// Then side by side: events at one indent that overlap and start close together form a cluster sharing lanes.
	for _, i := range order {
		if done[i] {
			continue
		}
		cluster := []int{i}
		for k := 0; k < len(cluster); k++ {
			for _, j := range order {
				if done[j] || slices.Contains(cluster, j) || out[j].indent != out[i].indent {
					continue
				}
				c := cluster[k]
				if overlap(c, j) && absDur(starts[c].Sub(starts[j])) < cascadeAfter {
					cluster = append(cluster, j)
				}
			}
		}
		slices.SortStableFunc(cluster, func(a, b int) int {
			return slices.Index(order, a) - slices.Index(order, b)
		})
		var laneEnds []time.Time
		for _, j := range cluster {
			done[j] = true
			lane := slices.IndexFunc(laneEnds, func(t time.Time) bool { return !t.After(starts[j]) })
			if lane < 0 {
				lane = len(laneEnds)
				laneEnds = append(laneEnds, end(j))
			} else {
				laneEnds[lane] = end(j)
			}
			out[j].i, out[j].lane = j, lane
		}
		for _, j := range cluster {
			out[j].lanes, out[j].span = len(laneEnds), 1
			// Widen over the lanes to the right that nothing in the cluster uses while this event runs.
			for next := out[j].lane + 1; next < len(laneEnds); next++ {
				if slices.ContainsFunc(cluster, func(k int) bool { return out[k].lane == next && overlap(j, k) }) {
					break
				}
				out[j].span++
			}
		}
	}
	return out, order
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
