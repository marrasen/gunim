package gunim

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// coverPanel is a node over the window whose own part is at.
type coverPanel struct {
	recorder
	at geom.Rect
}

func (c *coverPanel) CoversCaption(geom.Size) []geom.Rect { return []geom.Rect{c.at} }

// A node painted over the title bar takes its part out of the caption:
// a press there reaches the node and moves no window, and the caption
// round it still moves the window.
func TestACoverTakesItsPartOutOfTheCaption(t *testing.T) {
	w, fr, _ := chromelessStage(t, false)
	cover := &coverPanel{at: geom.Rc(150, 0, 100, 20)}
	w.ui.Insert(w.ui.Root(), cover)
	run(w, 2)
	want := []geom.Rect{geom.Rc(100, 20, 200, 10), geom.Rc(100, 0, 50, 20), geom.Rc(250, 0, 50, 20)}
	if !slices.Equal(fr.Caption(), want) || fr.MaximizeButton() != geom.Rc(300, 0, 46, 30) {
		t.Fatalf("the system was told the caption is %v and the maximize button %v, want %v round the cover", fr.Caption(), fr.MaximizeButton(), want)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(200, 10), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	if fr.Moves != 0 || len(cover.events) == 0 {
		t.Fatalf("a press on the cover started %d moves, and the cover saw %v", fr.Moves, cover.events)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(200, 10), Button: input.ButtonPrimary, Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(120, 10), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	if fr.Moves != 1 {
		t.Fatalf("a press on the caption beside the cover started %d moves", fr.Moves)
	}
	// Moved away, the cover leaves the caption whole again.
	cover.at = geom.Rc(400, 300, 100, 100)
	w.ui.Invalidate()
	run(w, 2)
	if !slices.Equal(fr.Caption(), []geom.Rect{geom.Rc(100, 0, 200, 30)}) {
		t.Fatalf("with the cover moved away the caption is %v", fr.Caption())
	}
}

// A cover that reaches the maximize button has the system told of none.
func TestACoverOverTheMaximizeButtonHidesIt(t *testing.T) {
	w, fr, _ := chromelessStage(t, true)
	cover := &coverPanel{at: geom.Rc(280, 0, 40, 30)}
	w.ui.Insert(w.ui.Root(), cover)
	run(w, 2)
	if !slices.Equal(fr.Caption(), []geom.Rect{geom.Rc(100, 0, 180, 30)}) || !fr.MaximizeButton().Empty() {
		t.Fatalf("the system was told the caption is %v and the maximize button %v, want the caption left of the cover and no button",
			fr.Caption(), fr.MaximizeButton())
	}
}

// The parts of a rectangle outside another are the rectangle itself
// where they don't meet, none where it is covered whole, and the
// strips round the other where it lies inside.
func TestThePartsOfARectangleOutsideAnother(t *testing.T) {
	r := geom.Rc(0, 0, 100, 40)
	for _, c := range []struct {
		cut  geom.Rect
		want []geom.Rect
	}{
		{geom.Rc(200, 0, 50, 40), []geom.Rect{r}},
		{geom.Rc(100, 0, 50, 40), []geom.Rect{r}},
		{geom.Rc(-10, -10, 200, 200), nil},
		{geom.Rc(40, 10, 20, 20), []geom.Rect{geom.Rc(0, 0, 100, 10), geom.Rc(0, 30, 100, 10), geom.Rc(0, 10, 40, 20), geom.Rc(60, 10, 40, 20)}},
		{geom.Rc(-50, -50, 100, 100), []geom.Rect{geom.Rc(50, 0, 50, 40)}},
	} {
		if got := appendOutside(nil, r, c.cut); !slices.Equal(got, c.want) {
			t.Errorf("%v outside %v is %v, want %v", r, c.cut, got, c.want)
		}
	}
}
