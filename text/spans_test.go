package text

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
)

func runesIn(p SpanParagraph) int {
	n := 0
	for _, l := range p.Lines {
		for _, pc := range l.Pieces {
			n += pc.Run.End - pc.Run.Start
		}
	}
	return n
}

func TestSpansSitSideBySide(t *testing.T) {
	regular, bold := GoSans(false, false), GoSans(true, false)
	p := LayoutSpans([]Span{{"hello ", regular, 14, 0}, {"world", bold, 14, 0}}, Style{}, 0)
	if len(p.Lines) != 1 || len(p.Lines[0].Pieces) != 2 {
		t.Fatalf("got %d lines, the first of %d pieces; want one of two", len(p.Lines), len(p.Lines[0].Pieces))
	}
	want := regular.Shape("hello ", 14).Advance
	if x := p.Lines[0].Pieces[1].At.X; x != want {
		t.Fatalf("the bold word starts at %v, want %v, after the regular one", x, want)
	}
}

func TestSpansWrapToTheWidth(t *testing.T) {
	regular, bold := GoSans(false, false), GoSans(true, false)
	text := []string{"The quick brown fox ", "jumps over", " the lazy dog, and then some more words to wrap."}
	spans := []Span{{text[0], regular, 14, 0}, {text[1], bold, 14, 0}, {text[2], regular, 14, 0}}
	p := LayoutSpans(spans, Style{}, 120)
	if len(p.Lines) < 3 {
		t.Fatalf("at 120 wide the text took %d lines, want several", len(p.Lines))
	}
	for i, l := range p.Lines {
		if l.Width > 120 {
			t.Fatalf("line %d is %v wide, past 120", i, l.Width)
		}
	}
	if got, want := runesIn(p), len([]rune(strings.Join(text, ""))); got != want {
		t.Fatalf("the lines hold %d runes, want all %d", got, want)
	}
}

func TestANewlineStartsALine(t *testing.T) {
	p := LayoutSpans([]Span{{"one\ntwo", GoSans(false, false), 14, 0}}, Style{}, 0)
	if len(p.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(p.Lines))
	}
	if p.Lines[1].Top <= p.Lines[0].Top {
		t.Fatalf("the second line is at %v, not below the first at %v", p.Lines[1].Top, p.Lines[0].Top)
	}
}

func TestAWordWiderThanTheLineIsSplit(t *testing.T) {
	word := strings.Repeat("m", 30)
	p := LayoutSpans([]Span{{word, GoSans(false, false), 14, 0}}, Style{}, 60)
	if len(p.Lines) < 3 {
		t.Fatalf("a word of 30 m at 60 wide took %d lines", len(p.Lines))
	}
	for i, l := range p.Lines {
		if l.Width > 60 {
			t.Fatalf("line %d is %v wide, past 60", i, l.Width)
		}
	}
	if got := runesIn(p); got != 30 {
		t.Fatalf("the lines hold %d runes, want 30", got)
	}
}

func TestSpansOfTwoSizesShareABaseline(t *testing.T) {
	f := GoSans(false, false)
	p := LayoutSpans([]Span{{"Big ", f, 28, 0}, {"small", f, 12, 0}}, Style{}, 0)
	a, b := p.Lines[0].Pieces[0], p.Lines[0].Pieces[1]
	if d := (a.At.Y + a.Run.Ascent) - (b.At.Y + b.Run.Ascent); d < -0.01 || d > 0.01 {
		t.Fatalf("baselines at %v and %v", a.At.Y+a.Run.Ascent, b.At.Y+b.Run.Ascent)
	}
}

// boxPiece returns the index of span's piece on line l, or -1.
func boxPiece(l SpanLine, span int) int {
	for i, pc := range l.Pieces {
		if pc.Span == span {
			return i
		}
	}
	return -1
}

func TestABoxTakesItsWidthAndWrapsAsAWord(t *testing.T) {
	f := GoSans(false, false)
	spans := []Span{{Text: "one two ", Face: f, Size: 14}, {Face: f, Size: 14, Box: 20}, {Text: " three", Face: f, Size: 14}}
	p := LayoutSpans(spans, Style{}, 0)
	if len(p.Lines) != 1 {
		t.Fatalf("got %d lines, want one", len(p.Lines))
	}
	pcs := p.Lines[0].Pieces
	box := boxPiece(p.Lines[0], 1)
	if box < 1 || pcs[box].Run.Advance != 20 || pcs[box].At.X != f.Shape("one two ", 14).Advance {
		t.Fatalf("the box is piece %d of %+v, want 20 wide after the text", box, pcs)
	}
	if next := pcs[box+1]; next.At.X != pcs[box].At.X+20 {
		t.Fatalf("the text after the box starts at %v, want %v", next.At.X, pcs[box].At.X+20)
	}
	if h := pcs[box].Run.Height(); h != pcs[0].Run.Height() {
		t.Fatalf("the box is %v tall, want as tall as its text, %v", h, pcs[0].Run.Height())
	}
	// Just too narrow for the box: it starts the next line.
	width := pcs[box].At.X + 10
	p = LayoutSpans(spans, Style{}, width)
	if len(p.Lines) < 2 || boxPiece(p.Lines[1], 1) != 0 || p.Lines[1].Pieces[0].At.X != 0 {
		t.Fatalf("at %v wide the box did not start the second line: %+v", width, p.Lines)
	}
}

func TestABoxBesideAWordWrapsWithIt(t *testing.T) {
	f := GoSans(false, false)
	spans := []Span{{Text: "alpha beta ", Face: f, Size: 14}, {Face: f, Size: 14, Box: 20}, {Text: "gamma", Face: f, Size: 14}}
	width := f.Shape("alpha beta ", 14).Advance + 25
	p := LayoutSpans(spans, Style{}, width)
	if len(p.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(p.Lines))
	}
	if l := p.Lines[1].Pieces; len(l) != 2 || l[0].Span != 1 || l[1].Span != 2 {
		t.Fatalf("the second line holds %+v, want the box and the word it touches", l)
	}
}

// A long word after a box still wraps in a narrow column, the box and a letter first.
func TestALongWordAfterABoxWrapsInANarrowColumn(t *testing.T) {
	f := GoSans(false, false)
	word := "averyveryverylongfilename.pdf"
	spans := []Span{{Face: f, Size: 14, Box: 22}, {Text: word, Face: f, Size: 14}}
	p := LayoutSpans(spans, Style{}, 22)
	if len(p.Lines) < 5 {
		t.Fatalf("at 22 wide the word took %d lines, want it cut up", len(p.Lines))
	}
	if l := p.Lines[0].Pieces; len(l) != 2 || l[0].Span != 0 || len(l[1].Run.Glyphs) != 1 {
		t.Fatalf("the first line holds %+v, want the box and one letter", l)
	}
}

// A box after a word that fills the line starts the next one.
func TestABoxAfterAWordThatFillsTheLineStartsTheNext(t *testing.T) {
	f := GoSans(false, false)
	spans := []Span{{Text: "gamma", Face: f, Size: 14}, {Face: f, Size: 14, Box: 20}}
	width := f.Shape("gamma", 14).Advance
	p := LayoutSpans(spans, Style{}, width)
	if len(p.Lines) != 2 || boxPiece(p.Lines[1], 1) != 0 || p.Lines[1].Pieces[0].At.X != 0 {
		t.Fatalf("at %v wide the lines are %+v, want the box to start the second", width, p.Lines)
	}
}

func TestPiecesKnowWhereTheyStart(t *testing.T) {
	regular, bold := GoSans(false, false), GoSans(true, false)
	spans := []Span{{"one two three ", regular, 14, 0}, {"four five", bold, 14, 0}, {"\nsix", regular, 14, 0}}
	p := LayoutSpans(spans, Style{}, 60)
	all := []rune("one two three four five\nsix")
	for _, l := range p.Lines {
		for _, pc := range l.Pieces {
			text := strings.TrimSpace(string(all[pc.Start : pc.Start+pc.Run.End]))
			if want := strings.TrimSpace(spans[pc.Span].Text); !strings.Contains(want, text) {
				t.Fatalf("piece at %d reads %q, not part of its span %q", pc.Start, text, want)
			}
		}
	}
}

func TestIndexFindsTheRuneUnderAPoint(t *testing.T) {
	regular, bold := GoSans(false, false), GoSans(true, false)
	p := LayoutSpans([]Span{{"hello ", regular, 14, 0}, {"world", bold, 14, 0}, {"\nagain", regular, 14, 0}}, Style{}, 0)
	for _, l := range p.Lines {
		for _, pc := range l.Pieces {
			for i := pc.Run.Start; i < pc.Run.End; i++ {
				// Just right of the caret before rune i.
				x := pc.At.X + pc.Run.CaretX(i) + 0.5
				if got := p.Index(geom.Pt(x, l.Top+l.Height/2)); got != pc.Start+i {
					t.Fatalf("Index at rune %d's caret = %d", pc.Start+i, got)
				}
			}
		}
	}
	if got := p.Index(geom.Pt(-5, -5)); got != 0 {
		t.Fatalf("Index above and left = %d, want 0", got)
	}
	if got := p.Index(geom.Pt(1000, 1000)); got != 17 {
		t.Fatalf("Index below and right = %d, want the end, 17", got)
	}
}

func TestSelectCoversEachLineOnce(t *testing.T) {
	p := LayoutSpans([]Span{{"one\ntwo\nthree", GoSans(false, false), 14, 0}}, Style{}, 0)
	var boxes []geom.Rect
	p.Select(1, 9, func(r geom.Rect) { boxes = append(boxes, r) })
	if len(boxes) != 3 {
		t.Fatalf("%d boxes for a selection over three lines, want 3", len(boxes))
	}
	if boxes[0].Min.Y != p.Lines[0].Top || boxes[2].Min.Y != p.Lines[2].Top {
		t.Fatalf("boxes %v are not on the lines", boxes)
	}
}
