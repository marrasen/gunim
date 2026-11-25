//go:build js && wasm

package text

import (
	"bytes"
	"strings"
	"syscall/js"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/marrasen/gunim/paint"
)

// These run in a real browser, by tools/webtest, which runs the tests named for it.

// count counts the calls of a method of every 2D canvas, until the test ends.
func count(t *testing.T, method string) func() int {
	t.Helper()
	proto := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	was := proto.Get(method)
	counter := js.Global().Call("eval", `(was => {
		const f = function (...args) { f.calls++; return was.apply(this, args); };
		f.calls = 0;
		return f;
	})`).Invoke(was)
	proto.Set(method, counter)
	t.Cleanup(func() { proto.Set(method, was) })
	return func() int { return counter.Get("calls").Int() }
}

// browserFontFor returns a table of the browser's of its own for a test, with room for few glyphs.
func browserFontFor(t *testing.T, cells bool, room int) *browserFont {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if !browserPad.ready() {
		t.Fatal("the browser gives no canvas")
	}
	b := newBrowserFont("", "px sans-serif", cells)
	b.room = room
	return b
}

// A table of glyphs stays in bounds whatever it is asked for, a glyph it dropped draws nothing, and never another's
// picture, and one asked for or kept stays as it was.
func TestBrowserTableStaysInBounds(t *testing.T) {
	for _, cells := range []bool{false, true} {
		const room = 50
		b := browserFontFor(t, cells, room)
		key := func(i int) browserKey {
			k := browserKey{s: string([]rune{'a' + rune(i%26), 0x300 + rune(i/26%0x40), 0x300 + rune(i/26/0x40)})}
			if cells {
				k.w, k.up, k.down = 64, 64, 16
			}
			return k
		}
		mu.Lock()
		text, cell, turned := generation.Load(), cellGeneration.Load(), turns.Load()
		asked, shown, lost := b.inTable(key(0)).id, b.inTable(key(1)).id, b.inTable(key(2)).id
		ids := map[uint32]bool{}
		for i := 3; i < 1000; i++ {
			id := b.inTable(key(i)).id
			if ids[id] || id == asked || id == shown || id == lost {
				t.Fatalf("glyph %d has a number used before", i)
			}
			ids[id] = true
			if got := b.inTable(key(0)).id; got != asked {
				t.Fatalf("after %d glyphs one asked for all along is another", i)
			}
			b.keepGlyph(shown)
			// The two asked for and kept, and no more, may come on top.
			if n := len(b.now) + len(b.old); n > 2*room+2 || len(b.byID) != n {
				t.Fatalf("after %d glyphs the table holds %d, and %d by number, with room for %d", i, n, len(b.byID), room)
			}
		}
		if _, ok := b.glyph(shown); !ok {
			t.Error("a glyph kept is gone")
		}
		if _, ok := b.glyph(lost); ok {
			t.Error("a glyph not asked for since is still there")
		}
		if cells != (cellGeneration.Load() > cell) || cells == (generation.Load() > text) {
			t.Errorf("cells: %v: the text's generation went from %d to %d, and the cells' from %d to %d", cells, text, generation.Load(), cell, cellGeneration.Load())
		}
		if turns.Load() == turned {
			t.Error("no turn was counted")
		}
		mu.Unlock()
		if m := b.rasterize(lost, 20, 0, false); m.W != 0 {
			t.Errorf("a glyph dropped draws %dx%d pixels", m.W, m.H)
		}
	}
}

// Holding on to glyphs drops none, however many there are, and nor does drawing them: those not yet kept, or drawn, of
// what shows are still to come.
func TestBrowserKeepingGlyphsDropsNone(t *testing.T) {
	const room = 20
	b := browserFontFor(t, true, room)
	mu.Lock()
	defer mu.Unlock()
	var ids []uint32
	add := func(r rune) {
		ids = append(ids, b.inTable(browserKey{s: string([]rune{r, 0x302}), w: 64, up: 64, down: 16}).id)
	}
	for i := range rune(room) {
		add('A' + i)
	}
	// All but one of a second turn.
	for i := range rune(room - 1) {
		add('a' + i)
	}
	for _, keep := range []func(id uint32){b.keepGlyph, func(id uint32) { b.isColor(id) }} {
		was := turns.Load()
		for _, id := range ids {
			keep(id)
		}
		if turns.Load() != was {
			t.Fatal("holding on to glyphs turned the table")
		}
		for _, id := range ids {
			if _, ok := b.glyph(id); !ok {
				t.Fatal("holding on to glyphs dropped one")
			}
		}
		// The next turn drops none that was kept.
		add('0')
		add('1')
	}
}

// Text shaped before its glyphs were dropped is shaped again, with glyphs that draw.
func TestBrowserTextIsShapedAgainAfterATurn(t *testing.T) {
	f := GoMono(true, true)
	mu.Lock()
	b := browserFaceOf(f, false).browser
	b.room = 20
	mu.Unlock()
	defer func() { b.room = browserRoom }()
	const word = "x\u0302"
	before := f.Shape(word, 20).Glyphs[0]
	for i := range rune(100) {
		f.Shape(string([]rune{'a' + i%26, 0x300 + i/26, 0x323}), 20)
	}
	after := f.Shape(word, 20).Glyphs[0]
	face, _ := Lookup(after.Face)
	if after.ID == before.ID || face.Rasterize(after.ID, 20, 0, Raster{}).W == 0 {
		t.Errorf("the glyph was %d, and is %d after it was dropped", before.ID, after.ID)
	}
	// What is shown is kept through a turn.
	Keep([]paint.Op{&paint.TextOp{Glyphs: []paint.Glyph{after}}})
	for i := range rune(15) {
		f.Shape(string([]rune{'A' + i%26, 0x300 + i/26, 0x323}), 20)
	}
	Keep([]paint.Op{&paint.TextOp{Glyphs: []paint.Glyph{after}}})
	for i := range rune(15) {
		f.Shape(string([]rune{'A' + i%26, 0x300 + i/26, 0x324}), 20)
	}
	if face.Rasterize(after.ID, 20, 0, Raster{}).W == 0 {
		t.Error("a glyph kept as shown is gone")
	}
}

// A page of Chinese is laid out with a measure or two, not one for each of its characters, and so is one of Korean or
// of emoji.
func TestBrowserMeasuresAlikeCharactersOnce(t *testing.T) {
	f := GoSans(true, false)
	f.Shape("中한\U0001f600", 20)
	calls := count(t, "measureText")
	for _, from := range []rune{0x4e00, 0xac00, 0x20000} {
		var s strings.Builder
		for i := range rune(2000) {
			s.WriteRune(from + i)
		}
		run := f.Shape(s.String(), 20)
		if len(run.Glyphs) != 2000 || run.Advance < 2000*10 {
			t.Errorf("2000 characters from %U are %d glyphs, %v wide", from, len(run.Glyphs), run.Advance)
		}
	}
	var s strings.Builder
	for r := rune(0x1f400); r < 0x1f43e; r++ {
		s.WriteRune(r)
	}
	f.Shape(s.String(), 20)
	if n := calls(); n > 4 {
		t.Errorf("6000 characters took %d measures", n)
	}
}

// A character wider than the one it is laid out like, as where the browser has a font for it and none for that one, is
// drawn within its room, and not over the next.
func TestBrowserDrawsACharacterInItsRoom(t *testing.T) {
	b := browserFontFor(t, false, 10)
	proto := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	was := proto.Get("measureText")
	proto.Set("measureText", js.Global().Call("eval", `(was => function (s) {
		return s == "中" ? {width: 20} : was.call(this, s);
	})`).Invoke(was))
	defer proto.Set("measureText", was)
	mu.Lock()
	id := b.id([]rune{'国'}, false)
	g, _ := b.glyph(id)
	mu.Unlock()
	if g.advance != 20.0/measureSize {
		t.Fatalf("it is %v wide, want as wide as the other, %v", g.advance, 20.0/measureSize)
	}
	m := b.rasterize(id, 64, 0, false)
	if m.W == 0 || m.Offset.X < -2 || m.Offset.X+m.W > 22 {
		t.Errorf("it is drawn from %d to %d, in room for 20", m.Offset.X, m.Offset.X+m.W)
	}
	if m := b.rasterize(b.id([]rune("中国"), false), 64, 0, false); m.W < 40 {
		t.Errorf("what is measured itself is drawn %d wide", m.W)
	}
}

// Text of one character at a time needs no table.
func TestBrowserCharactersTakeNoRoom(t *testing.T) {
	f := GoSans(false, true)
	mu.Lock()
	b := browserFaceOf(f, false).browser
	mu.Unlock()
	was := len(b.byID)
	f.Shape("中文한글⇒ไทย שלום", 20)
	if got := len(b.byID); got != was {
		t.Errorf("the table went from %d glyphs to %d", was, got)
	}
}

// A browser that gives back something else than was drawn, as one set to hide what it runs on does, or that gives back
// nothing, draws no glyphs.
func TestBrowserThatHidesItsPixelsDrawsNothing(t *testing.T) {
	proto := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	was := proto.Get("getImageData")
	defer proto.Set("getImageData", was)
	for name, fake := range map[string]string{
		"white":   `(function (x, y, w, h) { const d = new ImageData(w, h); d.data.fill(255); return d; })`,
		"noise":   `(function (x, y, w, h) { const d = new ImageData(w, h); for (let i = 0; i < d.data.length; i++) d.data[i] = (i * 7919) % 251; return d; })`,
		"refused": `(function () { throw new DOMException("no", "SecurityError"); })`,
	} {
		proto.Set("getImageData", js.Global().Call("eval", fake))
		var c browserCanvas
		for range 2 {
			if c.ready() {
				t.Errorf("%s: the canvas is used", name)
			}
		}
	}
	proto.Set("getImageData", was)
	var c browserCanvas
	if !c.ready() {
		t.Error("a browser that gives its pixels back has no canvas")
	}
}

// Whether the browser has a private use character is found with one look at what it draws for one it lacks.
func TestBrowserProbesPrivateUseCheaply(t *testing.T) {
	b := browserFontFor(t, false, 10)
	reads := count(t, "getImageData")
	mu.Lock()
	if b.draws(0xe123) {
		t.Error("U+E123, which no font here has, is drawn")
	}
	mu.Unlock()
	if n := reads(); n != 2 {
		t.Errorf("the first probe read %d pictures, want the two boxes", n)
	}
	// A font that has one: the browser's measure of it is not that of the box.
	proto := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	was := proto.Get("measureText")
	proto.Set("measureText", js.Global().Call("eval", `(was => function (s) {
		return s.codePointAt(0) == 0xe0a0 ? {width: 20, actualBoundingBoxLeft: 1, actualBoundingBoxRight: 19,
			actualBoundingBoxAscent: 30, actualBoundingBoxDescent: 2} : was.call(this, s);
	})`).Invoke(was))
	defer proto.Set("measureText", was)
	mu.Lock()
	defer mu.Unlock()
	if !b.draws(0xe0a0) {
		t.Error("a private use character the browser has is not drawn")
	}
	for r := rune(0xe200); r < 0xe300; r++ {
		b.draws(r)
	}
	if n := reads(); n != 2 {
		t.Errorf("%d pictures were read for %d probes", n, 0x102)
	}
	if !b.draws('中') {
		t.Error("a character that is not for private use is not drawn")
	}
}

// The browser draws a Hebrew letter and an emoji as themselves, and not as its box for what it has no font for. Which
// other scripts it has fonts for is its machine's business.
func TestBrowserDrawsGlyphsAndNotBoxes(t *testing.T) {
	b := browserFontFor(t, false, 10)
	draw := func(r rune) []byte { return b.rasterize(b.id([]rune{r}, false), 28, 0, false).Pix }
	box := draw(0x10fffd)
	if len(box) == 0 {
		t.Fatal("the browser draws nothing for what it lacks")
	}
	for _, r := range []rune{'א', 'Ж', 0x1f600} {
		if got := draw(r); len(got) == 0 || bytes.Equal(got, box) {
			t.Errorf("%c, %U, is the browser's box: is there a font for it?", r, r)
		}
	}
	for _, r := range []rune{'中', '한', '⇒', 'ไ', 'ب'} {
		if bytes.Equal(draw(r), box) {
			t.Logf("no font here has %c, %U", r, r)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !browserPad.paintsColor() {
		t.Error("the browser has no emoji in colour: install one, such as fonts-noto-color-emoji")
	}
}

// A word of letters that join is drawn whole, and one longer than words are in parts that show no cut: after a letter
// that joins no other, or with a joiner at either side.
func TestBrowserCutsLongWords(t *testing.T) {
	type part struct {
		from, to int
		text     string
	}
	cut := func(s string) (parts []part) {
		wordParts([]rune(s), func(from, to int, p []rune) { parts = append(parts, part{from, to, string(p)}) })
		return parts
	}
	if got := cut("مرحبا"); len(got) != 1 || got[0].text != "مرحبا" {
		t.Errorf("a word is in parts %+v", got)
	}
	for _, s := range []string{
		strings.Repeat("ب", 200), strings.Repeat("با", 100), strings.Repeat("بيت", 70),
		strings.Repeat("ب\u064e\u0651", 100), "ب" + strings.Repeat("\u064e", 500) + "ب",
	} {
		runes := []rune(s)
		parts := cut(s)
		at := 0
		for i, p := range parts {
			if p.from != at || p.to <= p.from {
				t.Fatalf("part %d of %d runes goes from %d to %d, after %d", i, len(runes), p.from, p.to, at)
			}
			at = p.to
			bare := strings.Trim(p.text, "\u200d")
			if bare != string(runes[p.from:p.to]) {
				t.Errorf("part %d is %q", i, p.text)
			}
			if n := len([]rune(p.text)); n > 4*wordMax+characterMax+2 {
				t.Errorf("part %d has %d runes", i, n)
			}
			if r, _ := utf8.DecodeRuneInString(bare); unicode.Is(unicode.Mn, r) && len(runes) < 400 {
				t.Errorf("part %d starts with a mark", i)
			}
			if i == 0 {
				continue
			}
			before := strings.HasSuffix(parts[i-1].text, "\u200d")
			after := strings.HasPrefix(p.text, "\u200d")
			if before != after {
				t.Errorf("the join between parts %d and %d is drawn on one side: %q and %q", i-1, i, parts[i-1].text, p.text)
			}
			if last := []rune(strings.Trim(parts[i-1].text, "\u200d")); !unicode.Is(unicode.Mn, last[len(last)-1]) && before != joinsOn(last[len(last)-1]) {
				t.Errorf("after %c a join is drawn: %v", last[len(last)-1], before)
			}
		}
		if at != len(runes) {
			t.Errorf("the parts end at %d of %d", at, len(runes))
		}
	}
	if parts := cut(strings.Repeat("با", 100)); strings.Contains(parts[0].text, "\u200d") {
		t.Errorf("a word that can be cut after an alef is not: %q", parts[0].text)
	}
}

// A heap of marks is no larger a glyph, nor a longer key.
func TestBrowserCutsHeapsOfMarks(t *testing.T) {
	s := "x" + strings.Repeat("\u0302", 100000)
	for _, g := range GoMono(false, false).Cell(s, 14, 8, 13, 4) {
		f, _ := Lookup(g.Face)
		mu.Lock()
		if e := f.browser.byID[g.ID]; e == nil || len(e.s) > 4*characterMax {
			t.Errorf("the glyph's key is %d bytes", len(e.s))
		}
		mu.Unlock()
	}
	n := 0
	characters([]rune(s), func(from, to int) {
		n++
		if to-from > characterMax {
			t.Fatalf("a character goes from %d to %d", from, to)
		}
	})
	if n < 100000/characterMax {
		t.Errorf("%d characters", n)
	}
}

// Cells drawn by the browser put nothing in the table text draws from.
func TestBrowserCellsLeaveTextAlone(t *testing.T) {
	f := GoMono(false, true)
	mu.Lock()
	b := browserFaceOf(f, false).browser
	mu.Unlock()
	was, gen := len(b.byID), generation.Load()
	for i := range rune(500) {
		f.Cell(string([]rune{'a' + i%26, 0x300 + i%0x60, 0x323}), 14, 8, 13, 4)
	}
	if len(b.byID) != was || generation.Load() != gen {
		t.Errorf("the table went from %d glyphs to %d", was, len(b.byID))
	}
}

// A line of the browser's words is one run, spaces and all, and wraps at its spaces as any text does.
func TestBrowserTextWrapsAtItsSpaces(t *testing.T) {
	for _, word := range []string{"한국어", "مرحبا", "שלום"} {
		s := strings.TrimSpace(strings.Repeat(word+" ", 12))
		f := GoSans(false, false)
		one := f.Shape(word, 20)
		p := f.Layout(s, Style{Size: 20}, 2.6*one.Advance)
		// Korean breaks between its syllables too.
		words := word != "한국어"
		if words && len(p.Lines) != 6 || p.Size.W > 2.6*one.Advance {
			t.Errorf("12 times %q wraps into %d lines, %v wide, in room for two and a half of %v", word, len(p.Lines), p.Size.W, one.Advance)
		}
		per := len([]rune(word)) + 1
		for i, l := range p.Lines {
			if words && l.Run.Start != 2*per*i {
				t.Errorf("line %d of %q starts at rune %d, want %d", i, word, l.Run.Start, 2*per*i)
			}
		}
		// The whole on one line: as wide as its words and their spaces.
		run := f.Shape(s, 20)
		space := f.Shape(word+" "+word, 20).Advance - 2*one.Advance
		if want := 12*one.Advance + 11*space; space < 2 || run.Advance < want-1 || run.Advance > want+1 {
			t.Errorf("12 times %q is %v wide, want %v, with a space of %v", word, run.Advance, want, space)
		}
	}
	// A space at the edge of them stays its own face's.
	run := GoSans(false, false).Shape("a 中 b", 20)
	if n := len(run.Glyphs); n != 5 {
		t.Fatalf("five characters are %d glyphs", n)
	}
	for _, i := range []int{1, 3} {
		if face, _ := Lookup(run.Glyphs[i].Face); face.FromBrowser() {
			t.Errorf("the space at %d is the browser's", i)
		}
	}
}

// Emoji and Chinese need no other face to be read first, as the faces built in are on their first use.
func TestBrowserColorsAreFoundWithTheFontsLocked(t *testing.T) {
	mu.Lock()
	defer mu.Unlock()
	browserPad.probed = false
	browserPad.paintsColor()
}
