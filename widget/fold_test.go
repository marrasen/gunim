package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

func TestAFoldGlidesShutAndOpenAndTakesNoPointerShut(t *testing.T) {
	f := NewFold(NewButton("Inside"), true)
	w, run := stage(t, &frame{child: Column(f, &recorder{}), size: geom.Sz(300, 300)})
	run(1)
	open := f.box.H
	if open <= 0 {
		t.Fatalf("an open fold is %v tall", open)
	}
	do(t, w, func(u *gunim.UI) { f.SetOpen(false, u) })
	run(3)
	if h := f.box.H; h <= 0 || h >= open {
		t.Fatalf("three frames into shutting, the fold is %v tall; want on its way from %v", h, open)
	}
	run(60)
	if f.box.H != 0 || f.Open() {
		t.Fatalf("a shut fold is %v tall", f.box.H)
	}
	if f.Covers(geom.Pt(5, 5)) {
		t.Fatal("a shut fold takes the pointer")
	}
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	run(60)
	if f.box.H != open || !f.Covers(geom.Pt(5, 5)) {
		t.Fatalf("opened again, the fold is %v tall; want %v", f.box.H, open)
	}
}

func TestAFoldMadeShutTakesNoRoom(t *testing.T) {
	f := NewFold(NewButton("Inside"), false)
	_, run := stage(t, &frame{child: Column(f), size: geom.Sz(300, 300)})
	run(2)
	if f.box.H != 0 {
		t.Fatalf("a fold made shut is %v tall", f.box.H)
	}
}
