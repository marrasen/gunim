package calendar

import (
	"cmp"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// stop is an event the keys can move to: its ID, the day it shows on, counted from the first shown, and when it
// starts.
type stop struct {
	id    string
	day   int
	start time.Time
	box   geom.Rect
}

// nextStop returns the event the key k moves to from the one with id: Up and Down go to the one before or after
// it on its day, and Left and Right to the one nearest its time on the nearest day either side that has one. With
// id empty, any of them goes to the first event. It reports false for a key that moves nowhere.
func nextStop(stops []stop, id string, k input.Key) (stop, bool) {
	if len(stops) == 0 {
		return stop{}, false
	}
	slices.SortStableFunc(stops, func(a, b stop) int {
		if a.day != b.day {
			return cmp.Compare(a.day, b.day)
		}
		return a.start.Compare(b.start)
	})
	at := slices.IndexFunc(stops, func(s stop) bool { return s.id == id })
	if at < 0 {
		return stops[0], true
	}
	cur := stops[at]
	switch k {
	case input.KeyUp:
		if at > 0 && stops[at-1].day == cur.day {
			return stops[at-1], true
		}
	case input.KeyDown:
		if at+1 < len(stops) && stops[at+1].day == cur.day {
			return stops[at+1], true
		}
	case input.KeyLeft, input.KeyRight:
		dir := map[input.Key]int{input.KeyLeft: -1, input.KeyRight: 1}[k]
		best, found := stop{}, false
		for day := cur.day + dir; day >= stops[0].day && day <= stops[len(stops)-1].day && !found; day += dir {
			for _, s := range stops {
				if s.day != day {
					continue
				}
				if !found || absDur(dayOffset(s.start)-dayOffset(cur.start)) < absDur(dayOffset(best.start)-dayOffset(cur.start)) {
					best, found = s, true
				}
			}
		}
		return best, found
	}
	return stop{}, false
}

// tap is a press on something that acts once the pointer lets go over it: the thing's box, and what it does.
type tap struct {
	box geom.Rect
	do  func()
}

// press holds do, to run when the pointer lets go within box.
func (t *tap) press(box geom.Rect, do func()) { t.box, t.do = box, do }

// release lets go at pt, running what the press held when pt is within its box. It reports whether a press was
// held.
func (t *tap) release(pt geom.Point) bool {
	do := t.do
	t.do = nil
	if do == nil {
		return false
	}
	if t.box.Contains(pt) {
		do()
	}
	return true
}

// swipe turns sideways scrolling, or scrolling with Shift held, into steps to the days either side: one step for
// each stretch of it, and none again until the scrolling pauses.
type swipe struct {
	sum  float32
	last time.Time
	done bool
}

// swipeStep is how far sideways scrolling goes before it steps.
const swipeStep = 120

// step takes in a scroll, along with vertical when it is the vertical scroll that steps, and returns the step it
// makes: -1, 1, or 0 for none.
func (s *swipe) step(e input.Scroll, vertical bool) int {
	if e.Time.Sub(s.last) > 250*time.Millisecond {
		s.sum, s.done = 0, false
	}
	s.last = e.Time
	d := e.Delta.X
	if vertical || e.Mods.Has(input.ModShift) {
		d = e.Delta.X + e.Delta.Y
	}
	s.sum += d
	if s.done || abs(s.sum) < swipeStep {
		return 0
	}
	s.done = true
	if s.sum > 0 {
		return -1
	}
	return 1
}

// eventKeys acts on a key for a view that lets the keys move between its events: the arrows move the choice,
// Enter opens the event chosen, Delete deletes it, and Escape lets it go. It reports whether it took the key.
func eventKeys(k input.KeyPress, u *gunim.UI, n gunim.Node, stops []stop, selected string, sel func(id string),
	open func(id string, box geom.Rect, u *gunim.UI) gunim.Intent, onDelete func(id string, u *gunim.UI) gunim.Intent,
) bool {
	switch k.Key {
	case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight:
		if selected == "" && (k.Key == input.KeyLeft || k.Key == input.KeyRight) {
			// With nothing chosen, left and right are the window's, which step the days.
			return false
		}
		if s, ok := nextStop(stops, selected, k.Key); ok {
			sel(s.id)
		}
		return true
	case input.KeyEnter, input.KeyKPEnter:
		if selected == "" || open == nil {
			return false
		}
		for _, s := range stops {
			if s.id == selected {
				send(u, n, open(s.id, s.box, u))
			}
		}
		return true
	case input.KeyDelete, input.KeyBackspace:
		if selected == "" || onDelete == nil {
			return false
		}
		send(u, n, onDelete(selected, u))
		return true
	case input.KeyEscape:
		if selected == "" {
			return false
		}
		sel("")
		return true
	}
	return false
}
