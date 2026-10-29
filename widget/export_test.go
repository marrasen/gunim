package widget

import "github.com/marrasen/gunim/geom"

// RowY is where row k of l is now, from the list's top, for the tests.
func RowY(l *List, k Key) (float32, bool) {
	r, ok := l.rows[k]
	if !ok {
		return 0, false
	}
	return r.y.Value(), true
}

// SelectionAt is where a list's selection fill is now.
func SelectionAt(l *List) geom.Rect { return l.sel.Value() }

// BadgeShown is how far a badge has grown in, 0 to 1.
func BadgeShown(b *Badge) float32 { return b.shown.Value() }

// LogBack is how many lines a log view is scrolled up from the newest.
func LogBack(v *LogView) int { return v.back }

// KnobAt is how far a status switch's knob is towards on, 0 to 1.
func KnobAt(s *StatusSwitch) float32 { return s.knob.Value() }
