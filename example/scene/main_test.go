package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// tapAt taps the window at p, where the stage fills it.
func tapAt(w interface{ Input(any) }, p geom.Point) {
	now := time.Now()
	w.Input(input.PointerDown{Pos: p, Time: now})
	w.Input(input.PointerUp{Pos: p, Time: now})
}

func TestATapOnTheHatOrTheHeadPicksIt(t *testing.T) {
	s := newStage()
	w := gunimtest.New(t, geom.Sz(420, 720), widget.NewSurface())
	gunim.RegisterView(w, "stage", func(struct{}) *stage { return s }, nil)
	if err := w.Client().Mount(gunim.Root, "stage", "stage", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	w.Frame(time.Second / 60)
	sc := s.scene()
	v := s.view()
	m := sc.Camera.Matrix(v.Size().W / v.Size().H)
	// shows returns where an item's own point shows in the window.
	shows := func(item int, at geom.Vec3) geom.Point {
		p := m.Apply(sc.Items[item].Model.Apply(at))
		return geom.Pt(v.Min.X+(p.X+1)/2*v.Size().W, v.Min.Y+(1-p.Y)/2*v.Size().H)
	}
	tapAt(w, shows(itemCrown, geom.V3(0, 0, 0.6)))
	if s.hat != 1 {
		t.Errorf("after a tap on the hat its colour is %d, want the next, 1", s.hat)
	}
	tapAt(w, shows(itemHead, geom.V3(0, -0.4, 0.9)))
	if !s.bubbled {
		t.Error("after a tap on the head there is no bubble round it")
	}
	w.Frame(time.Second / 60)
	tapAt(w, shows(itemHead, geom.V3(0, -0.4, 0.9)))
	if s.bubbled {
		t.Error("after a tap on the bubble it stays round the head")
	}
	tapAt(w, geom.Pt(v.Min.X+8, v.Max.Y-8))
	if s.hat != 1 || s.bubbled {
		t.Errorf("a tap on the view's empty corner changed the hat to %d and the bubble to %v", s.hat, s.bubbled)
	}
}
