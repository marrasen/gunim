package calendar

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// click presses and lets go at pt with the primary button, and draws a frame.
func click(w *gunim.Window, run func(int), pt geom.Point) {
	w.Input(input.PointerMove{Pos: pt})
	w.Input(input.PointerDown{Pos: pt, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: pt, Button: input.ButtonPrimary})
	run(1)
}

func TestManyAllDayEventsKeepToTheirRows(t *testing.T) {
	d := newWeek()
	const n = 30
	for i := range n {
		d.events = append(d.events, Event{ID: "all" + strconv.Itoa(i), Title: "Away", Start: monday,
			End: monday.Add(7 * 24 * time.Hour), AllDay: true})
	}
	var opened string
	d.Open = func(id string, _ geom.Rect, _ *gunim.UI) { opened = id }
	w, run, _ := stage(t, d)
	check := func(when string, most float32) {
		t.Helper()
		if d.bodyTop() > most {
			t.Fatalf("%s: the hours start at %v, want at most %v", when, d.bodyTop(), most)
		}
		shown := 0
		for _, k := range d.order {
			s := d.sprites[k]
			if s == nil || s.gone || !s.long {
				continue
			}
			shown++
			if r := s.rect.Value(); r.Min.Y < headerH || r.Max.Y > d.bodyTop() {
				t.Fatalf("%s: %s drawn at %v, outside the rows of whole days from %v to %v", when, s.e.ID, r, headerH,
					d.bodyTop())
			}
		}
		for i := range d.Count {
			if got := shown + d.longMore[i]; got != n {
				t.Fatalf("%s: day %d shows %d events of whole days and says %d more, want %d in all", when, i, shown,
					d.longMore[i], n)
			}
		}
	}
	shut := float32(headerH + 3*longRowH + 6)
	for f := range 30 {
		check("frame "+strconv.Itoa(f), shut)
		run(1)
	}
	// A bar that shows takes a click.
	bar := d.longBox(d.long[0][0])
	click(w, run, bar.Center())
	if opened != d.long[0][0].e.ID {
		t.Fatalf("a click on the first bar opened %q, want %q", opened, d.long[0][0].e.ID)
	}
	// A click on how many more opens the rows, up to half the grid.
	more, _ := d.moreRow()
	click(w, run, geom.Pt(d.colX(2)+20, more.Center().Y))
	if !d.longOpen {
		t.Fatal("a click on how many more left the rows shut")
	}
	for f := range 30 {
		check("opened, frame "+strconv.Itoa(f), headerH+(600-headerH)/2+6)
		run(1)
	}
	toggle, ok := d.longToggle()
	if !ok {
		t.Fatal("opened, the rows show no button to shut them")
	}
	click(w, run, toggle.Center())
	for f := range 30 {
		check("shut again, frame "+strconv.Itoa(f), shut)
		run(1)
	}
}
