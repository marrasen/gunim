package gunim

import (
	"sync"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// An application closes its window from its own goroutine, as Client's methods allow, while the
// test that drives the offscreen window steps it. Under -race this shows whether Close touches the
// popups while a frame reads them.
func TestClosingAnOffscreenWindowFromTheAppWhileFramesRun(t *testing.T) {
	w, _, _, _ := openMenu(t, nil)
	s := w.ui.popups[0]
	var wg sync.WaitGroup
	wg.Go(func() {
		time.Sleep(time.Millisecond)
		w.Client().Close()
	})
	for range 200 {
		w.Frame(time.Second / 60)
	}
	wg.Wait()
	w.Frame(time.Second / 60)
	if len(w.ui.popups) != 0 {
		t.Fatalf("closed, the window still has %d popups", len(w.ui.popups))
	}
	select {
	case <-s.stop:
	default:
		t.Fatal("closed, the popup's goroutine was not told to stop")
	}
}

// selfCloser closes its window from inside a frame, as a window that has left or failed to present does.
type selfCloser struct{ w *Window }

func (*selfCloser) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(geom.Sz(1, 1))
}
func (*selfCloser) Paint(*paint.Painter, Frame, geom.Size, Children) {}
func (c *selfCloser) Step(time.Duration) bool {
	c.w.Close()
	return false
}

// A window that closes inside its own frame still closes its popups, by the end of that frame.
func TestAnOffscreenWindowClosedInItsFrameClosesItsPopups(t *testing.T) {
	w, _, m, _ := openMenu(t, nil)
	s := w.ui.popups[0]
	w.ui.Insert(m, &selfCloser{w: w})
	w.Frame(time.Second / 60)
	if len(w.ui.popups) != 0 {
		t.Fatalf("closed, the window still has %d popups", len(w.ui.popups))
	}
	select {
	case <-s.stop:
	default:
		t.Fatal("closed, the popup's goroutine was not told to stop")
	}
}
