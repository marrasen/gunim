package gunim

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// seer is a node that watches the camera while on is set, and notes what it hears.
type seer struct {
	drawnBox
	on, front bool
	pictures  []*paint.Image
	stops     []error
}

// drawnBox is a node of no size that draws nothing.
type drawnBox struct{ _ int }

func (*drawnBox) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (*drawnBox) Paint(*paint.Painter, Frame, geom.Size, Children)    {}
func (s *seer) WatchesCamera() (CameraOptions, bool)                  { return CameraOptions{Front: s.front}, s.on }
func (s *seer) CameraPicture(img *paint.Image, _ *UI)                 { s.pictures = append(s.pictures, img) }
func (s *seer) CameraStopped(err error, _ *UI)                        { s.stops = append(s.stops, err) }

// withCamera returns a test window on a pretend device with a camera, and the calls it has had: "+" to start the one on the
// back, "+f" the one in front, and "-" to stop.
func withCamera() (w *Window, calls *[]string) {
	w = newTestWindow()
	calls = &[]string{}
	w.Offscreen().SetCamera(func(o CameraOptions, on bool) error {
		c := "-"
		if on {
			c = "+"
			if o.Front {
				c += "f"
			}
		}
		*calls = append(*calls, c)
		return nil
	})
	return w, calls
}

func wantCalls(t *testing.T, calls *[]string, when string, want ...string) {
	t.Helper()
	if !slices.Equal(*calls, want) {
		t.Fatalf("%s the camera had %v, want %v", when, *calls, want)
	}
}

// The camera runs while a node that watches is drawn, and stops as the node stops watching, leaves, is drawn no more, or
// the window goes out of sight.
func TestTheCameraRunsWhileItsNodeIsSeen(t *testing.T) {
	w, calls := withCamera()
	n := &seer{}
	cover := &coverer{child: n}
	w.ui.Insert(w.ui.Root(), cover)
	run(w, 2)
	wantCalls(t, calls, "with no one watching")
	n.on = true
	w.ui.Invalidate()
	run(w, 3)
	wantCalls(t, calls, "watched", "+")
	img := (&paint.Stream{}).Image(1, 1, make([]byte, 4))
	w.Offscreen().SetCameraPicture(img)
	w.Input(driver.CameraReady{})
	if len(n.pictures) != 1 || n.pictures[0] != img {
		t.Fatalf("the node has %d pictures", len(n.pictures))
	}
	n.on = false
	w.ui.Invalidate()
	run(w, 1)
	wantCalls(t, calls, "watched no more", "+", "-")
	if len(n.stops) != 1 || n.stops[0] != nil {
		t.Fatalf("the node heard %v", n.stops)
	}
	// One on its way as it stopped is for no one.
	w.Offscreen().SetCameraPicture(img)
	w.Input(driver.CameraReady{})
	if len(n.pictures) != 1 {
		t.Fatal("a stopped camera handed over a picture")
	}

	*calls, n.on = nil, true
	w.ui.Invalidate()
	run(w, 1)
	w.Input(driver.WindowShown{Shown: false})
	wantCalls(t, calls, "with the window hidden", "+", "-")
	run(w, 2)
	w.Input(driver.WindowShown{Shown: true})
	run(w, 1)
	wantCalls(t, calls, "shown again", "+", "-", "+")
	w.Input(driver.WindowCovered{Covered: true})
	wantCalls(t, calls, "covered", "+", "-", "+", "-")
	w.Input(driver.WindowCovered{Covered: false})
	run(w, 1)
	wantCalls(t, calls, "covered no more", "+", "-", "+", "-", "+")

	*calls = nil
	cover.hide = true
	w.ui.Invalidate()
	run(w, 1)
	wantCalls(t, calls, "drawn no more", "-")
	cover.hide = false
	w.ui.Invalidate()
	run(w, 1)
	w.ui.Remove(cover)
	run(w, 2)
	wantCalls(t, calls, "with the node gone", "-", "+", "-")
	if len(w.ui.cameraNodes) != 0 {
		t.Fatalf("frames still look among %d nodes", len(w.ui.cameraNodes))
	}
}

// The last in the tree has the camera, and another side starts it anew.
func TestTheLastInTheTreeHasTheCamera(t *testing.T) {
	w, calls := withCamera()
	a, b := &seer{on: true}, &seer{}
	w.ui.Insert(w.ui.Root(), a)
	w.ui.Insert(w.ui.Root(), b)
	run(w, 1)
	b.on, b.front = true, true
	w.ui.Invalidate()
	run(w, 1)
	wantCalls(t, calls, "with two watching", "+", "-", "+f")
	b.front = false
	w.ui.Invalidate()
	run(w, 1)
	wantCalls(t, calls, "turned round", "+", "-", "+f", "-", "+")
	if len(a.stops) != 1 || len(b.stops) != 1 {
		t.Fatalf("they heard %v and %v", a.stops, b.stops)
	}
	w.ui.Remove(b)
	run(w, 2)
	if !w.ui.camera.on || w.ui.camera.s.node != Node(a) {
		t.Fatal("the first does not have it back")
	}
}

// A camera that cannot start, or ends by itself, says so once, and is not tried again until the node stops watching or is
// out of sight. One that stopped before the one that runs says nothing.
func TestACameraThatEndsIsNotTriedAgainAtOnce(t *testing.T) {
	w, calls := withCamera()
	n := &seer{on: true}
	w.ui.Insert(w.ui.Root(), n)
	run(w, 1)
	first := w.ui.camera.run
	busy := errors.New("busy")
	w.Input(driver.CameraStopped{Run: first, Err: busy})
	run(w, 5)
	wantCalls(t, calls, "ended", "+")
	if len(n.stops) != 1 || !errors.Is(n.stops[0], busy) {
		t.Fatalf("the node heard %v", n.stops)
	}
	w.Input(driver.CameraStopped{Run: first, Err: busy})
	if len(n.stops) != 1 {
		t.Fatal("it was told twice")
	}
	w.Input(driver.WindowShown{Shown: false})
	w.Input(driver.WindowShown{Shown: true})
	run(w, 1)
	wantCalls(t, calls, "shown again", "+", "+")
	w.Input(driver.CameraStopped{Run: first, Err: busy})
	if len(n.stops) != 1 || !w.ui.camera.on {
		t.Fatal("the word of one before was taken for the one that runs")
	}

	// With none at all, the node is told, once.
	w = newTestWindow()
	n = &seer{on: true}
	w.ui.Insert(w.ui.Root(), n)
	run(w, 5)
	if len(n.stops) != 1 || !errors.Is(n.stops[0], ErrNoCamera) {
		t.Fatalf("with no camera the node heard %v", n.stops)
	}
	if !w.ui.InSight() || (*UI)(nil).InSight() {
		t.Fatal("InSight is wrong")
	}
}

// The camera stops before the window closes.
func TestTheCameraStopsAsTheWindowCloses(t *testing.T) {
	var calls []string
	dw := driver.Offscreen(geom.Sz(100, 100))
	dw.SetCamera(func(_ CameraOptions, on bool) error {
		calls = append(calls, map[bool]string{true: "+", false: "-"}[on])
		return nil
	})
	w := newWindow(dw, nil)
	w.ui.Insert(w.ui.Root(), &seer{on: true})
	w.Frame(time.Second / 60)
	w.Input(input.KeyPress{Key: input.KeyA})
	w.ui.stopCamera()
	w.ui.stopCamera()
	if !slices.Equal(calls, []string{"+", "-"}) {
		t.Fatalf("the camera had %v", calls)
	}
}
