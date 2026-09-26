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
