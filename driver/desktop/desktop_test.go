//go:build linux || windows || darwin

package desktop

import (
	"context"
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// display is the driver the tests share, or nil when no display is
// attached. GLFW has to run on the main thread, so TestMain keeps the
// main thread for the event loop and runs the tests beside it.
var display *Driver

func TestMain(m *testing.M) {
	d, err := Open()
	if err != nil {
		os.Exit(m.Run())
	}
	display = d
	d.stayOpen = true
	ctx, cancel := context.WithCancel(context.Background())
	code := make(chan int, 1)
	_ = d.Run(ctx, func() {
		go func() {
			code <- m.Run()
			cancel()
		}()
	})
	os.Exit(<-code)
}

func TestWindowPresentsAndReportsTheFrame(t *testing.T) {
	if display == nil {
		t.Skip("no display")
	}
	w, err := display.NewWindow(driver.Options{Title: "gunim test", Size: geom.Sz(200, 120)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()

	if s := w.Size(); s.W <= 0 || s.H <= 0 {
		t.Fatalf("Size = %v, want the window's size", s)
	}
	ops := []paint.Op{&paint.RRectOp{
		Rect:      geom.Rc(10, 10, 100, 60),
		Radius:    8,
		Fill:      paint.Solid(color.NRGBA{R: 0xff, A: 0xff}),
		Transform: paint.Identity,
	}}
	for i := range 3 {
		if err := w.Present(ops, geom.Rect{}); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		select {
		case f := <-w.Presented():
			if f.Shown.IsZero() {
				t.Fatalf("frame %d reported with no time", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("frame %d was never reported shown", i)
		}
	}
}

func TestPaceHoldsAnUnpacedSwapToTheRefreshRate(t *testing.T) {
	const rate = 50
	interval := time.Second / rate
	last := time.Now()
	shown := pace(last, rate)
	if want := last.Add(interval); !shown.Equal(want) {
		t.Fatalf("shown %v after the last frame, want %v", shown.Sub(last), interval)
	}
	if time.Now().Before(shown) {
		t.Fatal("pace returned before the refresh it reported")
	}
}

func TestPaceLeavesAPacedSwapAlone(t *testing.T) {
	last := time.Now().Add(-time.Second / 50)
	before := time.Now()
	shown := pace(last, 50)
	if shown.Before(before) || time.Since(before) > 5*time.Millisecond {
		t.Fatalf("pace slept after a swap that had already waited a refresh")
	}
}

func TestInboxDeliversInOrderAndCloses(t *testing.T) {
	quit := make(chan struct{})
	defer close(quit)
	q := newInbox(quit)
	for i := range 100 {
		q.push(i)
	}
	q.close()
	q.push("after close")

	want := 0
	for ev := range q.out {
		if ev != want {
			t.Fatalf("got %v, want %d", ev, want)
		}
		want++
	}
	if want != 100 {
		t.Fatalf("delivered %d events, want 100", want)
	}
}

// TestBackdropFramesStayUpright draws a frame that is white on top and
// black below, under a full-window backdrop layer followed by an
// unclipped layer, the way the confirm dialog paints. Under one software
// driver, the copy of such a frame to the window came out upside down
// when its shader read gl_FragCoord.
func TestBackdropFramesStayUpright(t *testing.T) {
	if display == nil {
		t.Skip("no display")
	}
	dw, err := display.NewWindow(driver.Options{Title: "gunim backdrop", Size: geom.Sz(160, 120)})
	if err != nil {
		t.Fatal(err)
	}
	w, ok := dw.(*Window)
	if !ok {
		t.Fatalf("NewWindow returned a %T", dw)
	}
	defer func() { _ = w.Close() }()

	type frame struct {
		pix  []byte
		w, h int
	}
	got := make(chan frame, 1)
	w.mu.Lock()
	w.readback = func(pix []byte, fw, fh int) {
		select {
		case got <- frame{pix, fw, fh}:
		default:
		}
	}
	w.mu.Unlock()

	size := w.Size()
	full := geom.Rect{Max: size.Point()}
	top := geom.Rect{Max: geom.Pt(size.W, size.H/2)}
	white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	black := color.NRGBA{A: 0xff}

	// 4, 12 and 30 pixels run the blur at full, half and quarter
	// resolution.
	for _, sigma := range []float32{4, 12, 30} {
		ops := []paint.Op{
			&paint.RRectOp{Rect: full, Fill: paint.Solid(black), Transform: paint.Identity},
			&paint.RRectOp{Rect: top, Fill: paint.Solid(white), Transform: paint.Identity},
			&paint.LayerOp{Opts: paint.LayerOpts{Bounds: full, Opacity: 1, Backdrop: sigma}, Transform: paint.Identity},
			&paint.LayerEndOp{},
			&paint.LayerOp{Opts: paint.LayerOpts{Bounds: full, Opacity: 1}, Transform: paint.Identity},
			&paint.LayerEndOp{},
		}
		// Several frames, since the flip came and went from frame to
		// frame.
		for i := range 4 {
			if err := w.Present(ops, geom.Rect{}); err != nil {
				t.Fatal(err)
			}
			var f frame
			select {
			case f = <-got:
			case <-time.After(5 * time.Second):
				t.Fatal("no frame read back")
			}
			<-w.Presented()
			// Rows arrive bottom up: the first row is the window's bottom.
			bottom := f.pix[(f.w/2)*4]
			topRow := f.pix[((f.h-1)*f.w+f.w/2)*4]
			if topRow < 200 || bottom > 55 {
				t.Fatalf("sigma %v, frame %d: top %d and bottom %d, want white on top of black", sigma, i, topRow, bottom)
			}
		}
	}
}
