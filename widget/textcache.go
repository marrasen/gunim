package widget

import (
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// faceIn returns t's face in th, and [Font]'s when t is the zero token.
func faceIn(t theme.Token[*text.Face], th *theme.Live) *text.Face {
	if t.Key() == "" {
		t = Font
	}
	if f := t.Get(th); f != nil {
		return f
	}
	return text.Default()
}

// shapedText holds a string's shaped form and shapes it again only
// when the string, face or size changes, so a widget can measure and
// paint its text every frame at no cost.
type shapedText struct {
	face *text.Face
	s    string
	size float32
	run  text.Run
	ok   bool
}

// shape returns s shaped in face at size logical pixels.
func (l *shapedText) shape(face *text.Face, s string, size float32) text.Run {
	if !l.ok || l.face != face || l.s != s || l.size != size {
		l.face, l.s, l.size, l.run, l.ok = face, s, size, face.Shape(s, size), true
	}
	return l.run
}

// laidText holds a string's laid-out form and lays it out again only
// when the string, face, style or width changes.
type laidText struct {
	face  *text.Face
	s     string
	style text.Style
	width float32
	p     text.Paragraph
	ok    bool
}

// layout returns s laid out in face at width.
func (pr *laidText) layout(face *text.Face, s string, st text.Style, width float32) text.Paragraph {
	if !pr.ok || pr.face != face || pr.s != s || pr.style != st || pr.width != width {
		pr.face, pr.s, pr.style, pr.width, pr.ok = face, s, st, width, true
		pr.p = face.Layout(s, st, width)
	}
	return pr.p
}
