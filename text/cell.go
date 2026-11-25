package text

import (
	"slices"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/text/unicode/norm"

	"github.com/marrasen/gunim/paint"
)

// Text in a grid of cells, such as a terminal's, skips shaping: each
// character takes its face's own glyph for it, placed at its cell.
// That is how terminals draw, and it keeps a screen that changes whole,
// as a scrolling one does, cheap to draw again.

var monoFaces = builtins{
	name:  "Go Mono",
	fonts: [4][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF},
}

// GoMono returns Go Mono, a monospaced face for code and terminals, in
// the style asked for.
func GoMono(bold, italic bool) *Face { return monoFaces.get(bold, italic) }

// builtins is a family of four built-in faces, each parsed on first use.
type builtins struct {
	name  string
	fonts [4][]byte
	once  [4]sync.Once
	faces [4]*Face
}

func (b *builtins) get(bold, italic bool) *Face {
	i := 0
	if bold {
		i |= 1
	}
	if italic {
		i |= 2
	}
	b.once[i].Do(func() {
		f, err := Parse(b.fonts[i])
		if err != nil {
			panic("text: the built-in " + b.name + " font fails to parse: " + err.Error())
		}
		b.faces[i] = f
	})
	return b.faces[i]
}

// Glyph returns the glyph that draws r on its own, from f, the faces it
// falls back to, the fonts installed on the system, or the browser's, with its origin
// at zero, and its advance at size logical pixels. ok is false when no
// font has r.
func (f *Face) Glyph(r rune, size float32) (g paint.Glyph, advance float32, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	if g, advance, ok = f.glyphLocked(r, size); !ok {
		return platformGlyph(f, r, size)
	}
	return g, advance, ok
}

// glyphLocked is [Face.Glyph] without the browser's fonts. It runs with mu held.
func (f *Face) glyphLocked(r rune, size float32) (g paint.Glyph, advance float32, ok bool) {
	ff := (&fontmap{f: f}).resolve(r)
	gid, ok := ff.NominalGlyph(r)
	if !ok {
		return paint.Glyph{}, 0, false
	}
	from := faceOf(ff)
	return paint.Glyph{ID: uint32(gid), Face: from.id}, ff.HorizontalAdvance(gid) * size / from.upem, true
}

// Cell returns the glyphs that draw s, one character with any marks over it, in a cell w wide that reaches ascent
// above its baseline and descent below it. They are placed from the cell's left edge on the baseline, in the middle
// of a cell wider than they are, as a fallback font's may be.
//
// A letter and its marks draw as one character where a font has it, and otherwise as the font sets the marks. In a
// browser, what no face has is drawn by the browser, small enough for the cell. A letter, number or symbol no font
// has draws as f's box for what it lacks, as it does in a paragraph. A character for private use, as icon fonts use,
// then draws nothing, and nor does one that shows nothing by itself.
func (f *Face) Cell(s string, size, w, ascent, descent float32) []paint.Glyph {
	runes := cellRunes(s)
	if len(runes) == 0 {
		return nil
	}
	if g, ok := f.browserCell(runes, false, size, w, ascent, descent); ok {
		return []paint.Glyph{g}
	}
	var glyphs []paint.Glyph
	var advance float32
	if len(runes) == 1 {
		mu.Lock()
		g, a, ok := f.glyphLocked(runes[0], size)
		mu.Unlock()
		if ok {
			glyphs, advance = []paint.Glyph{g}, a
		}
	} else if !platformLacks(f, runes) {
		// Not through the cache, where a terminal's would push out the text the interface is set in.
		run, _ := f.shapeUncached(string(runes), size)
		if run.drawn() {
			glyphs, advance = f.withoutHidden(run.Glyphs), run.Advance
			if !slices.ContainsFunc(runes[1:], func(r rune) bool { return !unicode.In(r, unicode.Mn, unicode.Me) }) {
				f.overLetter(glyphs, runes[0], size)
			}
		}
	}
	if glyphs == nil {
		if g, ok := f.browserCell(runes, true, size, w, ascent, descent); ok {
			return []paint.Glyph{g}
		}
		switch {
		case len(runes) > 1:
			return f.Cell(string(f.withoutMissing(runes)), size, w, ascent, descent)
		case !unicode.In(runes[0], unicode.L, unicode.N, unicode.P, unicode.S) || hidden(runes[0]) || runes[0] == '\u2800':
			return nil
		}
		run, _ := f.shapeUncached(string(runes), size)
		glyphs, advance = run.Glyphs, run.Advance
	}
	if advance > 0 && advance < w {
		for i := range glyphs {
			glyphs[i].At.X += (w - advance) / 2
		}
	}
	return glyphs
}

// browserCell is [platformCell], with mu held.
func (f *Face) browserCell(runes []rune, missing bool, size, w, ascent, descent float32) (paint.Glyph, bool) {
	mu.Lock()
	defer mu.Unlock()
	return platformCell(f, runes, missing, size, w, ascent, descent)
}

// characterMax is the most runes drawn as one character, as a heap of marks is none the clearer for more.
const characterMax = 32

// cellRunes returns the runes of s, one character, that are drawn: composed, where that leaves fewer, and without what
// draws nothing itself and changes nothing of this letter, such as a joiner between letters that do not join.
func cellRunes(s string) []rune {
	runes := make([]rune, 0, 4)
	for _, r := range s {
		if len(runes) == characterMax {
			break
		}
		runes = append(runes, r)
	}
	if len(runes) < 2 {
		// As it is, where composing could put another character in its place.
		return runes
	}
	// What is hidden asks for an emoji, joins two, or joins the letters of scripts whose letters join.
	if r := runes[0]; r < 0x590 && !emojiBase(r) && !keycap(runes) {
		runes = slices.DeleteFunc(runes, hidden)
	}
	if len(runes) < 2 {
		return runes
	}
	return []rune(norm.NFC.String(string(runes)))
}

// withoutMissing returns runes, a letter and its marks that did not draw together, without the marks no font has, or
// the letter alone where that is all of them.
func (f *Face) withoutMissing(runes []rune) []rune {
	mu.Lock()
	defer mu.Unlock()
	kept := runes[:1:1]
	for _, r := range runes[1:] {
		if _, _, ok := f.glyphLocked(r, 1); ok || hidden(r) {
			kept = append(kept, r)
		}
	}
	if len(kept) == len(runes) {
		return runes[:1]
	}
	return kept
}

// withoutHidden returns glyphs without those of characters that show nothing, for which a font gives its space.
func (f *Face) withoutHidden(glyphs []paint.Glyph) []paint.Glyph {
	mu.Lock()
	defer mu.Unlock()
	return slices.DeleteFunc(glyphs, func(g paint.Glyph) bool {
		space, ok := faces[g.Face].face.NominalGlyph(' ')
		return ok && uint32(space) == g.ID
	})
}

// overLetter puts each mark that comes from another face than its letter's over the middle of it. A face places its
// marks over letters of its own, and after another's they would hang at its edge.
func (f *Face) overLetter(glyphs []paint.Glyph, letter rune, size float32) {
	mu.Lock()
	defer mu.Unlock()
	of, advance, ok := f.glyphLocked(letter, size)
	if !ok {
		return
	}
	// The letter's own face, as text that reads from the right has its marks before it.
	at := slices.IndexFunc(glyphs, func(g paint.Glyph) bool { return g.Face == of.Face })
	if at < 0 {
		return
	}
	middle := glyphs[at].At.X + advance/2
	for i := range glyphs {
		g := &glyphs[i]
		if g.Face == of.Face {
			continue
		}
		face := faces[g.Face]
		if ink, ok := face.face.GlyphExtents(font.GID(g.ID)); ok {
			g.At.X = middle - (ink.XBearing+ink.Width/2)*size/face.upem
		}
	}
}

// drawn reports whether faces have every character of the run, without the browser's.
func (r Run) drawn() bool {
	return !slices.ContainsFunc(r.Glyphs, func(g paint.Glyph) bool {
		f, ok := Lookup(g.Face)
		return g.ID == 0 || !ok || f.FromBrowser()
	})
}

// hidden reports whether r draws nothing by itself, and is no glyph to miss: a joiner, a variation selector, or the
// like.
func hidden(r rune) bool {
	return unicode.In(r, unicode.Cf, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point)
}

// Metrics returns f's line extents at size logical pixels: how far it
// reaches above and below the baseline, and the gap it leaves between
// lines.
func (f *Face) Metrics(size float32) (ascent, descent, gap float32) {
	scale := size / f.upem
	return f.ascent * scale, f.descent * scale, f.gap * scale
}
