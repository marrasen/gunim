package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// measured is a root that records the room it was laid out in.
type measured struct{ room geom.Size }

func (m *measured) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	m.room = c.Max
	return c.Max
}

func (m *measured) Paint(*paint.Painter, Frame, geom.Size, Children) {}

func newZoomWindow(t *testing.T) (*Window, *measured) {
	t.Helper()
	m := &measured{}
	w := NewOffscreen(geom.Sz(800, 600), m)
	w.ui.zoomKeys = true
	run(w, 1)
	return w, m
}

// zooms returns the zooms reported since the last call.
func zooms(w *Window) []float32 {
	var out []float32
	for {
		select {
		case e := <-w.out:
			if z, ok := e.Intent.(Zoomed); ok {
				out = append(out, z.Zoom)
			}
		default:
			return out
		}
	}
}

func ctrl(key input.Key, char rune) input.KeyPress {
	return input.KeyPress{Key: key, Char: char, Mods: input.ModControl, Time: time.Now()}
}

func TestZoomLaysTheContentOutInLessRoomAndDrawsItLarger(t *testing.T) {
	w, m := newZoomWindow(t)
	w.ui.SetZoom(2)
	run(w, 1)
	if m.room != geom.Sz(400, 300) {
		t.Errorf("at zoom 2 the content has %v, want 400x300", m.room)
	}
	if got := w.Offscreen().Scale(); got != 2 {
		t.Errorf("at zoom 2 the scale is %v, want 2", got)
	}
	if got := zooms(w); len(got) != 0 {
		t.Errorf("SetZoom from the application reported %v, want nothing", got)
	}
}

func TestZoomKeysStepThroughTheZooms(t *testing.T) {
	w, _ := newZoomWindow(t)
	for _, c := range []struct {
		ev   input.KeyPress
		want float32
	}{
		{ctrl(input.KeyEqual, '='), 1.1},
		{ctrl(input.KeyKPAdd, 0), 1.25},
		{ctrl(input.KeyMinus, '-'), 1.1},
		// The + key of a Swedish keyboard sits where a US one has -
		{ctrl(input.KeyMinus, '+'), 1.25},
		{ctrl(input.Key0, '0'), 1},
		{ctrl(input.KeyMinus, 0), 0.9},
		// The 0 key of a French keyboard types à
		{ctrl(input.Key0, 'à'), 1},
	} {
		w.ui.handlePlatform(c.ev)
		if w.ui.Zoom() != c.want {
			t.Fatalf("after Ctrl with %v %q the zoom is %v, want %v", c.ev.Key, c.ev.Char, w.ui.Zoom(), c.want)
		}
	}
	if got := zooms(w); len(got) != 7 {
		t.Errorf("reported %v, want each of the 7 changes", got)
	}
}

func TestZoomKeysAreOffUnlessAsked(t *testing.T) {
	w, _ := newZoomWindow(t)
	w.ui.zoomKeys = false
	w.ui.handlePlatform(ctrl(input.KeyEqual, '='))
	if w.ui.Zoom() != 1 {
		t.Errorf("without ZoomKeys, Ctrl+= zoomed to %v", w.ui.Zoom())
	}
}

func TestTheWheelZoomsByWholeNotches(t *testing.T) {
	w, _ := newZoomWindow(t)
	wheel := func(y float32) {
		w.ui.handlePlatform(input.Scroll{Pos: geom.Pt(10, 10), Notches: geom.Pt(0, y), Mods: input.ModControl, Time: time.Now()})
	}
	wheel(0.5)
	if w.ui.Zoom() != 1 {
		t.Fatalf("half a notch zoomed to %v", w.ui.Zoom())
	}
	wheel(0.5)
	if w.ui.Zoom() != 1.1 {
		t.Fatalf("a whole notch up zoomed to %v, want 1.1", w.ui.Zoom())
	}
	wheel(-2)
	if w.ui.Zoom() != 0.9 {
		t.Fatalf("two notches down zoomed to %v, want 0.9", w.ui.Zoom())
	}
}

func TestZoomStaysWithinItsBounds(t *testing.T) {
	w, _ := newZoomWindow(t)
	for range 20 {
		w.ui.handlePlatform(ctrl(input.KeyEqual, '='))
	}
	if w.ui.Zoom() != zoomSteps[len(zoomSteps)-1] {
		t.Errorf("zooming in and in stops at %v, want %v", w.ui.Zoom(), zoomSteps[len(zoomSteps)-1])
	}
	w.ui.SetZoom(100)
	if w.ui.Zoom() != MaxZoom {
		t.Errorf("SetZoom(100) gave %v, want %v", w.ui.Zoom(), MaxZoom)
	}
	w.ui.SetZoom(0)
	if w.ui.Zoom() != 1 {
		t.Errorf("SetZoom(0) gave %v, want 1", w.ui.Zoom())
	}
}

func TestSetZoomCrossesTheWire(t *testing.T) {
	b, err := MarshalCommand(SetZoom{Zoom: 1.25})
	if err != nil {
		t.Fatal(err)
	}
	c, err := UnmarshalCommand(b)
	if err != nil {
		t.Fatal(err)
	}
	if c != (SetZoom{Zoom: 1.25}) {
		t.Errorf("the command came back as %#v", c)
	}
	w, m := newZoomWindow(t)
	if err := w.Client().Send(c); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	if m.room != geom.Sz(640, 480) {
		t.Errorf("after SetZoom 1.25 the content has %v, want 640x480", m.room)
	}
}

func TestAPopupTakesTheWindowsZoom(t *testing.T) {
	w, _, _, _ := openMenu(t, nil)
	pw := popupWindow(t, w)
	w.ui.SetZoom(2)
	run(w, 1)
	if pw.Zoom() != 2 || pw.Scale() != 2 {
		t.Errorf("the popup has zoom %v and scale %v, want 2 and 2", pw.Zoom(), pw.Scale())
	}
	if got := pw.Size(); got != geom.Sz(120, 80) {
		t.Errorf("the popup lays out in %v, want its content's 120x80", got)
	}
}

// wheelTaker zooms with Ctrl and the wheel while on, and counts the scrolls it hears.
type wheelTaker struct {
	on     bool
	scroll int
}

func (w *wheelTaker) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }

func (w *wheelTaker) Paint(*paint.Painter, Frame, geom.Size, Children) {}

func (w *wheelTaker) ZoomsWithWheel() bool { return w.on }

func (w *wheelTaker) Handle(e input.Event, _ *UI) bool {
	if s, ok := e.(input.Scroll); ok && s.Mods.Has(input.ModControl) {
		w.scroll++
		return true
	}
	return false
}

func TestAWheelZoomerTakesCtrlWithTheWheel(t *testing.T) {
	taker := &wheelTaker{on: true}
	w := NewOffscreen(geom.Sz(800, 600), &Box{})
	w.ui.zoomKeys = true
	w.ui.Insert(w.ui.Root(), taker)
	run(w, 2)
	wheel := func() {
		w.ui.handlePlatform(input.Scroll{Pos: geom.Pt(10, 10), Notches: geom.Pt(0, 1), Mods: input.ModControl, Time: time.Now()})
	}
	wheel()
	if w.ui.Zoom() != 1 || taker.scroll != 1 {
		t.Fatalf("over a wheel zoomer the window zoomed to %v and the node heard %d scrolls", w.ui.Zoom(), taker.scroll)
	}
	taker.on = false
	wheel()
	if w.ui.Zoom() != 1.1 || taker.scroll != 1 {
		t.Fatalf("with the zoomer off the window zoomed to %v and the node heard %d scrolls", w.ui.Zoom(), taker.scroll)
	}
	if got := zooms(w); len(got) != 1 {
		t.Errorf("reported %v, want the one zoom of the window", got)
	}
}

// Ctrl+Shift with the 0 key types ) on a US layout, and is left to the
// keys around: the zoom stays.
func TestCtrlShiftZeroLeavesTheZoom(t *testing.T) {
	w, _ := newZoomWindow(t)
	w.ui.SetZoom(1.25)
	e := ctrl(input.Key0, ')')
	e.Mods |= input.ModShift
	w.ui.handlePlatform(e)
	if w.ui.Zoom() != 1.25 {
		t.Fatalf("Ctrl+Shift+0 set the zoom to %v, want it left at 1.25", w.ui.Zoom())
	}
}
