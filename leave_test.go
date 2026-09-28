package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/paint"
)

// A window leaving draws its content in a layer that fades as it
// shrinks, takes no input meanwhile, and closes once it has gone.
func TestAWindowLeavesFadingAndThenCloses(t *testing.T) {
	w, _, r := newStage(t, paint.Identity)
	c := w.Client()
	c.Leave()
	w.Frame(time.Second / 60)
	w.Frame(LeaveTime / 2)
	ops := w.Offscreen().Ops()
	l, ok := ops[0].(*paint.LayerOp)
	if !ok || l.Opts.Opacity <= 0 || l.Opts.Opacity >= 1 {
		t.Fatalf("halfway out, the frame starts with %#v", ops[0])
	}
	if l.Transform.A >= 1 {
		t.Fatalf("halfway out, the content is drawn at scale %v", l.Transform.A)
	}
	press(w, 50, 30)
	if len(r.events) != 0 {
		t.Fatalf("leaving, the window passed on %v", r.events)
	}
	w.Frame(LeaveTime)
	select {
	case <-w.done:
	default:
		t.Fatal("gone, the window is still open")
	}
}

// A window opened to arrive grows and fades in, and is drawn plainly
// once it has.
func TestAWindowArrivesFadingIn(t *testing.T) {
	w, _, _ := newStage(t, paint.Identity)
	w.ui.arriving, w.ui.arrivedAt = true, time.Time{}
	w.Frame(time.Second / 60)
	l, ok := w.Offscreen().Ops()[0].(*paint.LayerOp)
	if !ok || l.Opts.Opacity > 0.1 || l.Transform.A >= 1 {
		t.Fatalf("arriving, the frame starts with %#v", w.Offscreen().Ops()[0])
	}
	w.Frame(ArriveTime)
	w.Frame(time.Second / 60)
	if ops := w.Offscreen().Ops(); len(ops) > 0 {
		if _, ok := ops[0].(*paint.LayerOp); ok {
			t.Fatal("arrived, the window is still drawn in a layer")
		}
	}
}

// A window that arrived animated, closed by the user with no one to
// ask, leaves animated too, and closes once it has.
func TestAWindowThatArrivedLeavesTheSameWay(t *testing.T) {
	w, _, _ := newStage(t, paint.Identity)
	w.ui.animated = true
	w.ui.AskToClose()
	w.Frame(time.Second / 60)
	w.Frame(LeaveTime / 2)
	if _, ok := w.Offscreen().Ops()[0].(*paint.LayerOp); !ok {
		t.Fatal("closed by the user, the window did not leave animated")
	}
	select {
	case <-w.done:
		t.Fatal("the window closed before it had left")
	default:
	}
	w.Frame(LeaveTime)
	select {
	case <-w.done:
	default:
		t.Fatal("left, the window is still open")
	}
}

// A window all there sets its background, fully opaque, under its
// frames, so nothing of the desktop shows through what the tree leaves
// unpainted. While it arrives, the background fades in with it.
func TestAWindowAllThereIsOpaqueUnderItsTree(t *testing.T) {
	w, _, _ := newStage(t, paint.Identity)
	w.Frame(time.Second / 60)
	want := WindowBackground.Get(w.ui.theme)
	if got := w.Offscreen().Background(); got != want || got.A != 0xff {
		t.Fatalf("the window's background is %v, want the theme's %v, opaque", got, want)
	}
	w.ui.arriving = true
	w.ui.arrivedAt = time.Time{}
	w.Frame(time.Second / 60)
	w.Frame(ArriveTime / 3)
	if a := w.Offscreen().Background().A; a == 0 || a == 0xff {
		t.Fatalf("a third of the way in, the background's alpha is %d, want it on its way in", a)
	}
	if _, ok := w.Offscreen().Ops()[0].(*paint.LayerOp); !ok {
		t.Fatalf("an arriving window's frame starts with %T, want its fading layer", w.Offscreen().Ops()[0])
	}
}
