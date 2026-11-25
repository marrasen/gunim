package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// A phone's Back takes a stack's screens off one at a time, whatever has the keyboard, then leaves. A dialog over the
// stack closes first.
func TestAPhonesBackTakesAStacksScreensOff(t *testing.T) {
	s := NewStack(NewLabel("Home"))
	g := &grab{child: s}
	w := gunim.NewOffscreenWith(gunim.WindowOptions{Size: geom.Sz(400, 600), LeaveOnBack: true})
	gunim.RegisterView(w, "stage", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "stage", "stage", nil); err != nil {
		t.Fatal(err)
	}
	popped := 0
	s.OnPop = func(*gunim.UI) gunim.Intent {
		popped++
		return nil
	}
	frames := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	frames(2)
	off := w.Offscreen()
	if !off.BackLeaves() {
		t.Fatal("on the first screen Back stays")
	}
	s.Push(NewLabel("Deeper"), g.ui)
	s.Push(NewLabel("Deepest"), g.ui)
	frames(1)
	if off.BackLeaves() {
		t.Fatal("with a screen on its way in Back leaves")
	}
	w.Input(driver.BackAsked{Time: time.Now()})
	if s.Len() != 2 || popped != 1 {
		t.Fatalf("%d screens are left, and OnPop ran %d times", s.Len(), popped)
	}
	// Again while the last one slides off.
	w.Input(driver.BackAsked{Time: time.Now()})
	if s.Len() != 1 || popped != 2 || !off.BackLeaves() || off.AppLeft() != 0 {
		t.Fatalf("%d screens are left, OnPop ran %d times, and Back leaves: %v", s.Len(), popped, off.BackLeaves())
	}
	frames(60)
	d := NewDialog("Sure?")
	dismissed := 0
	d.OnDismiss = func(*gunim.UI) gunim.Intent {
		dismissed++
		return nil
	}
	s.Push(NewLabel("Deeper"), g.ui)
	g.ui.Insert(g.ui.Root(), d)
	frames(60)
	w.Input(driver.BackAsked{Time: time.Now()})
	if dismissed != 1 || s.Len() != 2 {
		t.Fatalf("the dialog was dismissed %d times, and %d screens are left", dismissed, s.Len())
	}
}
