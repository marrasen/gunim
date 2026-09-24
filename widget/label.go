package widget

import "github.com/marrasen/gunim/text"

// label holds a string's shaped form and shapes it again only when the
// string or size changes, so a widget can measure and paint its text
// every frame at no cost.
type label struct {
	s    string
	size float32
	run  text.Run
	ok   bool
}

// shape returns s shaped in the default face at size logical pixels.
func (l *label) shape(s string, size float32) text.Run {
	if !l.ok || l.s != s || l.size != size {
		l.s, l.size, l.run, l.ok = s, size, text.Default().Shape(s, size), true
	}
	return l.run
}

// paragraph holds a string's laid-out form and lays it out again only
// when the string, style or width changes.
type paragraph struct {
	s     string
	style text.Style
	width float32
	p     text.Paragraph
	ok    bool
}

// layout returns s laid out in the default face at width.
func (pr *paragraph) layout(s string, st text.Style, width float32) text.Paragraph {
	if !pr.ok || pr.s != s || pr.style != st || pr.width != width {
		pr.s, pr.style, pr.width, pr.ok = s, st, width, true
		pr.p = text.Default().Layout(s, st, width)
	}
	return pr.p
}
