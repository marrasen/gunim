package text

import (
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/marrasen/gunim/geom"
)

const fox = "the quick brown fox jumps over the lazy dog"

func TestLayoutWrapsWithinTheWidth(t *testing.T) {
	p := Default().Layout(fox, Style{Size: 16}, 120)
	if len(p.Lines) < 3 {
		t.Fatalf("%d lines at 120 px, want the text to wrap", len(p.Lines))
	}
	glyphs := 0
	for i, l := range p.Lines {
		if l.Run.Advance > 120 {
			t.Fatalf("line %d is %v wide, past the 120 px width", i, l.Run.Advance)
		}
		glyphs += len(l.Run.Glyphs)
	}
	// Every letter survives the wrapping; spaces at line ends may go.
	if letters := len(strings.ReplaceAll(fox, " ", "")); glyphs < letters {
		t.Fatalf("%d glyphs for %d letters", glyphs, letters)
	}
	if p.Size.H <= 0 || p.Size.W > 120 {
		t.Fatalf("Size = %v", p.Size)
	}
}

func TestLayoutWithNoWidthKeepsEachLineWhole(t *testing.T) {
	p := Default().Layout(fox, Style{Size: 16}, 0)
	if len(p.Lines) != 1 {
		t.Fatalf("%d lines, want 1", len(p.Lines))
	}
}

func TestLayoutBreaksAtNewlinesAndKeepsEmptyLines(t *testing.T) {
	p := Default().Layout("one\n\nthree", Style{Size: 16}, 0)
	if len(p.Lines) != 3 {
		t.Fatalf("%d lines, want 3", len(p.Lines))
	}
	if len(p.Lines[1].Run.Glyphs) != 0 {
		t.Fatal("the empty line has glyphs")
	}
	step := p.Lines[1].At.Y - p.Lines[0].At.Y
	if step <= 0 || p.Lines[2].At.Y-p.Lines[1].At.Y != step {
		t.Fatalf("lines at y %v, %v, %v: want even steps", p.Lines[0].At.Y, p.Lines[1].At.Y, p.Lines[2].At.Y)
	}
	if want := 3 * step; p.Size.H < want-0.01 || p.Size.H > want+0.01 {
		t.Fatalf("height %v, want three lines, %v", p.Size.H, want)
	}
}

func TestLayoutEndsATruncatedLineWithAnEllipsis(t *testing.T) {
	f := Default()
	p := f.Layout(fox+" "+fox, Style{Size: 16, MaxLines: 2}, 120)
	if len(p.Lines) != 2 || !p.Truncated {
		t.Fatalf("%d lines, truncated %v; want 2 and true", len(p.Lines), p.Truncated)
	}
	ellipsis := f.Shape("…", 16).Glyphs[0].ID
	last := p.Lines[1].Run.Glyphs
	if last[len(last)-1].ID != ellipsis {
		t.Fatal("the last line does not end in an ellipsis")
	}
}

func TestLayoutTruncatesAcrossNewlines(t *testing.T) {
	p := Default().Layout("one\ntwo\nthree", Style{Size: 16, MaxLines: 2}, 0)
	if len(p.Lines) != 2 || !p.Truncated {
		t.Fatalf("%d lines, truncated %v; want 2 and true", len(p.Lines), p.Truncated)
	}
}

func TestLayoutAligns(t *testing.T) {
	f := Default()
	text := "a long first line\nshort"
	start := f.Layout(text, Style{Size: 16}, 0)
	center := f.Layout(text, Style{Size: 16, Align: AlignCenter}, 0)
	end := f.Layout(text, Style{Size: 16, Align: AlignEnd}, 0)
	short := start.Lines[1].Run.Advance
	if start.Lines[1].At.X != 0 {
		t.Fatalf("start-aligned short line at x %v, want 0", start.Lines[1].At.X)
	}
	if got, want := center.Lines[1].At.X, (start.Size.W-short)/2; got != want {
		t.Fatalf("centred short line at x %v, want %v", got, want)
	}
	if got, want := end.Lines[1].At.X, start.Size.W-short; got != want {
		t.Fatalf("end-aligned short line at x %v, want %v", got, want)
	}
}

// faceFile loads a system font for the right-to-left tests, which need
// glyphs Go Regular lacks.
func faceFile(t *testing.T, path string) *Face {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no %s on this machine", path)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

var (
	hebrewOnce sync.Once
	hebrew     *Face
)

// latinWithHebrew returns Go Regular falling back to Noto Sans Hebrew,
// parsed once.
// latinWithHebrew returns Go Regular falling back to a Hebrew face:
// Noto Sans Hebrew where Debian and Ubuntu put it, or else whatever
// installed font covers Hebrew, such as Arial on Windows.
func latinWithHebrew(t *testing.T) (latin, heb *Face) {
	t.Helper()
	hebrewOnce.Do(func() {
		const noto = "/usr/share/fonts/truetype/noto/NotoSansHebrew-Regular.ttf"
		if _, err := os.Stat(noto); err == nil {
			hebrew = faceFile(t, noto)
			return
		}
		if LoadSystemFonts() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if ff := systemFace('ש'); ff != nil {
			hebrew = byFont[ff]
		}
	})
	if hebrew == nil {
		t.Skip("no font on this machine covers Hebrew")
	}
	// A face of its own, so setting its fallback leaves Default alone.
	latin, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return latin.Fallback(hebrew), hebrew
}

// glyphOf returns the glyph face f uses for r.
func glyphOf(t *testing.T, f *Face, r rune) uint32 {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	gid, ok := f.face.NominalGlyph(r)
	if !ok {
		t.Fatalf("%q is missing from the face", r)
	}
	return uint32(gid)
}

func TestHebrewFallsBackAndSetsRightToLeft(t *testing.T) {
	latin, heb := latinWithHebrew(t)
	p := latin.Layout("שלום", Style{Size: 16}, 0)
	l := p.Lines[0]
	if !l.RightToLeft {
		t.Fatal("a Hebrew paragraph set left to right")
	}
	if len(l.Run.Glyphs) != 4 {
		t.Fatalf("%d glyphs, want 4", len(l.Run.Glyphs))
	}
	for _, g := range l.Run.Glyphs {
		if g.Face != heb.id {
			t.Fatal("a Hebrew letter came from the Latin face")
		}
	}
	// Visual order: the last letter, final mem, is drawn leftmost.
	if l.Run.Glyphs[0].ID != glyphOf(t, heb, 'ם') {
		t.Fatal("the leftmost glyph is not the word's last letter")
	}
}

func TestHebrewInsideLatinReadsInPlace(t *testing.T) {
	latin, heb := latinWithHebrew(t)
	l := latin.Layout("ab שלום cd", Style{Size: 16}, 0).Lines[0]
	if l.RightToLeft {
		t.Fatal("a paragraph starting in Latin set right to left")
	}
	// Left to right on screen: a, b, space, then the Hebrew word
	// reversed, space, c, d.
	g := l.Run.Glyphs
	if g[0].ID != glyphOf(t, latin, 'a') || g[len(g)-1].ID != glyphOf(t, latin, 'd') {
		t.Fatal("the Latin text lost its place at either end")
	}
	if g[3].ID != glyphOf(t, heb, 'ם') || g[6].ID != glyphOf(t, heb, 'ש') {
		t.Fatal("the Hebrew word is not reversed in place")
	}
	for i := 1; i < len(g); i++ {
		if g[i].At.X < g[i-1].At.X {
			t.Fatalf("glyph %d sits left of glyph %d", i, i-1)
		}
	}
}

func TestRightToLeftParagraphStartsOnTheRight(t *testing.T) {
	latin, _ := latinWithHebrew(t)
	p := latin.Layout("שלום עולם\nשלום", Style{Size: 16}, 0)
	if p.Lines[1].At.X <= 0 {
		t.Fatal("the short right-to-left line is not against the right edge")
	}
	if got, want := p.Lines[1].At.X+p.Lines[1].Run.Advance, p.Size.W; got < want-0.01 || got > want+0.01 {
		t.Fatalf("short line ends at %v, want the right edge %v", got, want)
	}
}

func TestCaretsRunAlongLatinText(t *testing.T) {
	r := Default().Shape("hello", 16)
	if r.Start != 0 || r.End != 5 {
		t.Fatalf("runes %d..%d, want 0..5", r.Start, r.End)
	}
	if r.CaretX(0) != 0 || r.CaretX(5) != r.Advance {
		t.Fatalf("carets at the ends are %v and %v, want 0 and %v", r.CaretX(0), r.CaretX(5), r.Advance)
	}
	for i := 1; i <= 5; i++ {
		if r.CaretX(i) <= r.CaretX(i-1) {
			t.Fatalf("caret %d at %v is not right of caret %d at %v", i, r.CaretX(i), i-1, r.CaretX(i-1))
		}
	}
	// A click lands on the nearest caret.
	for i := range 6 {
		if got := r.Index(r.CaretX(i) + 0.4); got != i {
			t.Fatalf("a click just right of caret %d went to %d", i, got)
		}
	}
	if r.Index(-50) != 0 || r.Index(1e6) != 5 {
		t.Fatal("a click past either end should land at that end")
	}
}

func TestCaretsRunRightToLeftInHebrew(t *testing.T) {
	latin, _ := latinWithHebrew(t)
	r := latin.Shape("שלום", 16)
	if r.CaretX(0) != r.Advance || r.CaretX(4) != 0 {
		t.Fatalf("carets at the ends are %v and %v, want %v on the right and 0 on the left", r.CaretX(0), r.CaretX(4), r.Advance)
	}
	for i := 1; i <= 4; i++ {
		if r.CaretX(i) >= r.CaretX(i-1) {
			t.Fatalf("caret %d is not left of caret %d", i, i-1)
		}
	}
}

func TestCaretsOfAWrappedLineCountWithinItsParagraph(t *testing.T) {
	p := Default().Layout("one two three four", Style{Size: 16}, 60)
	if len(p.Lines) < 2 {
		t.Fatal("setup: the text should wrap")
	}
	second := p.Lines[1].Run
	if second.Start != p.Lines[0].Run.End {
		t.Fatalf("the second line starts at rune %d, want where the first ends, %d", second.Start, p.Lines[0].Run.End)
	}
	if second.CaretX(second.Start) != 0 {
		t.Fatalf("the second line's first caret is at %v, want 0", second.CaretX(second.Start))
	}
}

func TestParagraphCaretsCountTheWholeString(t *testing.T) {
	p := Default().Layout("ab\n\ncd", Style{Size: 16}, 0)
	// Runes: a0 b1 \n2 \n3 c4 d5; the empty middle line holds rune 3.
	for _, tc := range []struct {
		i, line int
	}{{0, 0}, {2, 0}, {3, 1}, {4, 2}, {6, 2}} {
		line, at := p.Caret(tc.i)
		if line != tc.line {
			t.Errorf("caret %d on line %d, want %d", tc.i, line, tc.line)
		}
		if at.Y != p.Lines[tc.line].At.Y {
			t.Errorf("caret %d at y %v, want its line's top %v", tc.i, at.Y, p.Lines[tc.line].At.Y)
		}
	}
	if _, at := p.Caret(6); at.X != p.Lines[2].Run.Advance {
		t.Errorf("the last caret is at x %v, want the end of cd, %v", at.X, p.Lines[2].Run.Advance)
	}
}

func TestAWrappedLinesEndCaretGoesToTheNextLine(t *testing.T) {
	p := Default().Layout("one two three four", Style{Size: 16}, 60)
	if len(p.Lines) < 2 {
		t.Fatal("setup: the text should wrap")
	}
	if line, _ := p.Caret(p.Lines[1].Run.Start); line != 1 {
		t.Fatalf("the caret at the wrap is on line %d, want the later line", line)
	}
}

func TestParagraphIndexFindsTheLineAndRune(t *testing.T) {
	p := Default().Layout("ab\ncd", Style{Size: 16}, 0)
	if got := p.Index(geom.Pt(0, 1)); got != 0 {
		t.Fatalf("a click at the top left went to %d, want 0", got)
	}
	if got := p.Index(geom.Pt(1000, p.LineHeight*1.5)); got != 5 {
		t.Fatalf("a click past the end of the second line went to %d, want 5", got)
	}
	if got := p.Index(geom.Pt(0, 1e6)); got != 3 {
		t.Fatalf("a click below the text went to %d, want the last line's start, 3", got)
	}
}

func TestBesideStepsThroughLatin(t *testing.T) {
	r := Default().Shape("abc", 16)
	if i, _ := r.Beside(1, r.CaretX(1), true); i != 2 {
		t.Fatalf("right from 1 went to %d, want 2", i)
	}
	if i, _ := r.Beside(1, r.CaretX(1), false); i != 0 {
		t.Fatalf("left from 1 went to %d, want 0", i)
	}
	if i, x := r.Beside(3, r.CaretX(3), true); i != 3 || x != r.CaretX(3) {
		t.Fatal("Beside should stop at the right end")
	}
}

func TestBesideFollowsTheScreenThroughHebrew(t *testing.T) {
	latin, _ := latinWithHebrew(t)
	r := latin.Shape("a שלום b", 16)
	// From the far left, moving right reaches every caret place in
	// order across the screen, the Hebrew word's included, and ends at
	// the far right.
	i, x := 0, r.CaretX(0)
	steps := 0
	for {
		ni, nx := r.Beside(i, x, true)
		if ni == i && nx == x {
			break
		}
		if nx <= x {
			t.Fatalf("moving right, the caret went from x %v to %v", x, nx)
		}
		if !r.Places(ni, nx) {
			t.Fatalf("Beside put the caret at rune %d, x %v, where it cannot sit", ni, nx)
		}
		i, x, steps = ni, nx, steps+1
	}
	// Eight runes leave nine places; a caret stops at eight of them
	// after the first.
	if steps != 8 || x != r.Advance {
		t.Fatalf("%d steps ending at x %v, want 8 ending at %v", steps, x, r.Advance)
	}
	// And back again.
	for range 8 {
		i, x = r.Beside(i, x, false)
	}
	if x != 0 || i != 0 {
		t.Fatalf("eight steps left ended at rune %d, x %v, want 0, 0", i, x)
	}
}
