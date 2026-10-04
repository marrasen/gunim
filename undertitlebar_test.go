package gunim

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// barStub is a title bar 30 high across the top, that notes whether it was told the application draws under it.
type barStub struct {
	recorder
	under bool
}

func (b *barStub) Layout(c Constraints, _ Frame, _ Children) geom.Size { return geom.Sz(c.Max.W, 30) }
func (b *barStub) Paint(_ *paint.Painter, f Frame, _ geom.Size, _ Children) {
	b.under = f.UnderTitleBar()
}
func (b *barStub) SetTitle(string) {}
func (b *barStub) CaptionRects(size geom.Size) []geom.Rect {
	return []geom.Rect{{Max: size.Point()}}
}

// safeSpy fills what it is given, and notes the Safe its layout and its paint were given.
type safeSpy struct {
	recorder
	laid, painted geom.Insets
}

func (s *safeSpy) Layout(c Constraints, f Frame, _ Children) geom.Size {
	s.laid = f.Safe
	return c.Max
}
func (s *safeSpy) Paint(_ *paint.Painter, f Frame, _ geom.Size, _ Children) { s.painted = f.Safe }

// underBarStage is a chromeless window whose application, app, draws under or below a title bar.
func underBarStage(t *testing.T, under bool) (*Window, *barStub, *safeSpy) {
	t.Helper()
	app := &safeSpy{}
	w := NewOffscreen(geom.Sz(800, 600), app)
	w.ui.underBar = under
	w.Offscreen().MakeChromeless(true)
	bar := &barStub{}
	w.ui.startChrome(bar)
	run(w, 2)
	return w, bar, app
}

// An application that draws under the title bar fills the window, and is told the bar covers its top, as a phone's
// status bar would.
func TestAnApplicationUnderTheTitleBarFillsTheWindow(t *testing.T) {
	w, bar, app := underBarStage(t, true)
	if r, _ := w.ui.Bounds(app); r != geom.Rc(0, 0, 800, 600) {
		t.Fatalf("the application is at %v, want the whole window", r)
	}
	want := geom.Insets{Top: 30}
	if app.laid != want || app.painted != want {
		t.Fatalf("the application was laid out with Safe %v and painted with %v, want %v", app.laid, app.painted, want)
	}
	if !bar.under {
		t.Fatal("the title bar was not told the application draws under it")
	}
	// Full screen, the bar steps aside, and covers nothing
	w.ui.SetFullScreen(true)
	run(w, 2)
	if app.laid != (geom.Insets{}) || app.painted != (geom.Insets{}) {
		t.Fatalf("full screen, the application was given Safe %v and %v, want none", app.laid, app.painted)
	}
}

// By default the application sits below the title bar, with nothing over it.
func TestAnApplicationSitsBelowTheTitleBar(t *testing.T) {
	w, bar, app := underBarStage(t, false)
	if r, _ := w.ui.Bounds(app); r != geom.Rc(0, 30, 800, 570) {
		t.Fatalf("the application is at %v, want below the bar", r)
	}
	if app.laid != (geom.Insets{}) || app.painted != (geom.Insets{}) {
		t.Fatalf("the application was given Safe %v and %v, want none", app.laid, app.painted)
	}
	if bar.under {
		t.Fatal("the title bar was told the application draws under it")
	}
}
