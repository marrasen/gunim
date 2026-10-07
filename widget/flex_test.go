package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestARowWrapsALongLabelSoTheButtonAfterItStaysInside(t *testing.T) {
	l := NewLabel(strings.Repeat("word ", 14))
	b := NewButton("Go")
	b.On = pressed{1}
	w, run := stage(t, &frame{child: Row(l, b), size: geom.Sz(200, 300)})
	if b.size.W <= 0 {
		t.Fatalf("the button is %v wide, want it shown", b.size.W)
	}
	if len(l.laid.p.Lines) < 2 {
		t.Fatalf("the label laid out on %d lines, want it wrapped", len(l.laid.p.Lines))
	}
	// The button's middle is where a click reaches it.
	click(w, 199-b.size.W/2, b.size.H/2)
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a click on the button's place sent %v, want one intent", got)
	}
}

func TestARowShrinksChildrenInProportionToTheirSize(t *testing.T) {
	a, b := newSpot(300, 20), newSpot(100, 20)
	_, run := stage(t, &frame{child: Row(a, b), size: geom.Sz(200, 50)})
	run(1)
	room := 200 - Gap.Default()
	if want := room * 3 / 4; abs32(a.box.W-want) > 0.5 {
		t.Fatalf("the 300 px spot shrank to %v, want %v", a.box.W, want)
	}
	if want := room / 4; abs32(b.box.W-want) > 0.5 {
		t.Fatalf("the 100 px spot shrank to %v, want %v", b.box.W, want)
	}
	if end := b.at.X + b.box.W; end > 200.5 {
		t.Fatalf("the second spot ends at %v, past the row's 200 px", end)
	}
}

func TestARowSqueezesALabelNoNarrowerThanItsWidestWord(t *testing.T) {
	l := NewLabel("Fruit")
	s := newSpot(200, 20)
	_, run := stage(t, &frame{child: Row(l, s), size: geom.Sz(220, 50)})
	run(1)
	if n := len(l.laid.p.Lines); n != 1 {
		t.Fatalf("the label broke its one word over %d lines", n)
	}
	if end := s.at.X + s.box.W; end > 220.5 {
		t.Fatalf("the spot ends at %v, past the row's 220 px", end)
	}
}

func TestAColumnShrinksToo(t *testing.T) {
	a, b := newSpot(20, 300), newSpot(20, 300)
	_, run := stage(t, &frame{child: Column(a, b), size: geom.Sz(50, 100)})
	run(1)
	if end := b.at.Y + b.box.H; end > 100.5 || b.box.H <= 0 {
		t.Fatalf("the second spot spans %v to %v, want it inside the column's 100 px", b.at.Y, end)
	}
}

func TestChildrenPastAFullRowStayInsideIt(t *testing.T) {
	// Ten children whose gaps alone outgrow the row still end inside it.
	spots := make([]*spot, 0, 10)
	row := Row()
	for range 10 {
		s := newSpot(30, 20)
		spots = append(spots, s)
		row.kids = append(row.kids, s)
	}
	_, run := stage(t, &frame{child: row, size: geom.Sz(40, 50)})
	run(1)
	for i, s := range spots {
		if end := s.at.X + s.box.W; end > 40.5 {
			t.Fatalf("child %d ends at %v, past the row's 40 px", i, end)
		}
	}
}

func TestAGrowingChildSqueezedOutStaysBounded(t *testing.T) {
	fixed := newSpot(250, 20)
	grower := newSpot(1000, 20)
	row := Row(fixed, grower)
	row.Grow(grower, 1)
	_, run := stage(t, &frame{child: row, size: geom.Sz(200, 50)})
	run(1)
	if grower.at.X > 200 {
		t.Fatalf("the growing child starts at %v, past the row's 200 px", grower.at.X)
	}
	if grower.box.W <= 0 || grower.box.W > 1 {
		t.Fatalf("the growing child was laid out %v wide, want a sliver", grower.box.W)
	}
	if fixed.box.W > 200 {
		t.Fatalf("the fixed child is %v wide, past the row's 200 px", fixed.box.W)
	}
}
