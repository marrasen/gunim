package widget

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// breaks returns where each of l's pieces starts and ends.
func breaks(l *longLine) [][2]int {
	out := make([][2]int, 0, len(l.pieces))
	for _, p := range l.pieces {
		out = append(out, [2]int{p.start, p.end})
	}
	return out
}

func TestALongLineBreaksAsIfShapedAfresh(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	face := text.Default()
	words := []string{"a", "word", "of", "text", "Völlig", "é", "  ", "x"}
	var rs []rune
	for len(rs) < 4000 {
		rs = append(rs, []rune(words[r.IntN(len(words))]+" ")...)
	}
	var l longLine
	l.update(face, 14, rs, 0, 0, 0, true)
	for i := range 300 {
		was := len(rs)
		a := r.IntN(len(rs) + 1)
		b := min(len(rs), a+r.IntN(20))
		with := []rune(words[r.IntN(len(words))])
		if r.IntN(3) == 0 {
			with = nil
		}
		rs = slices.Replace(rs, a, b, with...)
		l.update(face, 14, rs, a, was-b, was, true)
		var fresh longLine
		fresh.update(face, 14, rs, 0, 0, 0, true)
		if !slices.Equal(breaks(&l), breaks(&fresh)) {
			t.Fatalf("after %d edits the pieces are %v, want %v", i+1, breaks(&l), breaks(&fresh))
		}
		if l.Advance != fresh.Advance {
			t.Fatalf("after %d edits the line is %v wide, want %v", i+1, l.Advance, fresh.Advance)
		}
	}
	if len(l.pieces) < 20 {
		t.Fatalf("%d pieces in %d runes, want the line in many pieces", len(l.pieces), len(rs))
	}
}

func TestALongLineAnswersAsTheWholeRunDoes(t *testing.T) {
	face := text.Default()
	for _, s := range []string{
		bigLine(3000),
		"plain words, then שלום עולם and more words after it, for a while longer " + bigLine(2000),
	} {
		rs := []rune(s)
		var l longLine
		l.update(face, 14, rs, 0, 0, 0, true)
		whole := face.Shape(s, 14)
		if rtl := slices.ContainsFunc(rs, rightToLeft); rtl != (len(l.pieces) == 1) {
			t.Fatalf("%d pieces for a line that runs right to left: %v", len(l.pieces), rtl)
		}
		if d := l.Advance - whole.Advance; d > 0.01 || d < -0.01 {
			t.Fatalf("the line is %v wide, want %v", l.Advance, whole.Advance)
		}
		for i := range rs {
			if d := l.CaretX(i) - whole.CaretX(i); d > 0.01 || d < -0.01 {
				t.Fatalf("a caret before rune %d is at %v, want %v", i, l.CaretX(i), whole.CaretX(i))
			}
		}
		for x := float32(-5); x < whole.Advance+5; x += 3.7 {
			if got, want := l.Index(x), whole.Index(x); got != want {
				t.Fatalf("x %v is before rune %d, want %d", x, got, want)
			}
		}
		// Walk the caret across the line both ways.
		for _, right := range []bool{true, false} {
			i, x := 0, whole.CaretX(0)
			if !right {
				i, x = len(rs), whole.CaretX(len(rs))
			}
			for range len(rs) + 1 {
				gi, gx := l.Beside(i, x, right)
				wi, wx := whole.Beside(i, x, right)
				if gi != wi || gx-wx > 0.01 || wx-gx > 0.01 {
					t.Fatalf("beside rune %d at %v is %d at %v, want %d at %v", i, x, gi, gx, wi, wx)
				}
				if !l.Places(gi, gx) {
					t.Fatalf("rune %d has no place at %v", gi, gx)
				}
				i, x = gi, gx
			}
		}
	}
}

// textOps returns the text ops w drew last.
func textOps(ops []paint.Op) []*paint.TextOp {
	var out []*paint.TextOp
	for _, op := range ops {
		if t, ok := op.(*paint.TextOp); ok {
			out = append(out, t)
		}
	}
	return out
}

func TestALongFieldDrawsTheTextInViewEachFrame(t *testing.T) {
	ty := newTyper(t)
	ty.field.SetText(bigLine(200000))
	ty.run(2)
	ty.key(input.KeyHome, 0)
	ty.run(30)
	// End glides the text across: each frame, the glyphs drawn cover the
	// field's inside, and few are drawn.
	ty.key(input.KeyEnd, 0)
	for f := range 40 {
		ops := textOps(ty.w.Offscreen().Ops())
		lo, hi, n := float32(1e9), float32(-1e9), 0
		for _, op := range ops {
			for _, g := range op.Glyphs {
				x := op.Transform.Apply(g.At).X
				lo, hi = min(lo, x), max(hi, x)
				n++
			}
		}
		pad := FieldPadding.Default()
		if lo > pad || hi < 300-pad-10 || n > 400 {
			t.Fatalf("frame %d drew %d glyphs from x %v to %v, want the field's inside covered by a few", f, n, lo, hi)
		}
		ty.run(1)
	}
	if ty.field.caret != len(ty.field.text) {
		t.Fatalf("the caret is at %d, want the end, %d", ty.field.caret, len(ty.field.text))
	}
}
