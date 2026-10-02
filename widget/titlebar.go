package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

func init() {
	gunim.RegisterTitleBar(func() gunim.TitleBar { return NewTitleBar("") })
}

// TitleBar is the title bar the engine gives a chromeless window: the title, centred, and the window's buttons at its
// end. An application that puts [WindowControls] in its own tree draws its own title bar instead.
//
// NoMinimize, NoMaximize and NoClose leave buttons out, as [WindowControls] has them. With none left, the bar is the
// title alone: a press on it still moves the window, and a double click no longer maximizes it.
type TitleBar struct {
	// Compact makes the bar [TitleBarCompactHeight] tall, with narrower buttons.
	Compact bool
	// Pin adds a button that keeps the window above other windows; see [WindowControls.Pin].
	Pin bool
	// NoMinimize, NoMaximize and NoClose leave out the minimize, maximize and close buttons.
	NoMinimize, NoMaximize, NoClose bool

	title    *WindowTitle
	controls *WindowControls
}

// NewTitleBar returns a title bar showing title.
func NewTitleBar(title string) *TitleBar {
	return &TitleBar{title: NewWindowTitle(title), controls: NewWindowControls()}
}

// SetTitle implements [gunim.TitleBar].
func (t *TitleBar) SetTitle(title string) { t.title.SetText(title) }

// Children implements [gunim.Composite].
func (t *TitleBar) Children() []gunim.Node { return []gunim.Node{t.title, t.controls} }

// Layout implements [gunim.Node]: the buttons at the end, and the title in the rest of the row.
func (t *TitleBar) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	if !f.Chromeless() {
		for k := range kids.All {
			k.Layout(gunim.Tight(geom.Size{}))
		}
		return cs.Constrain(geom.Size{})
	}
	t.controls.Compact, t.controls.Pin = t.Compact, t.Pin
	t.controls.NoMinimize, t.controls.NoMaximize, t.controls.NoClose = t.NoMinimize, t.NoMaximize, t.NoClose
	h := t.controls.height(f.Theme)
	w := cs.Max.W
	ctl := kids.At(1).Layout(gunim.Constraints{Max: geom.Sz(w, h)})
	kids.At(1).Place(geom.Pt(w-ctl.W, 0))
	kids.At(0).Layout(gunim.Tight(geom.Sz(max(0, w-ctl.W), h)))
	kids.At(0).Place(geom.Point{})
	return cs.Constrain(geom.Sz(w, h))
}

// Paint implements [gunim.Node].
func (t *TitleBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(MenubarFill.Get(f.Theme)))
	for k := range kids.All {
		k.Paint(p)
	}
}
