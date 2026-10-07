package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// hideFirstPage stages tabs whose first page is shown, and returns what moves to the second, hiding the first.
func hideFirstPage(t *testing.T, shown gunim.Node) (hide func(), run func(int)) {
	t.Helper()
	tabs := NewTabs([]string{"Shown", "Other"}, shown, NewLabel("Here"))
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(400, 300)})
	run(5)
	return func() {
		click(w, tabs.spans[1][0]+5, 10)
		run(60)
	}, run
}

// A grid waiting for rows pulses while shown, and lets the window rest once its page is out of sight.
func TestADataGridWaitingForRowsOutOfSightStopsAskingForFrames(t *testing.T) {
	g := NewDataGrid(GridColumn{Title: "Message"})
	g.Row = func(int) (GridRow, bool) { return GridRow{}, false }
	g.rows = 100
	hide, run := hideFirstPage(t, g)
	if !g.Step(time.Second / 60) {
		t.Fatal("a grid waiting for rows on the page shown asks for no frames")
	}
	run(1)
	hide()
	for i := range 5 {
		if g.Step(time.Second / 60) {
			t.Fatalf("step %d: a grid waiting for rows on a hidden page still asks for frames", i)
		}
		run(1)
	}
}

// A running graph draws while shown, and lets the window rest once its page is out of sight.
func TestARunningLiveGraphOutOfSightStopsAskingForFrames(t *testing.T) {
	g := NewLiveGraph(100*time.Millisecond, 50)
	for _, v := range []float64{10, 30, 20, 50, 40} {
		g.Add(v)
	}
	g.SetRunning(true)
	hide, run := hideFirstPage(t, g)
	if g.WakeIn() == 0 {
		t.Fatal("a running graph on the page shown asks for no frames")
	}
	hide()
	for i := range 5 {
		if g.Step(time.Second/60) || g.WakeIn() != 0 {
			t.Fatalf("step %d: a running graph on a hidden page still asks for frames", i)
		}
		run(1)
	}
}
