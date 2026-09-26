package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type chose struct{ I int }

func newPicker(t *testing.T) (*gunim.Window, func(int), *Dropdown) {
	t.Helper()
	d := NewDropdown("Apple", "Banana", "Cherry")
	d.OnChange = func(i int) gunim.Intent { return chose{i} }
	w, run := stage(t, &frame{child: d, size: geom.Sz(200, 36)})
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18)})
	run(1)
	return w, run, d
}

func TestDropdownOpensOnClickAndClosesOnSecondClick(t *testing.T) {
	w, run, d := newPicker(t)
	if !d.IsOpen() {
		t.Fatal("a click did not open the list")
	}
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Clicks: 1})
	run(1)
	if d.IsOpen() {
		t.Fatal("a second click on the drop-down left the list open")
	}
}

func TestDropdownPicksWithTheKeyboard(t *testing.T) {
	w, run, d := newPicker(t)
	for _, k := range []input.Key{input.KeyDown, input.KeyDown, input.KeyEnter} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	if d.IsOpen() || d.Selected != 2 {
		t.Fatalf("open %v, selected %d; want closed on 2", d.IsOpen(), d.Selected)
	}
	select {
	case e := <-w.Client().Intents():
		if e.Intent != (chose{2}) {
			t.Fatalf("intent %v, want chose{2}", e.Intent)
		}
	default:
		t.Fatal("no intent for the change")
	}
}

func TestAPickRunsInTheWindowToo(t *testing.T) {
	w, run, d := newPicker(t)
	got := -1
	d.OnPick(func(i int, _ *gunim.UI) { got = i })
	for _, k := range []input.Key{input.KeyDown, input.KeyEnter} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	if got != 1 {
		t.Fatalf("picking Banana told the window %d", got)
	}
}

func TestDropdownClosesOnEscapeAndOutsideClick(t *testing.T) {
	w, run, d := newPicker(t)
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	if d.IsOpen() {
		t.Fatal("Escape left the list open")
	}
	w.Input(input.KeyPress{Key: input.KeySpace})
	run(1)
	if !d.IsOpen() {
		t.Fatal("Space did not open the list")
	}
	w.Input(input.PointerDown{Pos: geom.Pt(500, 400), Clicks: 1})
	run(1)
	if d.IsOpen() {
		t.Fatal("a click outside left the list open")
	}
}

func TestTooltipShowsAfterItsDelay(t *testing.T) {
	b := NewButton("Hover me")
	tip := NewTooltip(b, "Hello")
	w, run := stage(t, &frame{child: tip, size: geom.Sz(200, 36)})
	w.Input(input.PointerMove{Pos: geom.Pt(20, 18), Time: time.Now()})
	run(1)
	if tip.popup != nil {
		t.Fatal("the tooltip showed at once")
	}
	run(60) // a second
	if tip.popup == nil {
		t.Fatal("the tooltip did not show after its delay")
	}
	w.Input(input.PointerLeave{Time: time.Now()})
	run(1)
	if tip.popup != nil {
		t.Fatal("the tooltip stayed after the pointer left")
	}
}
