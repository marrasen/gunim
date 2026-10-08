package gunim

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// watcher is a recorder that watches the heading while watching is set.
type watcher struct {
	recorder
	watching bool
}

func (w *watcher) WatchesHeading() bool { return w.watching }

// headings returns the headings the node heard.
func (r *recorder) headings() []input.Heading {
	var hs []input.Heading
	for _, e := range r.events {
		if h, ok := e.(input.Heading); ok {
			hs = append(hs, h)
		}
	}
	return hs
}

// withCompass returns a test window on a pretend device with a compass, and the calls its compass has had, on
// for true and off for false.
func withCompass() (w *Window, calls *[]bool) {
	w = newTestWindow()
	calls = &[]bool{}
	w.Offscreen().SetCompass(func(on bool) { *calls = append(*calls, on) })
	return w, calls
}

func TestAWatcherShownRunsTheCompassAndHearsIt(t *testing.T) {
	w, calls := withCompass()
	if !w.Client().HasCompass() || !w.ui.HasCompass() {
		t.Fatal("a window on a device with a compass says it has none")
	}
	n, other := &watcher{watching: true}, &recorder{}
	w.ui.Insert(w.ui.Root(), other)
	w.ui.Insert(w.ui.Root(), n)
	run(w, 1)
	if !slices.Equal(*calls, []bool{true}) {
		t.Fatalf("with a watcher shown the compass had %v, want it started", *calls)
	}
	at := time.Now()
	w.Input(input.Heading{Degrees: 92.5, Accuracy: input.HeadingMedium, Error: 12, Time: at})
	want := []input.Heading{{Degrees: 92.5, Accuracy: input.HeadingMedium, Error: 12, Time: at}}
	if got := n.headings(); !slices.Equal(got, want) {
		t.Fatalf("the watcher heard %v, want %v", got, want)
	}
	if len(other.headings()) != 0 {
		t.Fatalf("a node that does not watch heard %v", other.headings())
	}
	run(w, 3)
	if len(*calls) != 1 {
		t.Fatalf("the compass had %v over frames that changed nothing, want one start", *calls)
	}
}

func TestEveryWatcherHearsEachReading(t *testing.T) {
	w, calls := withCompass()
	a, b := &watcher{watching: true}, &watcher{watching: true}
	box := &Box{}
	w.ui.Insert(w.ui.Root(), a)
	w.ui.Insert(w.ui.Root(), box)
	w.ui.Insert(box, b)
	run(w, 1)
	w.Input(input.Heading{Degrees: 10})
	w.Input(input.Heading{Degrees: 20})
	if len(a.headings()) != 2 || len(b.headings()) != 2 {
		t.Fatalf("the watchers heard %v and %v, want both readings each", a.headings(), b.headings())
	}
	if !slices.Equal(*calls, []bool{true}) {
		t.Fatalf("two watchers gave the compass %v, want one start", *calls)
	}
}

func TestTheCompassStopsOnceNothingWatches(t *testing.T) {
	w, calls := withCompass()
	n := &watcher{watching: true}
	w.ui.Insert(w.ui.Root(), n)
	run(w, 1)

	// It says it no longer watches.
	n.watching = false
	w.ui.Invalidate()
	run(w, 1)
	if !slices.Equal(*calls, []bool{true, false}) {
		t.Fatalf("a watcher that stopped watching left the compass with %v, want started then stopped", *calls)
	}
	w.Input(input.Heading{Degrees: 10})
	if len(n.headings()) != 0 {
		t.Fatalf("a node that stopped watching heard %v", n.headings())
	}

	// It watches again, then leaves the tree.
	n.watching = true
	w.ui.Invalidate()
	run(w, 1)
	w.ui.Remove(n)
	w.Input(input.Heading{Degrees: 10})
	if len(n.headings()) != 0 {
		t.Fatalf("a watcher leaving the tree heard %v", n.headings())
	}
	run(w, 1)
	if !slices.Equal(*calls, []bool{true, false, true, false}) {
		t.Fatalf("a watcher removed left the compass with %v, want it stopped", *calls)
	}
	if len(w.ui.headingNodes) != 0 {
		t.Fatalf("with the watcher gone, frames still look for watchers among %d nodes", len(w.ui.headingNodes))
	}
}

func TestAWatcherNotDrawnLeavesTheCompassOff(t *testing.T) {
	w, calls := withCompass()
	st := &stage{skip: true}
	n := &watcher{watching: true}
	w.ui.Insert(w.ui.Root(), st)
	w.ui.Insert(st, n)
	run(w, 2)
	if len(*calls) != 0 {
		t.Fatalf("a watcher its parent does not paint gave the compass %v, want nothing", *calls)
	}
	w.Input(input.Heading{Degrees: 10})
	if len(n.headings()) != 0 {
		t.Fatalf("a watcher not drawn heard %v", n.headings())
	}

	st.skip = false
	w.ui.Invalidate()
	run(w, 1)
	if !slices.Equal(*calls, []bool{true}) {
		t.Fatalf("the watcher drawn at last gave the compass %v, want it started", *calls)
	}
}

func TestTheCompassRestsWhileTheWindowIsHidden(t *testing.T) {
	w, calls := withCompass()
	w.ui.Insert(w.ui.Root(), &watcher{watching: true})
	run(w, 1)
	w.Input(driver.WindowShown{Shown: false})
	if !slices.Equal(*calls, []bool{true, false}) {
		t.Fatalf("hiding the window left the compass with %v, want it stopped", *calls)
	}
	w.Input(driver.WindowShown{Shown: true})
	run(w, 1)
	if !slices.Equal(*calls, []bool{true, false, true}) {
		t.Fatalf("showing the window again left the compass with %v, want it started again", *calls)
	}
}

func TestTheCompassStopsAsTheWindowLeaves(t *testing.T) {
	w, calls := withCompass()
	w.ui.Insert(w.ui.Root(), &watcher{watching: true})
	run(w, 1)
	w.ui.startLeaving()
	run(w, 1)
	if !slices.Equal(*calls, []bool{true, false}) {
		t.Fatalf("a window leaving left the compass with %v, want it stopped", *calls)
	}
	w.ui.stopHeading()
	if len(*calls) != 2 {
		t.Fatalf("closing a window whose compass had stopped gave it %v, want nothing more", *calls)
	}
}

func TestAWindowWithNoCompassSaysSo(t *testing.T) {
	w := newTestWindow()
	if w.Client().HasCompass() || w.ui.HasCompass() {
		t.Fatal("a window on a device with no compass says it has one")
	}
	var u *UI
	if u.HasCompass() {
		t.Fatal("a nil UI says it has a compass")
	}
	n := &watcher{watching: true}
	w.ui.Insert(w.ui.Root(), n)
	run(w, 2)
	if w.ui.compassOn {
		t.Fatal("a watcher on a device with no compass turned one on")
	}
}
