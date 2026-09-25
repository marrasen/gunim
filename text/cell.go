package text

import (
	"sync"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"

	"github.com/marrasen/gunim/paint"
)

// Text in a grid of cells, such as a terminal's, skips shaping: each
// character takes its face's own glyph for it, placed at its cell.
// That is how terminals draw, and it keeps a screen that changes whole,
// as a scrolling one does, cheap to draw again.

var (
	monoOnce  sync.Once
	monoFaces [4]*Face
)

// GoMono returns Go Mono, a monospaced face for code and terminals, in
// the style asked for.
func GoMono(bold, italic bool) *Face {
	monoOnce.Do(func() {
		for i, data := range [4][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF} {
			f, err := Parse(data)
			if err != nil {
				panic("text: the built-in Go Mono font fails to parse: " + err.Error())
			}
			monoFaces[i] = f
		}
	})
	i := 0
	if bold {
		i |= 1
	}
	if italic {
		i |= 2
	}
	return monoFaces[i]
}

// Glyph returns the glyph that draws r on its own, from f, the faces it
// falls back to, or the fonts installed on the system, with its origin
// at zero, and its advance at size logical pixels. ok is false when no
// font has r.
func (f *Face) Glyph(r rune, size float32) (g paint.Glyph, advance float32, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	ff := fontmap{f}.ResolveFace(r)
	gid, ok := ff.NominalGlyph(r)
	if !ok {
		return paint.Glyph{}, 0, false
	}
	from := faceOf(ff)
	return paint.Glyph{ID: uint32(gid), Face: from.id}, ff.HorizontalAdvance(gid) * size / from.upem, true
}

// Metrics returns f's line extents at size logical pixels: how far it
// reaches above and below the baseline, and the gap it leaves between
// lines.
func (f *Face) Metrics(size float32) (ascent, descent, gap float32) {
	scale := size / f.upem
	return f.ascent * scale, f.descent * scale, f.gap * scale
}
