package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// dragRig is a dialog staged to drag: its body, its window, and a point on its title bar.
type dragRig struct {
	d    *Dialog
	body *spot
	w    *gunim.Window
	grip geom.Point
}

// dragStage is barWindow as a dragRig.
func dragStage(t *testing.T, title string) *dragRig {
	t.Helper()
	r := &dragRig{}
	r.d, r.body, r.w = barWindow(t, title, func(*Dialog) {})
	at := r.panel()
	r.grip = geom.Pt(at.Center().X, at.Min.Y+10)
	return r
}

// run runs n frames.
func (r *dragRig) run(n int) {
	for range n {
		r.w.Frame(time.Second / 60)
	}
}

// panel is the panel as it stands.
func (r *dragRig) panel() geom.Rect { return r.d.panel(geom.Sz(800, 600), gunim.Frame{}) }

// nearPoint reports how a is off b by more than half a pixel, or nil.
func nearPoint(a, b geom.Point) error {
	if err := near(a.X, b.X); err != nil {
		return err
	}
	return near(a.Y, b.Y)
}

// A press on the title bar drags the panel: it follows the pointer on every frame, with what it holds, stays where
// it is let go, and a pointer moving with no button down leaves it there.
func TestADialogDragsByItsTitleBar(t *testing.T) {
	r := dragStage(t, "Rename the file")
	body, w, run, panel, grip := r.body, r.w, r.run, r.panel, r.grip
	was, stood := body.at, panel()
	pressDrag(w, run, grip)
	var by geom.Point
	for i := 1; i <= 10; i++ {
		by = geom.Pt(float32(i)*12, float32(i)*-7)
		w.Input(input.PointerMove{Pos: grip.Add(by), Time: time.Now()})
		run(1)
		if err := nearPoint(body.at, was.Add(by)); err != nil {
			t.Fatalf("dragged by %v, the body is at %v, want %v: %v", by, body.at, was.Add(by), err)
		}
		if err := nearPoint(panel().Min, stood.Min.Add(by)); err != nil {
			t.Fatalf("dragged by %v, the panel is at %v: %v", by, panel().Min, err)
		}
	}
	w.Input(input.PointerUp{Pos: grip.Add(by), Button: input.ButtonPrimary, Time: time.Now()})
	run(30)
	if err := nearPoint(body.at, was.Add(by)); err != nil {
		t.Fatalf("let go, the body went to %v: %v", body.at, err)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(40, 500), Time: time.Now()})
	run(2)
	if err := nearPoint(body.at, was.Add(by)); err != nil {
		t.Fatalf("with no button down the pointer moved the body to %v: %v", body.at, err)
	}
	// It is taken hold of again where it now stands.
	grip = grip.Add(by)
	pressDrag(w, run, grip, grip.Add(geom.Pt(-30, 20)))
	if err := nearPoint(body.at, was.Add(by).Add(geom.Pt(-30, 20))); err != nil {
		t.Fatalf("dragged again, the body is at %v: %v", body.at, err)
	}
}

// A drag ends as any button comes up: the engine lets go of the press then, and the pointer no longer moves the panel.
func TestADialogsDragEndsAsAnyButtonComesUp(t *testing.T) {
	r := dragStage(t, "Rename the file")
	body, w, run, grip := r.body, r.w, r.run, r.grip
	was := body.at
	pressDrag(w, run, grip, grip.Add(geom.Pt(40, 30)))
	at := grip.Add(geom.Pt(40, 30))
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonSecondary, Time: time.Now()})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonSecondary, Time: time.Now()})
	w.Input(input.PointerMove{Pos: at.Add(geom.Pt(100, 100)), Time: time.Now()})
	run(2)
	if err := nearPoint(body.at, was.Add(geom.Pt(40, 30))); err != nil || r.d.DragsTouch() {
		t.Fatalf("with the other button let go, the pointer moved the body to %v: %v", body.at, err)
	}
}

// A drag keeps the panel whole inside the window, and the panel waits at the edge while the pointer is past it.
func TestADraggedDialogStaysInsideTheWindow(t *testing.T) {
	r := dragStage(t, "Rename the file")
	w, run, panel, grip := r.w, r.run, r.panel, r.grip
	size := panel().Size()
	pressDrag(w, run, grip, grip.Add(geom.Pt(1000, 1000)))
	if got, want := panel(), geom.Rc(800-size.W, 600-size.H, size.W, size.H); nearPoint(got.Min, want.Min) != nil {
		t.Fatalf("dragged far down and right, the panel is at %v, want %v", got, want)
	}
	w.Input(input.PointerMove{Pos: grip.Add(geom.Pt(-2000, -2000)), Time: time.Now()})
	run(1)
	if got := panel(); nearPoint(got.Min, geom.Point{}) != nil {
		t.Fatalf("dragged far up and left, the panel is at %v, want the window's corner", got)
	}
	w.Input(input.PointerMove{Pos: grip.Add(geom.Pt(-1950, -1960)), Time: time.Now()})
	run(1)
	if got := panel(); nearPoint(got.Min, geom.Point{}) != nil {
		t.Fatalf("with the pointer still far outside, the panel went to %v", got)
	}
}

// The panel stays clear of what the system draws over the window's edges.
func TestADraggedDialogStaysClearOfTheSystemsBars(t *testing.T) {
	r := dragStage(t, "Rename the file")
	w, run, panel := r.w, r.run, r.panel
	w.Offscreen().SetSafeArea(geom.Insets{Top: 48, Left: 10})
	run(2)
	at := panel()
	grip := geom.Pt(at.Center().X, at.Min.Y+10)
	pressDrag(w, run, grip, grip.Add(geom.Pt(-2000, -2000)))
	if got := panel(); nearPoint(got.Min, geom.Pt(10, 48)) != nil {
		t.Fatalf("dragged far up and left, the panel is at %v, want clear of the bars at 10, 48", got)
	}
}

// Only the title bar drags: a press under it leaves the panel where it is, as one on a dialog with no bar does.
func TestOnlyTheTitleBarDragsADialog(t *testing.T) {
	r := dragStage(t, "Rename the file")
	d, body, w, run, panel := r.d, r.body, r.w, r.run, r.panel
	was, at := body.at, panel()
	under := geom.Pt(at.Min.X+5, at.Min.Y+d.barHeight(nil)+5)
	pressDrag(w, run, under, under.Add(geom.Pt(60, 40)))
	if body.at != was || d.DragsTouch() {
		t.Fatalf("a press under the title bar dragged the body from %v to %v", was, body.at)
	}

	r = dragStage(t, "")
	d, body, w, run, panel = r.d, r.body, r.w, r.run, r.panel
	was, at = body.at, panel()
	top := geom.Pt(at.Center().X, at.Min.Y+5)
	pressDrag(w, run, top, top.Add(geom.Pt(60, 40)))
	if body.at != was || d.DragsTouch() {
		t.Fatalf("a press on a dialog with no title bar dragged the body from %v to %v", was, body.at)
	}
}

// Alt with an arrow key glides the panel a step, four with Shift, and no further than the window's edge.
func TestAltAndAnArrowMoveADialog(t *testing.T) {
	r := dragStage(t, "Rename the file")
	body, w, run, panel := r.body, r.w, r.run, r.panel
	was := body.at
	w.Input(input.KeyPress{Key: input.KeyRight, Mods: input.ModAlt})
	last := was.X
	for i := range 60 {
		run(1)
		if i == 0 && (body.at.X <= was.X || body.at.X >= was.X+dialogStep-1) {
			t.Fatalf("a frame after the key the body is at %v, from %v: want it on its way", body.at.X, was.X)
		}
		if body.at.X < last-1 || body.at.X > was.X+dialogStep+3 {
			t.Fatalf("on frame %d the body is at %v, after %v", i, body.at.X, last)
		}
		last = body.at.X
	}
	if err := nearPoint(body.at, was.Add(geom.Pt(dialogStep, 0))); err != nil {
		t.Fatalf("after Alt+Right the body is at %v: %v", body.at, err)
	}
	w.Input(input.KeyPress{Key: input.KeyUp, Mods: input.ModAlt | input.ModShift})
	run(60)
	if err := nearPoint(body.at, was.Add(geom.Pt(dialogStep, -4*dialogStep))); err != nil {
		t.Fatalf("after Alt+Shift+Up the body is at %v: %v", body.at, err)
	}
	for range 20 {
		w.Input(input.KeyPress{Key: input.KeyLeft, Mods: input.ModAlt | input.ModShift})
		w.Input(input.KeyPress{Key: input.KeyDown, Mods: input.ModAlt | input.ModShift})
	}
	run(120)
	size := panel().Size()
	if got := panel(); nearPoint(got.Min, geom.Pt(0, 600-size.H)) != nil {
		t.Fatalf("moved far left and down by the keys, the panel is at %v, want the window's bottom left", got)
	}
}

// A dialog with no title bar keeps its place under Alt and an arrow.
func TestAltAndAnArrowLeaveADialogWithNoBar(t *testing.T) {
	r := dragStage(t, "")
	body, w, run := r.body, r.w, r.run
	was := body.at
	w.Input(input.KeyPress{Key: input.KeyRight, Mods: input.ModAlt})
	run(30)
	if body.at != was {
		t.Fatalf("Alt+Right moved a dialog with no title bar from %v to %v", was, body.at)
	}
}

// The panel is the dialog's own to press where it lies over a title bar the application draws, wherever it is
// dragged.
func TestADialogCoversTheCaptionUnderItsPanel(t *testing.T) {
	r := dragStage(t, "Rename the file")
	d, w, run, panel, grip := r.d, r.w, r.run, r.panel, r.grip
	pressDrag(w, run, grip, grip.Add(geom.Pt(90, -2000)))
	if got := d.CoversCaption(geom.Sz(800, 600)); len(got) != 1 || got[0] != panel() || got[0].Min.Y != 0 {
		t.Fatalf("dragged to the top, the dialog covers %v, want its panel %v", got, panel())
	}
}
