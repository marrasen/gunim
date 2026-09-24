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
