package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
)

// barStage mounts a dialog titled title, with a spot for its body and set up by set, lets it settle, and returns
// the spot, the dialog's panel and the ops of the last frame.
func barStage(t *testing.T, title string, set func(*Dialog)) (*spot, geom.Rect, []paint.Op) {
	d, body, w := barWindow(t, title, set)
	panel := d.panel(geom.Sz(800, 600), gunim.Frame{})
	return body, panel, w.Offscreen().Ops()
}

// barWindow is barStage's window.
func barWindow(t *testing.T, title string, set func(*Dialog)) (*Dialog, *spot, *gunim.Window) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	var d *Dialog
	body := newSpot(100, 40)
	gunim.RegisterView(w, "dialog", func(title string) *Dialog {
		d = NewDialog(title)
		d.Body = body
		set(d)
		return d
	}, func(d *Dialog, title string, _ *gunim.UI) { d.SetTitle(title) })
	if err := w.Client().Mount(gunim.Root, "dialog", "dialog", title); err != nil {
		t.Fatal(err)
	}
	for range 120 {
		w.Frame(time.Second / 60)
	}
	return d, body, w
}

// barFills returns the rectangles painted in the title bar's fill, in window space.
func barFills(ops []paint.Op) []geom.Rect {
	var out []geom.Rect
	for _, op := range ops {
		if r, ok := op.(*paint.RRectOp); ok && r.Fill.Solid == MenubarFill.Default() && r.Fill.Gradient == nil {
			out = append(out, geom.Rect{Min: r.Transform.Apply(r.Rect.Min), Max: r.Transform.Apply(r.Rect.Max)})
		}
	}
	return out
}

// textsIn counts the text ops in ops whose tops lie between top and bottom.
func textsIn(ops []paint.Op, top, bottom float32) int {
	n := 0
	for _, op := range ops {
		if o, ok := op.(*paint.TextOp); ok && o.Transform.F >= top && o.Transform.F < bottom {
			n++
		}
	}
	return n
}

func TestADialogShowsItsTitleInATitleBar(t *testing.T) {
	body, panel, ops := barStage(t, "Rename the file", func(*Dialog) {})
	bar, pad := TitleBarCompactHeight.Default(), DialogPadding.Default()
	fills := barFills(ops)
	if len(fills) == 0 || near(fills[0].Min.Y, panel.Min.Y) != nil || fills[0].Min.X != panel.Min.X || fills[0].Max.X != panel.Max.X {
		t.Fatalf("the title bar's fill is at %v, want across the top of the panel %v", fills, panel)
	}
	if n := textsIn(ops, panel.Min.Y, panel.Min.Y+bar); n != 1 {
		t.Fatalf("the title bar holds %d texts, want the title", n)
	}
	if err := near(body.at.Y, panel.Min.Y+bar+pad); err != nil {
		t.Fatalf("the body starts at %v, want under the title bar: %v", body.at.Y, err)
	}
}

func TestADialogWithoutATitleHasNoTitleBar(t *testing.T) {
	body, panel, ops := barStage(t, "", func(*Dialog) {})
	if fills := barFills(ops); len(fills) != 0 {
		t.Fatalf("a dialog with no title painted a title bar at %v", fills)
	}
	if err := near(body.at.Y, panel.Min.Y+DialogPadding.Default()); err != nil {
		t.Fatalf("the body starts at %v, want at the top of the panel: %v", body.at.Y, err)
	}
}

func TestADialogsHostCanLeaveTheTitleBarOut(t *testing.T) {
	body, panel, ops := barStage(t, "me@server asks", func(d *Dialog) { d.NoTitleBar, d.Icon = true, icon.LogIn })
	if fills := barFills(ops); len(fills) != 0 {
		t.Fatalf("with NoTitleBar, the dialog painted a title bar at %v", fills)
	}
	if n := textsIn(ops, panel.Min.Y, body.at.Y); n != 0 {
		t.Fatalf("with NoTitleBar, the dialog drew %d texts above its body, want no title", n)
	}
	if len(maskOps(opList(ops))) != 0 {
		t.Fatalf("with NoTitleBar, the dialog drew %d icons", len(maskOps(opList(ops))))
	}
	if err := near(body.at.Y, panel.Min.Y+DialogPadding.Default()); err != nil {
		t.Fatalf("the body starts at %v, want at the top of the panel: %v", body.at.Y, err)
	}
}

// A dialog's title can change after it opens, and the bar shows the new one; set to nothing, the bar goes.
func TestADialogsTitleBarFollowsSetTitle(t *testing.T) {
	d, body, w := barWindow(t, "One", func(*Dialog) {})
	set := func(title string) {
		if err := w.Client().Update("dialog", title); err != nil {
			t.Fatal(err)
		}
		w.Frame(time.Second / 60)
	}
	set("A longer title")
	if d.titleText.s != "A longer title" || len(barFills(w.Offscreen().Ops())) == 0 {
		t.Fatalf("after SetTitle the bar shows %q", d.titleText.s)
	}
	set("")
	panel := d.panel(geom.Sz(800, 600), gunim.Frame{})
	if fills := barFills(w.Offscreen().Ops()); len(fills) != 0 || near(body.at.Y, panel.Min.Y+DialogPadding.Default()) != nil {
		t.Fatalf("with the title gone, the bar is at %v and the body at %v", fills, body.at.Y)
	}
}

// opList holds ops, for helpers that read a window's.
type opList []paint.Op

func (o opList) Ops() []paint.Op { return o }

// near returns an error when a is more than half a pixel from b.
func near(a, b float32) error {
	if d := a - b; d < -0.5 || d > 0.5 {
		return errNear{a, b}
	}
	return nil
}

type errNear struct{ a, b float32 }

func (e errNear) Error() string { return "off by more than half a pixel" }
