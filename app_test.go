package gunim

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// pumpDriver runs ready and then blocks until ctx ends, the way a
// platform event loop does.
type pumpDriver struct {
	err error
}

func (d *pumpDriver) Run(ctx context.Context, ready func()) error {
	ready()
	<-ctx.Done()
	return d.err
}

func (d *pumpDriver) NewWindow(driver.Options) (driver.Window, error) {
	return driver.Offscreen(geom.Sz(800, 600)), nil
}

func (d *pumpDriver) Monitors() []driver.Monitor { return nil }

func TestMainReturnsTheErrorFromFn(t *testing.T) {
	want := errors.New("fn failed")
	err := runApp(context.Background(), &pumpDriver{}, func(*App) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestMainJoinsTheDriverError(t *testing.T) {
	fnErr := errors.New("fn failed")
	runErr := errors.New("pump failed")
	err := runApp(context.Background(), &pumpDriver{err: runErr}, func(*App) error { return fnErr })
	if !errors.Is(err, fnErr) || !errors.Is(err, runErr) {
		t.Fatalf("err = %v, want both %v and %v", err, fnErr, runErr)
	}
}

func TestMainReturnsNilWhenFnSucceeds(t *testing.T) {
	if err := runApp(context.Background(), &pumpDriver{}, func(*App) error { return nil }); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

// failingWindow is an offscreen window whose Present always fails.
type failingWindow struct {
	*driver.OffscreenWindow
	err error
}

func (w failingWindow) Present([]paint.Op, geom.Rect) error { return w.err }

func TestFailedPresentClosesTheWindowOnce(t *testing.T) {
	want := errors.New("lost the GL context")
	w := newWindow(failingWindow{driver.Offscreen(geom.Sz(800, 600)), want}, nil)

	w.Frame(time.Second / 60)
	if !errors.Is(w.Err(), want) {
		t.Fatalf("Err = %v, want %v", w.Err(), want)
	}
	if err := w.Client().Update(Root, nil); !errors.Is(err, ErrWindowClosed) {
		t.Fatalf("Send after a failed present = %v, want ErrWindowClosed", err)
	}

	// The application closing a window that already closed itself is
	// ordinary, and must not panic on a second close of done.
	w.Close()
	w.Frame(time.Second / 60)
}

// textWindow is an offscreen window that records what the engine says
// about taking text.
type textWindow struct {
	*driver.OffscreenWindow
	active []bool
}

func (w *textWindow) SetTextInput(active bool) { w.active = append(w.active, active) }

type takesText struct{ recorder }

func (*takesText) TakesText() bool { return true }

func TestFocusTellsTheDriverWhenTextIsTaken(t *testing.T) {
	tw := &textWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(tw, nil)
	field, other := &takesText{}, &recorder{}
	w.ui.Insert(w.ui.Root(), field)
	w.ui.Insert(w.ui.Root(), other)

	w.ui.Focus(field)
	w.ui.Focus(other)
	if len(tw.active) != 2 || !tw.active[0] || tw.active[1] {
		t.Fatalf("text input went %v, want on for the field and off after it", tw.active)
	}
}
