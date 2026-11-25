package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"golang.org/x/image/font/gofont/goregular"
)

// What keeps text shaped shapes it again once the fonts change what it is shaped into: a face's fallbacks here, as the
// browser's glyphs dropped do in a browser, after which what was kept draws nothing.
func TestKeptTextIsShapedAgainAfterTheFontsChange(t *testing.T) {
	withoutSystemFonts(t)
	// A character Go Regular lacks, and Inter has.
	const s = "x̂"
	runOf := func(p text.Paragraph) text.Run { return p.Lines[0].Run }
	for name, keep := range map[string]func(face *text.Face) func() text.Run{
		"shapedText": func(face *text.Face) func() text.Run {
			var l shapedText
			return func() text.Run { return l.shape(face, s, 14) }
		},
		"laidText": func(face *text.Face) func() text.Run {
			var l laidText
			return func() text.Run { return runOf(l.layout(face, s, text.Style{Size: 14}, 0)) }
		},
		"a long line": func(face *text.Face) func() text.Run {
			var l longLine
			changed := true
			return func() text.Run {
				l.update(face, 14, []rune(s), 0, 0, 0, changed)
				changed = false
				return l.pieces[0].run
			}
		},
		"an area's lines": func(face *text.Face) func() text.Run {
			var a areaText
			changed := true
			return func() text.Run {
				a.update(face, text.Style{Size: 14}, 500, []rune(s), 0, 0, 0, changed)
				changed = false
				return runOf(a.paras[0].p)
			}
		},
		"rich text": func(face *text.Face) func() text.Run {
			r := NewRichText(RichSpan{Text: s})
			th := themeWith(face)
			return func() text.Run { return r.paragraph(th, 500).Lines[0].Pieces[0].Run }
		},
		"a data grid": func(face *text.Face) func() text.Run {
			g := NewDataGrid()
			return func() text.Run { return g.shape(spanKey{}, face, s, 14) }
		},
		"a code editor": func(face *text.Face) func() text.Run {
			c := NewCodeEditor()
			c.SetText(s, nil)
			th := themeWith(face)
			return func() text.Run {
				c.relayout(th)
				return c.lines[0].segs[0].run
			}
		},
	} {
		face, err := text.Parse(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		shaped := keep(face)
		missing := func(run text.Run) bool {
			for _, g := range run.Glyphs {
				if g.ID == 0 {
					return true
				}
			}
			return len(run.Glyphs) == 0
		}
		if !missing(shaped()) {
			t.Fatalf("%s: the face has the mark", name)
		}
		face.Fallback(text.Inter(false))
		if missing(shaped()) {
			t.Errorf("%s keeps the text as it was", name)
		}
	}
}

// themeWith returns a theme that sets text in face.
func themeWith(face *text.Face) *theme.Live {
	return theme.NewLive(theme.Make("test", theme.Set(Font, face), theme.Set(MonoFont, face)))
}

// What a drawing kept shows is listed, for the fonts to hold on to.
func TestARecordingListsItsGlyphs(t *testing.T) {
	var p paint.Painter
	mark := p.Mark()
	run := text.Default().Shape("abc", 14)
	run.Paint(&p, geom.Pt(0, 0), Ink.Default())
	var rec paint.Recording
	p.Keep(mark, &rec)
	if got := rec.Glyphs(); len(got) != 3 || got[0].ID != run.Glyphs[0].ID {
		t.Errorf("the recording lists %+v", got)
	}
}
