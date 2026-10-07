package calendar

import (
	"strconv"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// glyphs returns where the glyphs of the last frame of ops start, each with its line's baseline, in the window's
// space.
func glyphs(ops []paint.Op) []geom.Point {
	var out []geom.Point
	for _, op := range ops {
		if t, ok := op.(*paint.TextOp); ok {
			for _, g := range t.Glyphs {
				out = append(out, t.Transform.Apply(g.At))
			}
		}
	}
	return out
}

func TestADateFieldKeepsItsDayLeftOfItsIcon(t *testing.T) {
	for _, width := range []float32{60, 100, 130, 170} {
		f := NewDateField(monday)
		w, run, _ := stage(t, &frame{child: f, size: geom.Sz(width, 32)})
		for frame := range 3 {
			run(1)
			icon := width - 16 - 8
			line := float32(-1)
			for _, g := range glyphs(w.Offscreen().Ops()) {
				if g.X >= icon {
					t.Fatalf("%v wide, frame %d: a glyph of the day starts at %v, past the icon at %v", width, frame, g.X,
						icon)
				}
				if line >= 0 && g.Y != line {
					t.Fatalf("%v wide, frame %d: the day runs onto a second line at %v", width, frame, g.Y)
				}
				line = g.Y
			}
		}
	}
}

func TestASmallMonthKeepsToItsBox(t *testing.T) {
	for _, size := range []geom.Size{geom.Sz(300, 150), geom.Sz(150, 400), geom.Sz(120, 120), geom.Sz(20, 20)} {
		name := strconv.Itoa(int(size.W)) + " by " + strconv.Itoa(int(size.H))
		m := NewMiniMonth(monday)
		w, run, _ := stage(t, &frame{child: m, size: size})
		for f := range 3 {
			run(1)
			if m.side < 0 {
				t.Fatalf("%s, frame %d: the days are %v wide", name, f, m.side)
			}
			last := m.cell(41)
			if size.W >= miniWeekW+7 && size.H >= miniHeadH+22+6 && (last.Max.X > size.W || last.Max.Y > size.H) {
				t.Fatalf("%s, frame %d: the last day is at %v, outside the month", name, f, last)
			}
			back, _ := m.arrows()
			for _, g := range glyphs(w.Offscreen().Ops()) {
				if g.Y < back.Max.Y && g.X >= back.Min.X-4 && g.X < back.Min.X {
					t.Fatalf("%s, frame %d: the month's name runs into its arrows at %v", name, f, g)
				}
				if g.Y < back.Max.Y && g.X >= back.Min.X && g.X < back.Max.X+28 && back.Min.X > 8 {
					t.Fatalf("%s, frame %d: the month's name runs under its arrows at %v", name, f, g)
				}
			}
		}
	}
}

func TestADateFieldsMonthFitsAShortScreen(t *testing.T) {
	f := NewDateField(monday)
	w, run, _ := stage(t, &frame{child: f, size: geom.Sz(170, 32)})
	// A phone on its side: 260 pixels tall, the field at the top.
	w.Offscreen().SetWorkArea(geom.Rc(0, 0, 800, 260))
	w.Input(input.PointerDown{Pos: geom.Pt(20, 16), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 16), Button: input.ButtonPrimary})
	for frame := range 10 {
		run(1)
		if f.popup == nil {
			t.Fatal("a click on the field opened nothing")
		}
		pw := f.popup.Offscreen()
		if pw == nil {
			continue
		}
		if h, a := pw.Size().H, pw.Anchor(); h > max(260-a.Max.Y, a.Min.Y) {
			t.Fatalf("frame %d: the month is %v tall under %v, past the foot of the screen at 260", frame, h, a)
		}
	}
	if pw := f.popup.Offscreen(); pw == nil || pw.Size().H <= 0 {
		t.Fatal("the month never showed")
	}
}
