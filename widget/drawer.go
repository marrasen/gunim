package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// DrawerWidth is how wide a [Drawer]'s panel is when open.
var DrawerWidth = theme.Length("drawer.width", 280)

// Drawer holds a main child and a panel at its end that slides in and out, such as a list of a conversation's
// members. The main child narrows as the panel comes, so the panel covers nothing, and the panel's content slides
// in with it from the edge and fades in. A closed panel is not drawn and takes no input, and closing it takes the
// keyboard from inside it to the main child.
type Drawer struct {
	anim.Group
	main, panel gunim.Node
	// Width is the open panel's width. NewDrawer sets it to [DrawerWidth].
	Width theme.Token[float32]
	open  *anim.Float
	// laid is set by the first layout. A drawer opened before it shows
	// open at once.
	laid bool
}

// NewDrawer returns main with panel closed beside it.
func NewDrawer(main, panel gunim.Node) *Drawer {
	d := &Drawer{main: main, panel: panel, open: anim.NewFloat(0), Width: DrawerWidth}
	d.Add(d.open)
	return d
}

// SetOpen slides the panel in, or out.
func (d *Drawer) SetOpen(on bool, u *gunim.UI) {
	if on == d.Open() {
		return
	}
	if on {
		u.Cue(gunim.CueOpen, d.panel)
	} else {
		u.Cue(gunim.CueClose, d.panel)
	}
	// No overshoot: the main child reflows as the panel slides, and should do so once each way.
	if to := map[bool]float32{false: 0, true: 1}[on]; d.laid {
		d.open.Animate(to, Settle.Get(u.Theme()))
	} else {
		d.open.Jump(to)
	}
	if !on && u.HasFocus(d.panel) {
		u.FocusFirst(d.main)
	}
	u.Invalidate()
}

// Open reports whether the panel is open, or opening.
func (d *Drawer) Open() bool { return d.open.Target() > 0.5 }

// Children implements [gunim.Composite].
func (d *Drawer) Children() []gunim.Node { return []gunim.Node{d.main, d.panel} }

// shown returns how much of the panel shows, from 0 to 1.
func (d *Drawer) shown() float32 { return min(max(d.open.Value(), 0), 1) }

// Layout implements [gunim.Node]: the main child in what the panel leaves, and the panel at its full width, only
// the part in view showing.
func (d *Drawer) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	d.laid = true
	w := min(d.Width.Get(f.Theme), c.Max.W)
	in := w * d.shown()
	main, panel := kids.At(0), kids.At(1)
	main.Layout(gunim.Tight(geom.Sz(max(c.Max.W-in, 0), c.Max.H)))
	main.Place(geom.Point{})
	panel.Layout(gunim.Tight(geom.Sz(w, c.Max.H)))
	panel.Place(geom.Pt(c.Max.W-in, 0))
	return c.Max
}

// Paint implements [gunim.Node].
func (d *Drawer) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	t := d.shown()
	if t <= 0.01 {
		return
	}
	w := min(d.Width.Get(f.Theme), box.W)
	edge := max(box.W-w*t, 0)
	area := geom.Rect{Min: geom.Pt(edge, 0), Max: box.Point()}
	defer p.Layer(paint.LayerOpts{Bounds: area, Opacity: t, Clip: true})()
	kids.At(1).Paint(p)
	// The line along the panel's edge, over it.
	p.RRect(geom.Rc(edge, 0, 1, box.H), 0, paint.Solid(MenuBorder.Get(f.Theme)))
}
