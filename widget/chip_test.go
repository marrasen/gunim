package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

type removed struct{ Label string }

type picked struct{ Item int }

func TestAClickOnAChipsCrossRemovesIt(t *testing.T) {
	c := NewChip("level", "error")
	c.OnRemove = func() gunim.Intent { return removed{"error"} }
	w, run := stage(t, &frame{child: Row(c), size: geom.Sz(400, 40)})
	click(w, 5, 12)
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a click on the label sent %v, want nothing", got)
	}
	click(w, c.crossX, 12)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (removed{"error"}) {
		t.Fatalf("a click on the cross sent %v, want the chip removed", got)
	}
}

func TestALongChipInANarrowWrapKeepsItsCrossInside(t *testing.T) {
	c := NewChip("path", "/home/someone/a/folder/with/a/very/long/name/indeed/and/more/yet")
	c.Icon = icon.Folder
	c.OnRemove = func() gunim.Intent { return removed{"path"} }
	w, run := stage(t, &frame{child: NewWrap(c), size: geom.Sz(200, 100)})
	run(1)
	if c.size.W > 200 {
		t.Fatalf("in a 200 px wrap the chip is %v wide", c.size.W)
	}
	if bad := spills(painted(c, c.size), c.size, 0); len(bad) > 0 {
		t.Fatalf("the chip drew past its %v box: %v", c.size, bad)
	}
	click(w, c.size.W-c.size.H/2, c.size.H/2)
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a click on the cross at the chip's end sent %v, want the chip removed", got)
	}
}

func TestAWrapStartsANewRowWhereTheNextChildWouldPassItsWidth(t *testing.T) {
	a, b, c := newSpot(150, 20), newSpot(150, 30), newSpot(150, 20)
	wrap := NewWrap()
	wrap.Gap = tableNoGap
	w := gunimtest.New(t, geom.Sz(320, 200), nil)
	gunim.RegisterView(w, "wrap", func(struct{}) gunim.Node { return wrap },
		func(n gunim.Node, _ struct{}, u *gunim.UI) {
			for _, k := range []gunim.Node{a, b, c} {
				u.Insert(n, k)
			}
		})
	if err := w.Client().Mount(gunim.Root, "wrap", "wrap", nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		w.Frame(time.Second / 60)
	}
	at(t, a, geom.Pt(0, 0))
	at(t, b, geom.Pt(150, 0))
	// The row is as tall as its tallest child.
	at(t, c, geom.Pt(0, 30))
}

func TestAWrapCanCentreEachChildInItsRow(t *testing.T) {
	a, b, c := newSpot(150, 20), newSpot(150, 30), newSpot(150, 20)
	wrap := NewWrap()
	wrap.Gap, wrap.Cross = tableNoGap, CrossCenter
	w := gunimtest.New(t, geom.Sz(320, 200), nil)
	gunim.RegisterView(w, "wrap", func(struct{}) gunim.Node { return wrap },
		func(n gunim.Node, _ struct{}, u *gunim.UI) {
			for _, k := range []gunim.Node{a, b, c} {
				u.Insert(n, k)
			}
		})
	if err := w.Client().Mount(gunim.Root, "wrap", "wrap", nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		w.Frame(time.Second / 60)
	}
	at(t, a, geom.Pt(0, 5))
	at(t, b, geom.Pt(150, 0))
	at(t, c, geom.Pt(0, 30))
}

func TestAMenuButtonThatStaysOpenTicksWhatIsPicked(t *testing.T) {
	b := NewMenuButton("Files", "app.log", "app.log.1", "app.log.2")
	b.StayOpen = true
	b.OnPick = func(i int) gunim.Intent { return picked{i} }
	w, run := stage(t, &frame{child: Row(b), size: geom.Sz(400, 300)})
	click(w, 10, 10)
	run(10)
	if !b.IsOpen() {
		t.Fatal("a click did not open the menu")
	}
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if !b.IsOpen() {
		t.Fatal("a pick closed a menu that stays open")
	}
	if len(b.Checked) < 2 || !b.Checked[1] || b.menu.Checked[1] != true {
		t.Fatalf("after picking the second item, ticks are %v, want it ticked", b.Checked)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if b.Checked[1] {
		t.Fatal("picking a ticked item left it ticked")
	}
	if got := sent(w); len(got) != 2 || got[0] != (picked{1}) || got[1] != (picked{1}) {
		t.Fatalf("intents %v, want two picks of item 1", got)
	}
}

func TestAPaletteOffersTypedItemsFirst(t *testing.T) {
	w, o, run := newPaletteStage(t)
	o.p.Typed = func(q string) []PaletteItem {
		if q == "" {
			return nil
		}
		return []PaletteItem{{Title: "path = " + q}}
	}
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(20)
	w.Input(input.TextInput{Text: "sp"})
	run(20)
	f := o.p.card.found
	if len(f) < 2 || f[0].Index != len(o.p.Items) {
		t.Fatalf("typing sp found %v, want the typed item first", f)
	}
	if got := o.p.item(f[0].Index).Title; got != "path = sp" {
		t.Fatalf("the typed item is %q, want path = sp", got)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(20)
	if len(o.picked) != 1 || o.picked[0] != len(o.p.Items) {
		t.Fatalf("picked %v, want the typed item at %d", o.picked, len(o.p.Items))
	}
}

// Picked hears a pick on the UI goroutine, for a pick that works in the
// window.
func TestAMenuButtonsPickedHearsThePick(t *testing.T) {
	b := NewMenuButton("Add", "Server", "Window")
	got := -1
	b.Picked = func(i int, u *gunim.UI) { got = i }
	w, run := stage(t, &frame{child: Row(b), size: geom.Sz(400, 300)})
	click(w, 10, 10)
	run(10)
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if got != 1 || b.IsOpen() {
		t.Fatalf("picked %d, open %v; want the second, closed", got, b.IsOpen())
	}
}
