package calendar

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
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
	d.OnOpen = func(id string, _ geom.Rect, _ *gunim.UI) gunim.Intent { opened = id; return nil }
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

// frame gives its one child size, which a test can change, and lays it out only once size has some area.
type frame struct {
	child gunim.Node
	size  geom.Size
}

func (f *frame) Children() []gunim.Node { return []gunim.Node{f.child} }

func (f *frame) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	if f.size.W > 0 && f.size.H > 0 {
		kids.At(0).Layout(gunim.Tight(f.size))
		kids.At(0).Place(geom.Point{})
	}
	return c.Max
}

func (f *frame) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	if f.size.W > 0 && f.size.H > 0 {
		kids.At(0).Paint(p)
	}
}

// awayAllWeek returns n events lasting the whole week shown by newWeek.
func awayAllWeek(n int) []Event {
	out := make([]Event, 0, n)
	for i := range n {
		out = append(out, Event{ID: "all" + strconv.Itoa(i), Title: "Away", Start: monday,
			End: monday.Add(7 * 24 * time.Hour), AllDay: true})
	}
	return out
}

func TestTheGridNeverScrollsPastTheEndOfTheDay(t *testing.T) {
	d := newWeek()
	d.events = append(d.events, awayAllWeek(2)...)
	fr := &frame{child: d, size: geom.Sz(800, 600)}
	w, run, _ := stage(t, fr)
	check := func(when string) {
		t.Helper()
		if end := d.timeY(24 * time.Hour); end < fr.size.H-0.5 {
			t.Fatalf("%s: midnight is drawn at %v, over %v of blank", when, end, fr.size.H-end)
		}
		if top := d.timeY(0); top > d.bodyTop()+0.5 {
			t.Fatalf("%s: the day starts at %v, under the top of the hours at %v", when, top, d.bodyTop())
		}
	}
	// To the bottom, with the gliding scroll half way there.
	w.Input(input.Scroll{Pos: geom.Pt(400, 400), Delta: geom.Pt(0, -2000), Time: time.Now()})
	run(3)
	steps := []struct {
		name string
		do   func()
	}{
		{"the row of whole days gone", func() {
			withUI(t, w, func(u *gunim.UI) { d.SetEvents(d.events[:1], u) })
		}},
		{"the window taller", func() { fr.size = geom.Sz(800, 700) }},
		{"the window shorter, then taller still", func() { fr.size = geom.Sz(800, 400) }},
		{"the window tallest", func() { fr.size = geom.Sz(800, 900) }},
	}
	for _, s := range steps {
		s.do()
		for f := range 40 {
			run(1)
			check(s.name + ", frame " + strconv.Itoa(f))
		}
		w.Input(input.Scroll{Pos: geom.Pt(400, 500), Delta: geom.Pt(0, -2000), Time: time.Now()})
		run(2)
	}
}

func TestScrollToHourBeforeTheFirstLayoutStartsThere(t *testing.T) {
	d := newWeek()
	fr := &frame{child: d}
	w, run, _ := stage(t, fr)
	withUI(t, w, func(u *gunim.UI) { d.ScrollToHour(13, u) })
	run(1)
	fr.size = geom.Sz(800, 600)
	for f := range 30 {
		run(1)
		if top := d.scrollY() / d.hour(); top != 13 {
			t.Fatalf("frame %d: the grid shows %v hours at its top, want 13", f, top)
		}
	}
}

func TestTheEventUnderARestingPointerLiftsAsTheGridScrolls(t *testing.T) {
	d := newWeek()
	w, run, _ := stage(t, d)
	at := point(d, 1, 10*time.Hour+30*time.Minute)
	w.Input(input.PointerMove{Pos: at})
	run(20)
	if d.hover != "a" {
		t.Fatalf("the pointer rests on the event and lifts %q", d.hover)
	}
	check := func(when string) {
		t.Helper()
		under, _ := d.EventAt(at)
		if d.hover != under {
			t.Fatalf("%s: %q lifts under the pointer, which is over %q", when, d.hover, under)
		}
		for _, s := range d.sprites {
			if up := s.lift.Target() > 0; up != (s.e.ID == under) {
				t.Fatalf("%s: %s lifts to %v with %q under the pointer", when, s.e.ID, s.lift.Target(), under)
			}
		}
	}
	// Away down the day, then back up half way, the lift follows what is under the pointer.
	for _, by := range []float32{-300, 150} {
		w.Input(input.Scroll{Pos: at, Delta: geom.Pt(0, by), Time: time.Now()})
		for f := range 40 {
			run(1)
			check("scrolled " + strconv.Itoa(int(by)) + ", frame " + strconv.Itoa(f))
		}
	}
}

func TestAClickActsOnceAsThePrimaryButtonLetsGo(t *testing.T) {
	d := newWeek()
	opened := 0
	d.OnOpen = func(string, geom.Rect, *gunim.UI) gunim.Intent { opened++; return nil }
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
