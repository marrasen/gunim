package text

import (
	"strings"
	"testing"
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
	p := LayoutSpans([]Span{{"hello ", regular, 14}, {"world", bold, 14}}, Style{}, 0)
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
	spans := []Span{{text[0], regular, 14}, {text[1], bold, 14}, {text[2], regular, 14}}
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
	p := LayoutSpans([]Span{{"one\ntwo", GoSans(false, false), 14}}, Style{}, 0)
	if len(p.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(p.Lines))
	}
	if p.Lines[1].Top <= p.Lines[0].Top {
		t.Fatalf("the second line is at %v, not below the first at %v", p.Lines[1].Top, p.Lines[0].Top)
	}
}

func TestAWordWiderThanTheLineIsSplit(t *testing.T) {
	word := strings.Repeat("m", 30)
	p := LayoutSpans([]Span{{word, GoSans(false, false), 14}}, Style{}, 60)
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
	p := LayoutSpans([]Span{{"Big ", f, 28}, {"small", f, 12}}, Style{}, 0)
	a, b := p.Lines[0].Pieces[0], p.Lines[0].Pieces[1]
	if d := (a.At.Y + a.Run.Ascent) - (b.At.Y + b.Run.Ascent); d < -0.01 || d > 0.01 {
		t.Fatalf("baselines at %v and %v", a.At.Y+a.Run.Ascent, b.At.Y+b.Run.Ascent)
	}
}
