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
