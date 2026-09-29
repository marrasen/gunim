package calendar

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// monday is the first day the tests show.
var monday = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

// Intents the tests' views send.
type (
	drawn struct {
		start, end time.Time
		allDay     bool
	}
	changed struct {
		id         string
		start, end time.Time
	}
	picked struct{ day time.Time }
)

// stage shows n in an offscreen window 800 by 600, and returns it with a way to draw frames and the intents sent.
func stage(t *testing.T, n gunim.Node) (*gunim.Window, func(int), func() []gunim.Intent) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "v", func(struct{}) gunim.Node { return n }, nil)
	if err := w.Client().Mount(gunim.Root, "v", "v", nil); err != nil {
		t.Fatal(err)
	}
	run := func(k int) {
		for range k {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	sent := func() []gunim.Intent {
		var out []gunim.Intent
		for {
			select {
			case e := <-w.Client().Intents():
				out = append(out, e.Intent)
			default:
				return out
			}
		}
	}
	return w, run, sent
}

// drag presses at from, moves through to, and lets go there.
func drag(w *gunim.Window, run func(int), from, to geom.Point) {
	w.Input(input.PointerMove{Pos: from})
	w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1})
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		w.Input(input.PointerMove{Pos: geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)})
		run(1)
	}
	w.Input(input.PointerUp{Pos: to, Button: input.ButtonPrimary})
	run(2)
}

// newWeek returns a week from monday with one event on Tuesday from ten to eleven.
func newWeek() *Days {
	d := NewDays(monday, 7)
	d.OnCreate = func(s, e time.Time, allDay bool) gunim.Intent { return drawn{s, e, allDay} }
	d.OnChange = func(id string, s, e time.Time) gunim.Intent { return changed{id, s, e} }
	d.OnDay = func(day time.Time) gunim.Intent { return picked{day} }
	d.events = []Event{{ID: "a", Title: "Review", Start: monday.Add(34 * time.Hour), End: monday.Add(35 * time.Hour),
		Color: color.NRGBA{R: 0x4f, G: 0x8c, B: 0xf0, A: 0xff}}}
	return d
}

// point returns where the time offset into day i is on the grid.
func point(d *Days, i int, offset time.Duration) geom.Point {
	return geom.Pt(d.colX(i)+d.colW()/3, d.timeY(offset))
}

func TestDrawingOnFreeTimeBeginsAnEventOnTheSteps(t *testing.T) {
	d := newWeek()
	w, run, sent := stage(t, d)
	drag(w, run, point(d, 3, 13*time.Hour+5*time.Minute), point(d, 3, 14*time.Hour+20*time.Minute))
	got := sent()
	want := drawn{monday.Add(3*24*time.Hour + 13*time.Hour), monday.Add(3*24*time.Hour + 14*time.Hour + 30*time.Minute), false}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("drawing out Thursday 13:05 to 14:20 sent %v, want %v", got, want)
	}
}

func TestDraggingAnEventMovesItAcrossDaysAndTimes(t *testing.T) {
	d := newWeek()
	w, run, sent := stage(t, d)
	drag(w, run, point(d, 1, 10*time.Hour+10*time.Minute), point(d, 2, 12*time.Hour+10*time.Minute))
	got := sent()
	want := changed{"a", monday.Add(2*24*time.Hour + 12*time.Hour), monday.Add(2*24*time.Hour + 13*time.Hour)}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("dragging the event a day on and two hours later sent %v, want %v", got, want)
	}
	// It shows where it was left while the application catches up.
	if e, _ := d.event("a"); !e.Start.Equal(want.start) {
		t.Fatalf("the event shows at %v after the drop, want %v", e.Start, want.start)
	}
}

func TestDraggingAnEventsBottomEdgeChangesItsLength(t *testing.T) {
	d := newWeek()
	w, run, sent := stage(t, d)
	edge := point(d, 1, 11*time.Hour)
	edge.Y -= 4
	drag(w, run, edge, point(d, 1, 12*time.Hour+20*time.Minute))
	got := sent()
	want := changed{"a", monday.Add(34 * time.Hour), monday.Add(24*time.Hour + 12*time.Hour + 30*time.Minute)}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("dragging the bottom edge down sent %v, want %v", got, want)
	}
}

func TestAClickOnAnEventOpensItAndOnADayPicksIt(t *testing.T) {
	d := newWeek()
	var opened string
	d.Open = func(id string, _ geom.Rect, _ *gunim.UI) { opened = id }
	w, run, sent := stage(t, d)
	at := point(d, 1, 10*time.Hour+20*time.Minute)
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(1)
	if opened != "a" || len(sent()) != 0 {
		t.Fatalf("a click on the event opened %q, want it opened and nothing sent", opened)
	}
	head := geom.Pt(d.colX(4)+10, 20)
	w.Input(input.PointerDown{Pos: head, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: head, Button: input.ButtonPrimary})
	run(1)
	if got := sent(); len(got) != 1 || got[0] != (picked{monday.Add(4 * 24 * time.Hour)}) {
		t.Fatalf("a click on Friday's heading sent %v", got)
	}
}

func TestAnEventDraggedInTheMonthKeepsItsTime(t *testing.T) {
	m := NewMonth(monday)
	m.OnChange = func(id string, s, e time.Time) gunim.Intent { return changed{id, s, e} }
	start := monday.Add(34 * time.Hour)
	m.events = []Event{{ID: "a", Title: "Review", Start: start, End: start.Add(time.Hour)}}
	w, run, sent := stage(t, m)
	// Tuesday the 29th is in the fifth row of September, Thursday two cells on.
	from, to := m.cell(29), m.cell(31)
	chipAt := geom.Pt(from.Min.X+30, from.Min.Y+dayNumH+chipH/2)
	drag(w, run, chipAt, to.Center())
	got := sent()
	want := changed{"a", start.Add(48 * time.Hour), start.Add(49 * time.Hour)}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("dragging Tuesday's event to Thursday sent %v, want %v", got, want)
	}
}
