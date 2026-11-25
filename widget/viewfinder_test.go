package widget

import (
	"errors"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// finderStage mounts a viewfinder in a window with a pretend camera.
func finderStage(t *testing.T, v *Viewfinder) (w *gunim.Window, g *grab, run func(int), on *bool) {
	t.Helper()
	g = &grab{child: v}
	w, run = stageWith(t, g, func(w *gunim.Window) {
		on = new(bool)
		w.Offscreen().SetCamera(func(_ driver.CameraOptions, is bool) error {
			*on = is
			return nil
		})
	})
	return w, g, run, on
}

// stageWith is stage with the window set up by set before anything is in it.
func stageWith(t *testing.T, root gunim.Node, set func(w *gunim.Window)) (w *gunim.Window, run func(int)) {
	t.Helper()
	w, run = stage(t, &frame{child: Column(), size: geom.Sz(1, 1)})
	set(w)
	gunim.RegisterView(w, "finder", func(struct{}) gunim.Node { return root }, nil)
	if err := w.Client().Mount(gunim.Root, "finder", "finder", nil); err != nil {
		t.Fatal(err)
	}
	run(2)
	return w, run
}

// A viewfinder fades its picture in on every frame, and out as it is turned off, and then keeps none.
func TestAViewfinderFadesInAndOutAndKeepsNoPicture(t *testing.T) {
	v := NewViewfinder()
	v.Label = "What the camera sees"
	w, g, run, on := finderStage(t, v)
	if !*on {
		t.Fatal("a viewfinder that shows does not run the camera")
	}
	img := (&paint.Stream{}).Image(4, 2, make([]byte, 32))
	w.Offscreen().SetCameraPicture(img)
	w.Input(driver.CameraReady{})
	last := v.in.Value()
	if last != 0 {
		t.Fatalf("the first picture starts %v in", last)
	}
	for range 60 {
		run(1)
		if now := v.in.Value(); now < last-1e-3 {
			t.Fatalf("it went from %v in to %v", last, now)
		} else {
			last = now
		}
	}
	if last < 0.99 || v.img != img {
		t.Fatalf("it rests %v in", last)
	}
	v.SetOn(false, g.ui)
	run(1)
	if *on || v.On() {
		t.Fatal("turned off, it runs the camera")
	}
	for range 90 {
		run(1)
		if now := v.in.Value(); now > last+1e-3 {
			t.Fatalf("on its way out it went from %v in to %v", last, now)
		} else {
			last = now
		}
	}
	if v.img != nil {
		t.Fatal("it keeps the last picture")
	}
	if a := v.Access(); a.Name != "What the camera sees" {
		t.Fatalf("a screen reader hears %q", a.Name)
	}
}

// Out of sight there is no frame to fade in, and it keeps no picture.
func TestAViewfinderOutOfSightKeepsNoPicture(t *testing.T) {
	v := NewViewfinder()
	w, _, run, on := finderStage(t, v)
	w.Offscreen().SetCameraPicture((&paint.Stream{}).Image(1, 1, make([]byte, 4)))
	w.Input(driver.CameraReady{})
	run(30)
	w.Input(driver.WindowShown{Shown: false})
	if *on || v.img != nil || v.in.Value() != 0 {
		t.Fatalf("out of sight the camera runs: %v, and a picture is kept: %v", *on, v.img != nil)
	}
	w.Input(driver.WindowShown{Shown: true})
	run(2)
	if !*on {
		t.Fatal("in sight again it does not run the camera")
	}
}

// Why a camera does not run is told once, and a stop that was asked for is not.
func TestAViewfinderSaysWhyTheCameraDoesNotRun(t *testing.T) {
	v := NewViewfinder()
	var said []error
	v.OnStopped = func(err error, _ *gunim.UI) gunim.Intent {
		said = append(said, err)
		return nil
	}
	w, g, run, _ := finderStage(t, v)
	v.SetOn(false, g.ui)
	run(2)
	v.SetOn(true, g.ui)
	run(2)
	if len(said) != 0 {
		t.Fatalf("a stop that was asked for said %v", said)
	}
	w.Input(driver.CameraStopped{Run: 2, Err: driver.ErrCameraBusy})
	run(5)
	if len(said) != 1 || !errors.Is(said[0], driver.ErrCameraBusy) {
		t.Fatalf("it said %v", said)
	}
}

func TestAViewfinderTakesTheRoomOrTheSizeItHas(t *testing.T) {
	v := NewViewfinder()
	if got := v.Layout(gunim.Constraints{Max: geom.Sz(300, 200)}, gunim.Frame{}, gunim.Children{}); got != geom.Sz(300, 200) {
		t.Errorf("it took %v", got)
	}
	v.Size = geom.Sz(120, 90)
	if got := v.Layout(gunim.Constraints{Max: geom.Sz(300, 200)}, gunim.Frame{}, gunim.Children{}); got != geom.Sz(120, 90) {
		t.Errorf("it took %v", got)
	}
	if bad := spills(painted(v, geom.Sz(120, 90)), geom.Sz(120, 90), 3); len(bad) > 0 {
		t.Errorf("it draws outside its box: %v", bad)
	}
}
