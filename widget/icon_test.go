package widget

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// maskOps returns the mask ops in the window's last frame.
func maskOps(w interface{ Ops() []paint.Op }) []*paint.MaskOp {
	var out []*paint.MaskOp
	for _, op := range w.Ops() {
		if m, ok := op.(*paint.MaskOp); ok {
			out = append(out, m)
		}
	}
	return out
}

// strokeOf returns the icon stroke a mask op draws.
func strokeOf(t *testing.T, m *paint.MaskOp) icon.Stroke {
	t.Helper()
	s, ok := m.Shape.(icon.Stroke)
	if !ok {
		t.Fatalf("the mask draws a %T, want an icon stroke", m.Shape)
	}
	return s
}

func TestAnIconTakesItsSizeAndSaysItsName(t *testing.T) {
	i := NewIcon(icon.Funnel, "Filter")
	w, _ := stage(t, Row(i))
	ms := maskOps(w.Offscreen())
	if len(ms) != 1 {
		t.Fatalf("the icon drew %d masks, want one", len(ms))
	}
	if r := ms[0].Rect; r != geom.Rc(0, 0, 16, 16) {
		t.Errorf("the icon drew into %v, want 16 square at its origin", r)
	}
	if s := strokeOf(t, ms[0]); s.Icon != icon.Funnel || s.Width != 2 || !s.Settled() {
		t.Errorf("the icon drew %+v, want funnel stroked 2 wide and whole", s)
	}
	if ms[0].Color != Ink.Default() {
		t.Errorf("the icon is %v, want the text colour", ms[0].Color)
	}
	if info := i.Access(); info.Role != access.RoleImage || info.Name != "Filter" {
		t.Errorf("the icon reads as %v %q", info.Role, info.Name)
	}
	if info := NewIcon(icon.Funnel, "").Access(); info.Role != access.RoleGroup || info.Name != "" {
		t.Errorf("an icon without a name reads as %v %q, want nothing", info.Role, info.Name)
	}
}

// A new colour tints the same shape, so the renderer keeps the mask it has.
func TestAnIconRecolouredKeepsItsShape(t *testing.T) {
	i := NewIcon(icon.Check, "Done")
	w, run := stage(t, Row(i))
	before := maskOps(w.Offscreen())[0]
	shape := before.Shape
	red := color.NRGBA{R: 0xff, A: 0xff}
	w.RegisterTheme(theme.Make("red", theme.Set(Ink, red)))
	if err := w.Client().SetTheme("red"); err != nil {
		t.Fatal(err)
	}
	run(120)
	after := maskOps(w.Offscreen())[0]
	if after.Color != red || after.Shape != shape {
		t.Fatalf("recoloured, the icon drew %v in %v, want the same shape in red", after.Shape, after.Color)
	}
}

func TestAnIconDrawsItselfOn(t *testing.T) {
	i := NewIcon(icon.CircleCheck, "Saved")
	w, run := stage(t, Row(i))
	i.DrawOn(500 * time.Millisecond)
	run(1)
	ms := maskOps(w.Offscreen())
	if len(ms) != 1 {
		t.Fatalf("drawing on, the icon drew %d masks, want one", len(ms))
	}
	first := strokeOf(t, ms[0])
	if first.Settled() || first.Progress <= 0 || first.Progress > 0.1 {
		t.Fatalf("a frame into drawing on, progress is %v", first.Progress)
	}
	run(15)
	if mid := strokeOf(t, maskOps(w.Offscreen())[0]); mid.Settled() || mid.Progress <= first.Progress {
		t.Fatalf("a quarter of a second in, progress is %v", mid.Progress)
	}
	run(30)
	if last := strokeOf(t, maskOps(w.Offscreen())[0]); !last.Settled() {
		t.Fatalf("after the draw-on, progress is %v, want 1", last.Progress)
	}
}

func TestASpinningIconTurns(t *testing.T) {
	i := NewIcon(icon.Loader2, "Loading")
	i.Spin = true
	w, run := stage(t, Row(i))
	run(15)
	m := maskOps(w.Offscreen())[0]
	if m.Transform.B == 0 {
		t.Fatalf("a quarter second in, the spinner is not turned: %+v", m.Transform)
	}
	if !strokeOf(t, m).Settled() {
		t.Fatal("the spinner rasterizes anew as it turns, want its mask kept")
	}
	if !i.Step(time.Millisecond) {
		t.Fatal("a spinning icon says it has settled")
	}
}

// A spinner on a page out of sight lets the window rest.
func TestASpinnerOutOfSightStopsAskingForFrames(t *testing.T) {
	i := NewIcon(icon.Loader2, "Loading")
	i.Spin = true
	_, run := stage(t, NewTabs([]string{"Shown", "Hidden"}, NewLabel("Here"), i))
	run(5)
	if i.Step(time.Second / 60) {
		t.Fatal("a spinner on a hidden page still asks for frames")
	}
}
