package widget_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

type kitState struct{ N int }

// mountKit mounts root in a window of size, and is how to change it:
// change runs on the UI goroutine with each number published, and the
// window lays out again.
func mountKit(t *testing.T, size geom.Size, root gunim.Node, change func(n int)) (w *gunim.Window, publish func(n int)) {
	t.Helper()
	w = gunimtest.New(t, size, nil)
	gunim.RegisterView(w, "kit", func(kitState) gunim.Node { return root },
		func(_ gunim.Node, s kitState, u *gunim.UI) {
			if s.N > 0 {
				change(s.N)
			}
			u.Invalidate()
		})
	c := w.Client()
	if err := c.Mount(gunim.Root, "kit", "kit", kitState{}, "kit"); err != nil {
		t.Fatal(err)
	}
	return w, func(n int) {
		if err := c.Publish("kit", kitState{N: n}); err != nil {
			t.Fatal(err)
		}
	}
}

func frameN(w *gunim.Window, n int) {
	for range n {
		w.Frame(time.Second / 60)
	}
}

// A badge is there at once the first time; hidden, it shrinks and
// fades out over a few frames, not at once.
func TestABadgeFadesOutWhenHidden(t *testing.T) {
	b := widget.NewBadge("2 problems")
	w, publish := mountKit(t, geom.Sz(300, 100), widget.Row(b, widget.NewSpacer()), func(int) { b.Hidden = true })
	frameN(w, 2)
	if got := widget.BadgeShown(b); got != 1 {
		t.Fatalf("shown at first, it is %v of the way in", got)
	}
	publish(1)
	frameN(w, 3)
	if got := widget.BadgeShown(b); got <= 0 || got >= 1 {
		t.Fatalf("a few frames after hiding, it is %v of the way in; want it part way out", got)
	}
	frameN(w, 120)
	if got := widget.BadgeShown(b); got > 0.001 {
		t.Fatalf("hidden and settled, it is %v of the way in", got)
	}
}

type toggled struct{ On bool }

// A click on a status switch asks for the other state, and the knob
// slides there as On follows.
func TestAStatusSwitchAsksForTheOtherState(t *testing.T) {
	s := widget.NewStatusSwitch("not connected")
	s.OnToggle = func(on bool, _ *gunim.UI) gunim.Intent { return toggled{On: on} }
	w, publish := mountKit(t, geom.Sz(300, 100), widget.Column(widget.Row(s, widget.NewSpacer()), widget.NewSpacer()),
		func(int) { s.Checked = true })
	frameN(w, 2)
	w.Input(input.PointerMove{Pos: geom.Pt(10, 10)})
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary})
	w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary})
	frameN(w, 1)
	select {
	case env := <-w.Client().Intents():
		if env.Intent != (toggled{On: true}) {
			t.Fatalf("the click sent %#v", env.Intent)
		}
	case <-time.After(time.Second):
		t.Fatal("the click sent nothing")
	}
	publish(1)
	frameN(w, 3)
	if k := widget.KnobAt(s); k <= 0 || k >= 1 {
		t.Fatalf("a few frames after turning on, the knob is at %v; want it on its way", k)
	}
}

// A log view follows the newest line and says it was seen; scrolled
// back, it stays on the lines it shows as more come.
func TestALogViewStaysWhereItWasScrolledBack(t *testing.T) {
	var lines []widget.LogLine
	add := func(n int) {
		for range n {
			lines = append(lines, widget.LogLine{Lead: "09:00:00", Text: fmt.Sprint("line ", len(lines))})
		}
	}
	add(100)
	v := widget.NewLogView(func(most int) ([]widget.LogLine, int64) {
		return lines[max(0, len(lines)-most):], int64(len(lines))
	})
	var seen int64
	v.OnSeen = func(end int64, _ *gunim.UI) gunim.Intent { seen = end; return nil }
	w, publish := mountKit(t, geom.Sz(600, 500), v, func(int) { add(5) })
	frameN(w, 2)
	if seen != 100 {
		t.Fatalf("at the bottom, it says %d were seen", seen)
	}
	w.Input(input.Scroll{Pos: geom.Pt(100, 100), Notches: geom.Pt(0, 2)})
	frameN(w, 1)
	back := widget.LogBack(v)
	if back != 6 {
		t.Fatalf("two notches up, it is %d lines back", back)
	}
	publish(1)
	frameN(w, 1)
	if got := widget.LogBack(v); got != back+5 {
		t.Fatalf("five lines on, it is %d back; want %d, on the same lines", got, back+5)
	}
	if seen != 100 {
		t.Fatalf("scrolled back, it says %d were seen", seen)
	}
}

// A compact live graph takes its room before its first samples, and
// asks Sample for them while it runs.
func TestACompactLiveGraphSamplesItself(t *testing.T) {
	g := widget.NewLiveGraph(100*time.Millisecond, 30)
	g.Compact, g.Width = true, 40
	asked := 0
	g.Sample = func() float64 { asked++; return float64(asked) }
	g.SetRunning(true)
	w, _ := mountKit(t, geom.Sz(300, 100), widget.Column(widget.Row(g, widget.NewSpacer()), widget.NewSpacer()), func(int) {})
	frameN(w, 1)
	if asked != 0 {
		t.Fatalf("before any time passed, Sample was asked %d times", asked)
	}
	frameN(w, 30)
	if asked < 3 {
		t.Fatalf("half a second on, Sample was asked %d times; want about 5", asked)
	}
}
