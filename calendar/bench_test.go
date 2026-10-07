package calendar

import (
	"image/color"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// manyEvents returns n events over the two years from a year before monday: mostly an hour or less in the working
// day, some overlapping, and one in twenty lasting whole days.
func manyEvents(n int) []Event {
	out := make([]Event, 0, n)
	first := AddDays(monday, -365)
	for i := range n {
		day := AddDays(first, (i*7919)%730)
		e := Event{ID: "e" + strconv.Itoa(i), Title: "Event " + strconv.Itoa(i),
			Color: color.NRGBA{R: 0x4f, G: 0x8c, B: 0xf0, A: 0xff}}
		if i%20 == 0 {
			e.Start, e.End, e.AllDay = day, AddDays(day, 1+i%3), true
		} else {
			e.Start = day.Add(time.Duration(7*60+(i*37)%600) * time.Minute)
			e.End = e.Start.Add(time.Duration(15+(i*13)%90) * time.Minute)
		}
		out = append(out, e)
	}
	return out
}

func TestTheIndexFindsWhatAScanFinds(t *testing.T) {
	evs := manyEvents(3000)
	// Moments, events running over midnight, and some lasting weeks.
	for i := range 40 {
		s := AddDays(monday, i-20).Add(time.Duration(i) * 37 * time.Minute)
		evs = append(evs, Event{ID: "m" + strconv.Itoa(i), Start: s, End: s},
			Event{ID: "n" + strconv.Itoa(i), Start: s.Add(20 * time.Hour), End: s.Add(30 * time.Hour)},
			Event{ID: "w" + strconv.Itoa(i), Start: s, End: s.Add(time.Duration(i) * 24 * time.Hour)})
	}
	x := indexOf(nil, evs)
	for d := -60; d < 60; d += 3 {
		from := AddDays(monday, d)
		for _, n := range []int{1, 7, 42} {
			to := AddDays(from, n)
			var want []int
			for i, e := range evs {
				if overlaps(e, from, to) {
					want = append(want, i)
				}
			}
			if got := x.within(from, to); !slices.Equal(got, want) {
				t.Fatalf("%d days from %v: the index finds %d events, a scan %d", n, from, len(got), len(want))
			}
		}
	}
}

// benchLayout lays n out again and again, as each frame of an animation does, with the events set anew every
// fresh-th time, as an application's changes do.
func benchLayout(b *testing.B, n gunim.Node, set func([]Event, *gunim.UI), th func() gunim.Frame, fresh int) {
	evs := manyEvents(20000)
	w, run, _ := stage(b, n)
	withUI(b, w, func(u *gunim.UI) { set(evs, u) })
	run(2)
	var ui *gunim.UI
	withUI(b, w, func(u *gunim.UI) { ui = u })
	run(1)
	c := gunim.Tight(geom.Sz(800, 600))
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if fresh > 0 && i%fresh == 0 {
			set(evs, ui)
		}
		n.Layout(c, th(), gunim.Children{})
	}
}

func BenchmarkWeekLayout(b *testing.B) {
	for _, fresh := range []int{0, 1} {
		b.Run(map[int]string{0: "frames", 1: "new events"}[fresh], func(b *testing.B) {
			d := NewDays(monday, 7)
			benchLayout(b, d, d.SetEvents, func() gunim.Frame { return gunim.Frame{Theme: d.th} }, fresh)
		})
	}
}

func BenchmarkMonthLayout(b *testing.B) {
	for _, fresh := range []int{0, 1} {
		b.Run(map[int]string{0: "frames", 1: "new events"}[fresh], func(b *testing.B) {
			m := NewMonth(monday)
			var th gunim.Frame
			benchLayout(b, m, func(evs []Event, u *gunim.UI) { th.Theme = u.Theme(); m.SetEvents(evs, u) },
				func() gunim.Frame { return th }, fresh)
		})
	}
}
