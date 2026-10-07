package widget

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
)

// sameAsFresh fails t unless c's lines are as a new editor lays out
// its code.
func sameAsFresh(t *testing.T, c *CodeEditor, what string) {
	t.Helper()
	f := NewCodeEditor()
	f.Highlight = c.Highlight
	f.SetText(c.Text(), nil)
	f.preedit, f.caret, f.anchor = c.preedit, c.caret, c.anchor
	f.relayout(nil)
	// Lay out the lines whose colours changed, as showing them does.
	widest := c.widest
	for i := range c.lines {
		c.laidLine(i)
	}
	if c.widest != widest {
		t.Fatalf("%s: laying out lines whose colours changed made the widest %v, want it %v as it was", what, c.widest, widest)
	}
	if len(c.lines) != len(f.lines) {
		t.Fatalf("%s: %d lines, want %d", what, len(c.lines), len(f.lines))
	}
	for i, want := range f.lines {
		got := c.lines[i]
		if got.start != want.start || got.end != want.end || got.text != want.text || got.cont != want.cont ||
			!slices.Equal(got.toks, want.toks) || !slices.Equal(got.spans, want.spans) || got.width != want.width {
			t.Fatalf("%s: line %d is %q from %d to %d, tokens %v, want %q from %d to %d, tokens %v",
				what, i, got.text, got.start, got.end, got.toks, want.text, want.start, want.end, want.toks)
		}
	}
	if c.widest != f.widest || c.gutterW != f.gutterW {
		t.Fatalf("%s: widest %v and gutter %v, want %v and %v", what, c.widest, c.gutterW, f.widest, f.gutterW)
	}
}

func TestCodeLaysOutAnEditAsTheWholeCodeAfresh(t *testing.T) {
	for seed := range uint64(8) {
		layOutEdits(t, rand.New(rand.NewPCG(seed, 12)))
	}
}

// layOutEdits makes random edits in random code, checking the layout
// after each.
func layOutEdits(t *testing.T, r *rand.Rand) {
	t.Helper()
	bits := []string{
		"\n", "\n\n", "func f() {\n", "}\n", "\tx := 1\n", "/*", "*/", "`", "\"", "// note\n",
		"type\n", "T int\n", "f\n", "(", ")", "\tv := fmt.Sprintf(\"%d\", 1)\n", "é😀", "\t", " ",
	}
	src := make([]byte, 0, 4096)
	for range 200 {
		src = append(src, bits[r.IntN(len(bits))]...)
	}
	c := NewCodeEditor()
	c.SetText(string(src), nil)
	c.relayout(nil)
	sameAsFresh(t, c, "at first")
	for i := range 100 {
		n := len(c.text)
		a := r.IntN(n + 1)
		b := min(n, a+r.IntN(6))
		c.last = otherEdit
		c.replace(a, b, []rune(bits[r.IntN(len(bits))]), nil)
		if r.IntN(8) == 0 {
			// A composition shows in place too.
			c.preedit = []rune("/*")
		} else {
			c.preedit = nil
		}
		c.relayout(nil)
		sameAsFresh(t, c, fmt.Sprintf("after edit %d", i+1))
	}
}

func TestAnOpenedCommentColoursTheCodeAfterIt(t *testing.T) {
	c := NewCodeEditor()
	c.SetText(bigCode(2000), nil)
	c.relayout(nil)
	kinds := func(l codeLine) []string {
		out := make([]string, 0, len(l.toks))
		for _, tk := range l.toks {
			out = append(out, tk.Kind.String())
		}
		return out
	}
	before := kinds(c.lines[1500])
	at := c.lines[10].start
	c.replace(at, at, []rune("/*"), nil)
	c.relayout(nil)
	if got := kinds(c.lines[1500]); !slices.Equal(got, []string{"comment"}) {
		t.Fatalf("after a comment opened, line 1500 has tokens %v, want one comment", got)
	}
	c.last = otherEdit
	c.replace(at, at+2, nil, nil)
	c.relayout(nil)
	if got := kinds(c.lines[1500]); !slices.Equal(got, before) {
		t.Fatalf("after the comment went, line 1500 has tokens %v, want %v", got, before)
	}
}

func TestTheShapeCacheLetsGoOfTheOldest(t *testing.T) {
	var sc shapeCache
	face := faceIn(MonoFont, nil)
	for i := range maxCodeShapes + 10 {
		sc.shape(face, strconv.Itoa(i), 13)
		// Keep 0 in use.
		sc.shape(face, "0", 13)
	}
	if len(sc.runs) != maxCodeShapes {
		t.Fatalf("the cache holds %d shapes, want %d", len(sc.runs), maxCodeShapes)
	}
	n := 0
	for e := sc.newest; e != nil; e = e.older {
		n++
	}
	if n != maxCodeShapes {
		t.Fatalf("the order of use holds %d shapes, want %d", n, maxCodeShapes)
	}
	if _, ok := sc.runs["0"]; !ok {
		t.Fatal("the cache let go of a shape in use")
	}
	if _, ok := sc.runs["1"]; ok {
		t.Fatal("the cache kept the shape used longest ago")
	}
}
