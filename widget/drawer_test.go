package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// measured records the box it was last painted in.
type measured struct{ box geom.Size }

func (m *measured) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}
func (m *measured) Paint(_ *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	m.box = box
}

func TestADrawerSlidesItsPanelInBesideTheMain(t *testing.T) {
	main, panel := &measured{}, newSpot(10, 10)
	d := NewDrawer(main, panel)
	_, run := stage(t, &frame{child: d, size: geom.Sz(800, 400)})
	run(2)
	if main.box.W != 800 || panel.at != (geom.Point{}) {
		t.Fatalf("closed, the main is %v wide and the panel drawn at %v; want the main whole and the panel not drawn",
			main.box.W, panel.at)
	}
	d.open.Animate(1, Settle.Default())
	run(120)
	want := 800 - DrawerWidth.Default()
	if main.box.W != want || panel.at.X != want {
		t.Fatalf("open, the main is %v wide and the panel at %v; want %v and beside it", main.box.W, panel.at.X, want)
	}
}

func TestClosingADrawerTakesTheKeyboardOutOfItsPanel(t *testing.T) {
	field := NewTextField()
	var d *Drawer
	closer := NewButton("Close")
	closer.OnClick = func(u *gunim.UI) gunim.Intent { d.SetOpen(false, u); return nil }
	d = NewDrawer(field, closer)
	w, run := stage(t, &frame{child: d, size: geom.Sz(800, 400)})
	d.open.Jump(1)
	run(5)
	// Press the panel's button, which takes the keyboard, and closes the panel.
	at := geom.Pt(800-DrawerWidth.Default()+20, 15)
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(60)
	if d.Open() {
		t.Fatal("the panel did not close")
	}
	w.Input(input.TextInput{Text: "hi"})
	run(1)
	if field.Text() != "hi" {
		t.Fatalf("after the panel closed, typing gave the field %q, want it", field.Text())
	}
}
