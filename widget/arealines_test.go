package widget

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/text"
)

// sameAsWhole fails t unless a answers as rs laid out whole does.
func sameAsWhole(t *testing.T, a *areaText, rs []rune, face *text.Face, st text.Style, width float32, what string) {
	t.Helper()
	whole := face.Layout(string(rs), st, width)
	if a.count() != len(whole.Lines) || a.Size != whole.Size || a.LineHeight != whole.LineHeight {
		t.Fatalf("%s: %d lines in %v, want %d in %v", what, a.count(), a.Size, len(whole.Lines), whole.Size)
	}
	for n, l := range whole.Lines {
		run, base, at := a.line(n)
		if run.Start+base != l.Run.Start || run.End+base != l.Run.End || at != l.At || run.Advance != l.Run.Advance {
			t.Fatalf("%s: line %d holds runes %d to %d at %v, want %d to %d at %v",
				what, n, run.Start+base, run.End+base, at, l.Run.Start, l.Run.End, l.At)
		}
	}
	for i := 0; i <= len(rs); i++ {
		gl, gat := a.Caret(i)
		wl, wat := whole.Caret(i)
		if gl != wl || gat != wat {
			t.Fatalf("%s: a caret before rune %d is on line %d at %v, want line %d at %v", what, i, gl, gat, wl, wat)
		}
	}
	for y := float32(-3); y < whole.Size.H+10; y += 7 {
		for x := float32(-3); x < width+10; x += 13 {
			if got, want := a.Index(geom.Pt(x, y)), whole.Index(geom.Pt(x, y)); got != want {
				t.Fatalf("%s: a press at %v, %v is before rune %d, want %d", what, x, y, got, want)
			}
		}
	}
}

func TestAnAreaLaysOutAsTheWholeTextWould(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	face := text.Default()
	st := text.Style{Size: 14}
	pieces := []string{"word ", "two words ", "\n", "\r\n", "\r", " ", "longer text that wraps ", "שלום ", ""}
	var rs []rune
	for range 120 {
		rs = append(rs, []rune(pieces[r.IntN(len(pieces))])...)
	}
	var a areaText
	width := float32(160)
	a.update(face, st, width, rs, 0, 0, 0, true)
	sameAsWhole(t, &a, rs, face, st, width, "at first")
	for i := range 200 {
		was := len(rs)
		at := r.IntN(len(rs) + 1)
		end := min(len(rs), at+r.IntN(8))
		rs = slices.Replace(rs, at, end, []rune(pieces[r.IntN(len(pieces))])...)
		if r.IntN(10) == 0 {
			width = float32(60 + r.IntN(300))
		}
		a.update(face, st, width, rs, at, was-end, was, true)
		sameAsWhole(t, &a, rs, face, st, width, "after edit "+string(rune('0'+i%10)))
		if r.IntN(10) == 0 {
			// The width alone changes.
			width = float32(60 + r.IntN(300))
			a.update(face, st, width, rs, 0, 0, 0, false)
			sameAsWhole(t, &a, rs, face, st, width, "after a new width")
		}
	}
}

func TestALongAreaDrawsTheLinesInViewEachFrame(t *testing.T) {
	wr := newWriter(t, 400)
	var b strings.Builder
	for i := range 3000 {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%40))
		b.WriteString("\n")
	}
	wr.area.SetText(b.String(), nil)
	wr.run(2)
	wr.key(input.KeyHome, input.ModControl)
	wr.run(30)
	// Select everything, then Page Down glides the text up a page at a
	// time: each frame draws the lines in view, a few of them, and the
	// selection only on those.
	wr.key(input.KeyA, input.ModControl)
	for range 5 {
		wr.key(input.KeyPageDown, input.ModShift)
		for f := range 20 {
			ops := wr.w.Offscreen().Ops()
			texts := textOps(ops)
			top, bottom := float32(1e9), float32(-1e9)
			for _, op := range texts {
				y := op.Transform.Apply(geom.Point{}).Y
				top, bottom = min(top, y), max(bottom, y)
			}
			if len(texts) > 40 || len(ops) > 150 || top > FieldPadding.Default()+20 || bottom < 400-40 {
				t.Fatalf("frame %d drew %d ops, %d of text, from y %v to %v; want the inside covered by a few",
					f, len(ops), len(texts), top, bottom)
			}
			wr.run(1)
		}
	}
}
