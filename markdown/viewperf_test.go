package markdown

import (
	"fmt"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// longDoc returns a document of n sections, each a heading, a paragraph with a link, a list and a code block, and
// after them a code block of lines lines.
func longDoc(n, lines int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "## Section %d\n\nSome words about section %d, with [a link](https://example.com/%d) in them, and more words after.\n\n", i, i, i)
		fmt.Fprintf(&b, "- the first point\n- the second point\n\n1. a step\n2. another\n\n```\ncode line one\ncode line two\n```\n\n")
	}
	if lines > 0 {
		b.WriteString("```\n")
		for i := range lines {
			fmt.Fprintf(&b, "line %d of a long listing\n", i)
		}
		b.WriteString("```\n")
	}
	return b.String()
}

// paintAt paints v, laid out, scrolled down to y, as a scroll view 400 by 600 shows it, and returns the ops.
func paintAt(p *paint.Painter, v *View, y float32) []paint.Op {
	p.Reset()
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, 0, 400, 600), Opacity: 1, Clip: true})()
		defer p.Push(paint.Translate(geom.Pt(0, -y)))()
		v.Paint(p, gunim.Frame{Scale: 1}, v.size, gunim.Children{})
	}()
	return p.Ops()
}

// layLong lays a long document out 400 wide.
func layLong(sections, lines int) *View {
	v := New(longDoc(sections, lines))
	v.Layout(gunim.Loose(geom.Sz(400, 1e9)), gunim.Frame{Scale: 1}, gunim.Children{})
	return v
}

// BenchmarkALongMarkdownFrame paints a screenful of a document of 2 000 sections and a listing of 5 000 lines, a
// third and two thirds of the way down in turn, as each frame of a scroll does.
func BenchmarkALongMarkdownFrame(b *testing.B) {
	v := layLong(2000, 5000)
	var p paint.Painter
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		paintAt(&p, v, v.size.H*float32(i%2+1)/3)
	}
}

// BenchmarkALongMarkdownHover finds the link under the pointer half way down a document of 2 000 sections.
func BenchmarkALongMarkdownHover(b *testing.B) {
	v := layLong(2000, 0)
	pt := geom.Pt(200, v.size.H/2)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		v.linkAt(pt)
	}
}

// A long document draws only what shows, and draws all of that: the same ops as painting everything, less what
// lies outside the screenful.
func TestALongDocumentDrawsAScreenfulAndAllOfIt(t *testing.T) {
	v := layLong(200, 300)
	var p, all paint.Painter
	for _, y := range []float32{0, v.size.H / 3, v.size.H - 600, v.size.H - 3000} {
		// Everything drawn, with no clip to say what shows, and the runs of text of it whose baselines are on the screen
		all.Reset()
		func() {
			defer all.Push(paint.Translate(geom.Pt(0, -y)))()
			v.Paint(&all, gunim.Frame{Scale: 1}, v.size, gunim.Children{})
		}()
		type place struct{ x, y float32 }
		want := map[place]bool{}
		total := 0
		for _, op := range all.Ops() {
			if o, ok := op.(*paint.TextOp); ok {
				total++
				if o.Transform.F >= 0 && o.Transform.F <= 600 {
					want[place{o.Transform.C, o.Transform.F}] = true
				}
			}
		}
		got := 0
		for _, op := range paintAt(&p, v, y) {
			if o, ok := op.(*paint.TextOp); ok {
				got++
				delete(want, place{o.Transform.C, o.Transform.F})
				if o.Transform.F < -100 || o.Transform.F > 700 {
					t.Fatalf("scrolled to %v, the view draws text at y=%v, far off the screen", y, o.Transform.F)
				}
			}
		}
		if len(want) > 0 {
			t.Fatalf("scrolled to %v, %d runs of text that reach the screen are not drawn", y, len(want))
		}
		if got*5 > total {
			t.Fatalf("scrolled to %v, the view draws %d of its %d runs of text", y, got, total)
		}
		// Every link in view is found under the pointer.
		for i, lp := range v.paras {
			for _, l := range lp.p.Lines {
				for _, pc := range l.Pieces {
					if lp.spans[pc.Span].url == "" {
						continue
					}
					at := pc.At.Add(lp.at).Add(geom.Pt(pc.Run.Advance/2, pc.Run.Height()/2))
					if at.Y < y || at.Y > y+600 {
						continue
					}
					if got := v.linkAt(at); got != [2]int{i, pc.Span} {
						t.Fatalf("the link of paragraph %d at %v is found as %v", i, at, got)
					}
				}
			}
		}
	}
}
