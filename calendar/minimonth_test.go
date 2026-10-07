package calendar

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
)

func TestAMiniMonthPicksOnceAsThePrimaryButtonLetsGo(t *testing.T) {
	m := NewMiniMonth(monday)
	var picks []time.Time
	m.Pick = func(day time.Time, _ *gunim.UI) { picks = append(picks, day) }
	w, run, _ := stage(t, m)
	at := m.cell(10).Center()
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	run(1)
	if len(picks) != 0 {
		t.Fatalf("a press picked %v before the pointer let go", picks)
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonSecondary})
	if len(picks) != 0 {
		t.Fatal("the secondary button, let go, picked a day")
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 2})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(1)
	if len(picks) != 1 || !picks[0].Equal(m.day(10)) {
		t.Fatalf("a double click picked %v, want the day under it once", picks)
	}
	// Let go over another day, the press picks nothing.
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: m.cell(20).Center(), Button: input.ButtonPrimary})
	run(1)
	if len(picks) != 1 {
		t.Fatalf("a press let go over another day picked %v", picks[1:])
	}
	// Each click on an arrow turns a month.
	_, next := m.arrows()
	was := m.month
	for clicks := 1; clicks <= 2; clicks++ {
		w.Input(input.PointerDown{Pos: next.Center(), Button: input.ButtonPrimary, Clicks: clicks})
		w.Input(input.PointerUp{Pos: next.Center(), Button: input.ButtonPrimary})
	}
	run(1)
	if want := was.AddDate(0, 2, 0); !m.month.Equal(want) {
		t.Fatalf("a double click on the next arrow shows %v, want %v", m.month, want)
	}
}
