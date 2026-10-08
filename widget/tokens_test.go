package widget

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// paintedIn returns the ops n paints into box under th.
func paintedIn(n gunim.Node, box geom.Size, th *theme.Live) []paint.Op {
	var p paint.Painter
	n.Paint(&p, gunim.Frame{Scale: 1, Theme: th}, box, gunim.Children{})
	return p.Ops()
}

// rrects returns the rounded rectangles among ops.
func rrects(ops []paint.Op) []*paint.RRectOp {
	var out []*paint.RRectOp
	for _, op := range ops {
		if r, ok := op.(*paint.RRectOp); ok {
			out = append(out, r)
		}
	}
	return out
}

// The looks the widgets once drew in fixed values come from the theme: the knobs' shadow, the ring round a link
// and a tab's title, the histogram's channels and the live graph's head.
func TestTheLooksComeFromTheTheme(t *testing.T) {
	shadow := color.NRGBA{R: 1, G: 2, B: 3, A: 0x33}
	head := color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}
	th := theme.NewLive(theme.Make("test",
		theme.Set(KnobShadow, shadow),
		theme.Set(FocusRadius, 11),
		theme.Set(HistogramRed, color.NRGBA{A: 0xff}),
		theme.Set(HistogramGreen, color.NRGBA{G: 0xff, A: 0xff}),
		theme.Set(HistogramBlue, color.NRGBA{A: 0xff}),
		theme.Set(LiveGraphHead, head),
	))
	hasShadow := func(ops []paint.Op) bool {
		for _, r := range rrects(ops) {
			if r.Shadow.Color == shadow {
				return true
			}
		}
		return false
	}
	if !hasShadow(paintedIn(NewSwitch(""), geom.Sz(40, 28), th)) {
		t.Fatal("a switch's knob casts a shadow other than KnobShadow")
	}
	if !hasShadow(paintedIn(NewSlider(0, 1), geom.Sz(200, 28), th)) {
		t.Fatal("a slider's knob casts a shadow other than KnobShadow")
	}

	// The ring sits 3 past the control, and rounds that much wider.
	ringed := func(ops []paint.Op) bool {
		for _, r := range rrects(ops) {
			if r.Stroke.Width == 2 && r.Radius == 11+3 {
				return true
			}
		}
		return false
	}
	l := NewLink("More")
	l.ring.Jump(1)
	if !ringed(paintedIn(l, geom.Sz(60, 18), th)) {
		t.Fatal("a link's focus ring is rounded other than by FocusRadius")
	}
	tabs := NewTabs([]string{"One", "Two"}, NewLabel("1"), NewLabel("2"))
	_, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	run(1)
	tabs.ring.Jump(1)
	if !ringed(paintedIn(tabs.bar, geom.Sz(300, tabs.head), th)) {
		t.Fatal("a tab's focus ring is rounded other than by FocusRadius")
	}

	// Green alone, at its height, mixes only the theme's green into the fill.
	h := NewHistogram()
	var counts [3][256]uint32
	for i := range 256 {
		counts[1][i] = 100
	}
	hw, hrun := stage(t, &frame{child: h, size: geom.Sz(256, 64)})
	do(t, hw, func(u *gunim.UI) { h.SetCounts(counts, u) })
	hrun(1)
	h.t.Jump(1)
	want := lightMix(HistogramFill.Get(th), [3]color.NRGBA{{A: 0xff}, {G: 0xff, A: 0xff}, {A: 0xff}}, [3]bool{false, true, false})
	found := false
	for _, r := range rrects(paintedIn(h, geom.Sz(256, 64), th)) {
		found = found || r.Fill.Solid == want
	}
	if !found {
		t.Fatalf("the histogram's green is drawn other than in %v", want)
	}

	g := NewLiveGraph(200*time.Millisecond, 50)
	for _, v := range []float64{1, 2, 3} {
		g.Add(v)
		g.Step(time.Second / 5)
	}
	found = false
	for _, r := range rrects(paintedIn(g, geom.Sz(400, 56), th)) {
		found = found || r.Fill.Solid == head
	}
	if !found {
		t.Fatal("the live graph's head is drawn other than in LiveGraphHead")
	}
}
