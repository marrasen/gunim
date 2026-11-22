package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// apartRow is a steadyRow whose key holds what happens elsewhere in the window.
type apartRow struct{ *steadyRow }

func (apartRow) SteadyApart() bool { return true }

// apartStage mounts two apartRows and a plain steadyRow, each with a steadyLeaf in it, beside a ticker, and lets
// the window settle.
func apartStage(t *testing.T) (w *Window, a, b apartRow, plain *steadyRow, leafA *steadyLeaf) {
	t.Helper()
	w = newTestWindow()
	a, b = apartRow{&steadyRow{key: 1, ok: true, colour: rowBlue}}, apartRow{&steadyRow{key: 1, ok: true, colour: rowBlue}}
	plain = &steadyRow{key: 1, ok: true, colour: rowBlue}
	leafA = &steadyLeaf{}
	for _, r := range []Node{a, b, plain} {
		w.ui.Insert(w.ui.Root(), r)
	}
	w.ui.Insert(a, leafA)
	w.ui.Insert(b, &steadyLeaf{})
	w.ui.Insert(plain, &steadyLeaf{})
	w.ui.Insert(w.ui.Root(), &ticker{})
	run(w, 60)
	return w, a, b, plain, leafA
}

func TestASteadyApartNodeIsCarriedThroughWhatHappensElsewhere(t *testing.T) {
	w, a, b, plain, leafA := apartStage(t)
	u := w.ui
	frame := func(do func()) (aDrawn, bDrawn, plainDrawn bool) {
		run(w, 5)
		pa, pb, pp := a.painted, b.painted, plain.painted
		do()
		run(w, 1)
		return a.painted != pa, b.painted != pb, plain.painted != pp
	}

	if aDrawn, bDrawn, plainDrawn := frame(func() { u.on(u.index[leafA], func() {}) }); !aDrawn || bDrawn || !plainDrawn {
		t.Errorf("a handler in one row: that row drawn %v, the other %v, the plain row %v; want true, false, true",
			aDrawn, bDrawn, plainDrawn)
	}
	if aDrawn, bDrawn, _ := frame(func() { u.on(u.index[leafA], func() { u.Invalidate() }) }); !aDrawn || bDrawn {
		t.Errorf("Invalidate from a handler in one row: that row drawn %v, the other %v", aDrawn, bDrawn)
	}
	if aDrawn, bDrawn, _ := frame(func() { u.hear(u.index[leafA], func() {}) }); aDrawn || bDrawn {
		t.Errorf("input overheard in one row: drawn %v and %v, want both carried", aDrawn, bDrawn)
	}
	if aDrawn, bDrawn, _ := frame(func() { w.Input(input.KeyPress{Key: input.KeyF13}) }); aDrawn || bDrawn {
		t.Errorf("a key no node took: drawn %v and %v, want both carried", aDrawn, bDrawn)
	}
	if aDrawn, bDrawn, plainDrawn := frame(func() { u.Invalidate() }); !aDrawn || !bDrawn || !plainDrawn {
		t.Error("Invalidate from no node carried a row")
	}
	run(w, 5)
	u.on(u.index[leafA], func() { u.After(40*time.Millisecond, func(*UI) {}) })
	run(w, 1)
	pa, pb := a.painted, b.painted
	run(w, 5)
	if a.painted == pa || b.painted != pb {
		t.Errorf("a row's timer: that row drawn %d times, the other %d; want it drawn, the other carried",
			a.painted-pa, b.painted-pb)
	}
	if aDrawn, bDrawn, _ := frame(func() { u.on(u.index[leafA], func() {}); b.key = 2 }); !aDrawn || !bDrawn {
		t.Error("a row whose key changed was carried")
	}
}
