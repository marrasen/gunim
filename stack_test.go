package gunim

import (
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// fakeStacker says how deep each window lies, and counts the times it
// was asked. A window it does not know gets -1.
type fakeStacker struct {
	depth map[driver.Window]int
	asked int
}

func (s *fakeStacker) Depths(ws []driver.Window) []int {
	s.asked++
	out := make([]int, len(ws))
	for i, w := range ws {
		d, ok := s.depth[w]
		if !ok {
			d = -1
		}
		out[i] = d
	}
	return out
}

// stacked returns three 800 by 600 windows of one application on a
// pretend screen: a at 0,0, b at 400,0 and c at 1000,0, so that a and
// b overlap from 400 to 800, and b and c from 1000 to 1200.
func stacked(t *testing.T, s driver.Stacker) (ws *windows, a, b, c *Window) {
	t.Helper()
	app := &App{}
	app.windows.stacker = s
	a, b, c = newTestWindow(), newTestWindow(), newTestWindow()
	for i, w := range []*Window{a, b, c} {
		w.app = app
		app.windows.add(w)
		w.mustOffscreen(t).SetOrigin(geom.Pt([]float32{0, 400, 1000}[i], 0))
	}
	return &app.windows, a, b, c
}

func TestADragGoesToTheWindowInFrontWhereWindowsOverlap(t *testing.T) {
	s := &fakeStacker{}
	ws, a, b, c := stacked(t, s)
	s.depth = map[driver.Window]int{a.dw: 5, b.dw: 2, c.dw: 0}
	both := geom.Pt(600, 100)
	if got := ws.at(both, a); got != b {
		t.Fatalf("a drag from a over a and b went to %p, want b %p, which is in front", got, b)
	}
	if got := ws.at(geom.Pt(1100, 100), a); got != c {
		t.Fatalf("a drag over b and c went to %p, want c %p, which is in front", got, c)
	}
	s.depth[a.dw] = 1
	if got := ws.at(both, a); got != a {
		t.Fatalf("a drag from a, in front, went to %p, want a %p", got, a)
	}
}

func TestADragAsksTheStackOnlyWhereWindowsOverlap(t *testing.T) {
	s := &fakeStacker{}
	ws, a, b, _ := stacked(t, s)
	if got := ws.at(geom.Pt(100, 100), b); got != a {
		t.Fatalf("a drag over a alone went to %p, want a %p", got, a)
	}
	if got := ws.at(geom.Pt(900, 100), a); got != b {
		t.Fatalf("a drag over b alone went to %p, want b %p", got, b)
	}
	if got := ws.at(geom.Pt(100, 700), a); got != nil {
		t.Fatalf("a drag over no window went to %p", got)
	}
	if s.asked != 0 {
		t.Fatalf("the stack was asked %d times with one window or none under the drag, want none", s.asked)
	}
	ws.at(geom.Pt(600, 100), a)
	if s.asked != 1 {
		t.Fatalf("the stack was asked %d times where two windows overlap, want once", s.asked)
	}
}

func TestADragWithoutTheStackPrefersItsSourceThenTheWindowFocusedLast(t *testing.T) {
	for _, s := range []driver.Stacker{nil, &fakeStacker{}} {
		ws, a, b, c := stacked(t, s)
		both := geom.Pt(600, 100)
		ws.focus(b)
		ws.focus(a)
		if got := ws.at(both, a); got != a {
			t.Fatalf("with %T, a drag from a over a and b went to %p, want a %p", s, got, a)
		}
		if got := ws.at(both, c); got != a {
			t.Fatalf("with %T, a drag from c over a and b went to %p, want a %p, focused last", s, got, a)
		}
		ws.focus(b)
		if got := ws.at(both, c); got != b {
			t.Fatalf("with %T, a drag from c over a and b went to %p, want b %p, focused last", s, got, b)
		}
	}
}

func TestAWindowTakingTheKeyboardIsRecorded(t *testing.T) {
	ws, a, b, _ := stacked(t, nil)
	b.ui.handlePlatform(driver.WindowFocus{Focused: true})
	a.ui.handlePlatform(driver.WindowFocus{Focused: true})
	a.ui.handlePlatform(driver.WindowFocus{Focused: false})
	if ws.focusedAt[a] <= ws.focusedAt[b] || ws.focusedAt[b] == 0 {
		t.Fatalf("focused a at %d and b at %d, want b then a", ws.focusedAt[a], ws.focusedAt[b])
	}
}
