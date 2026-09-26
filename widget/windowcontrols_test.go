package widget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The buttons minimize, maximize and close the window, and the
// maximize button is reported to the system.
func TestWindowControlsWorkTheWindow(t *testing.T) {
	c := NewWindowControls()
	w := gunim.NewOffscreen(geom.Sz(600, 300), nil)
	fr := w.MakeChromeless(true)
	sp := NewSpacer()
	row := Row(sp, c).Grow(sp, 1)
	gunim.RegisterView(w, "bar", func(struct{}) gunim.Node { return &frame{child: row, size: geom.Sz(600, 30)} }, nil)
	if err := w.Client().Mount(gunim.Root, "bar", "bar", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	x0 := float32(600 - 3*windowButtonWidth)
	if want := geom.Rc(x0+windowButtonWidth, 0, windowButtonWidth, 30); fr.Maximize != want {
		t.Fatalf("the system was told the maximize button is at %v, want %v", fr.Maximize, want)
	}
	click := func(i int) {
		p := geom.Pt(x0+float32(i)*windowButtonWidth+20, 15)
		w.Input(input.PointerMove{Pos: p, Time: time.Now()})
		w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
		w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Time: time.Now()})
		run(1)
	}
	click(0)
	if fr.Minimized != 1 {
		t.Fatalf("minimize minimized %d times", fr.Minimized)
	}
	click(1)
	if !fr.IsMaximized {
		t.Fatal("maximize left the window as it was")
	}
	click(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Client().Input(ctx, input.PointerMove{}); !errors.Is(err, gunim.ErrWindowClosed) {
		t.Fatalf("after close, the window takes input: %v", err)
	}
}
