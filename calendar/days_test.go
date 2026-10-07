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

func TestAGlidingEventTakesThePointerWhereItIsDrawn(t *testing.T) {
	d := newWeek()
	w, run, _ := stage(t, d)
	// Moved from ten on Tuesday to two, it glides there over some frames.
	ev := d.events[0]
	ev.Start, ev.End = monday.Add(24*time.Hour+14*time.Hour), monday.Add(24*time.Hour+15*time.Hour)
	withUI(t, w, func(u *gunim.UI) { d.SetEvents([]Event{ev}, u) })
	run(1)
	s := d.sprites[d.order[0]]
	gliding := 0
	for f := 0; s.rect.Active() && f < 120; f++ {
		drawn := d.onScreen(s, s.rect.Value())
		target := d.onScreen(s, s.rect.Target())
		if id, ok := d.EventAt(drawn.Center()); !ok || id != "a" {
			t.Fatalf("frame %d: the event drawn at %v takes no pointer at its middle", f, drawn)
		}
		if mid := target.Center(); !drawn.Contains(mid) {
			gliding++
			if _, ok := d.EventAt(mid); ok {
				t.Fatalf("frame %d: the pointer finds the event at %v, where it is going, while it is drawn at %v",
					f, mid, drawn)
			}
		}
		run(1)
	}
	if gliding == 0 {
		t.Fatal("the event never glided")
	}
}

func TestAClickActsOnceAsThePrimaryButtonLetsGo(t *testing.T) {
	d := newWeek()
	opened := 0
	d.Open = func(string, geom.Rect, *gunim.UI) { opened++ }
	w, run, sent := stage(t, d)
	at := point(d, 1, 10*time.Hour+20*time.Minute)
	// A secondary button let go while the primary holds the event does nothing.
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonSecondary})
	run(1)
	if opened != 0 {
		t.Fatal("the secondary button, let go, opened the event")
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(1)
	// The second press of a double click opens nothing more.
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(1)
	if opened != 1 {
		t.Fatalf("a double click opened the event %d times, want once", opened)
	}
	// A day's heading picks the day as the pointer lets go, once for a double click.
	head := geom.Pt(d.colX(4)+10, 20)
	w.Input(input.PointerDown{Pos: head, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	if got := sent(); len(got) != 0 {
		t.Fatalf("a press on a heading sent %v before the pointer let go", got)
	}
	w.Input(input.PointerUp{Pos: head, Button: input.ButtonPrimary})
	w.Input(input.PointerDown{Pos: head, Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: head, Button: input.ButtonPrimary})
	run(1)
	if got := sent(); len(got) != 1 {
		t.Fatalf("a double click on a heading sent %v, want one pick", got)
	}
	// Let go away from the heading, the press does nothing.
	w.Input(input.PointerDown{Pos: head, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(d.colX(6)+10, 20), Button: input.ButtonPrimary})
	run(1)
	if got := sent(); len(got) != 0 {
		t.Fatalf("a press let go over another heading sent %v", got)
	}
	// A double click on free time begins one event.
	free := point(d, 3, 15*time.Hour)
	w.Input(input.PointerDown{Pos: free, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: free, Button: input.ButtonPrimary})
	w.Input(input.PointerDown{Pos: free, Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: free, Button: input.ButtonPrimary})
	run(1)
	if got := sent(); len(got) != 1 {
		t.Fatalf("a double click on free time sent %v, want one new event", got)
	}
}
