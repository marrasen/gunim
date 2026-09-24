package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Dialog is a modal panel that fades, scales and blurs its way in and
// out.
//
// It is the worked example for the whole design. The application says
// "remove this dialog" and then stops thinking about it. The dialog
// stays in the tree, watches its own [gunim.Presence], animates itself
// away, and tells the engine when it is finished. How long that takes
// and what it looks like stay inside the dialog, which is what makes
// the animation easy to change later.
type Dialog struct {
	anim.Group

	Title string
	// Accept is the intent sent when the user confirms, and Dismiss the
	// one sent when they back out. Both travel as data.
	//
	// The dialog closes itself either way, on the frame the button is
	// released. Waiting for the application to say so would put a round
	// trip between the click and the animation, which is the thin
	// client feeling this design exists to avoid.
	Accept  gunim.Intent
	Dismiss gunim.Intent

	// in runs from 0 (gone) to 1 (fully present) and drives every visual
	// property. One value for the whole transition keeps the fade, the
	// scale and the backdrop blur locked together through any retuning
	// of the motion.
	in *anim.Float

	ok     *Button
	cancel *Button
}

// NewDialog returns a dialog with an OK and a Cancel button. Mount it
// from a view and it animates itself in.
func NewDialog(title string) *Dialog {
	d := &Dialog{Title: title, in: anim.NewFloat(0)}
	d.Add(d.in)

	d.ok = NewButton("OK")
	d.ok.OnActivate(func(u *gunim.UI) { d.finish(u, d.Accept) })
	d.cancel = NewButton("Cancel")
	d.cancel.OnActivate(func(u *gunim.UI) { d.finish(u, d.Dismiss) })
	return d
}

// SetTitle changes the title. Call it from a view's update function.
func (d *Dialog) SetTitle(title string) { d.Title = title }

// Children implements [gunim.Composite], so mounting the dialog brings
// its buttons with it.
func (d *Dialog) Children() []gunim.Node { return []gunim.Node{d.ok, d.cancel} }

// finish closes the dialog and tells the application what happened.
//
// The close lands first and locally, so the fade starts on this frame.
// The intent goes out behind it, and the application reads it whenever
// it is ready.
func (d *Dialog) finish(u *gunim.UI, what gunim.Intent) {
	u.Remove(d)
	if what != nil {
		u.Send(d, what)
	}
}

// Handle implements [gunim.Handler]. Escape dismisses.
func (d *Dialog) Handle(e gunim.Event, u *gunim.UI) bool {
	if k, ok := e.(gunim.KeyPress); ok && k.Key == gunim.KeyEscape {
		d.finish(u, d.Dismiss)
		return true
	}
	// A modal swallows the pointer events that reach it, keeping clicks
	// off whatever lies behind.
	switch e.(type) {
	case gunim.PointerDown, gunim.PointerUp, gunim.Scroll:
		return true
	}
	return false
}

var (
	dialogFill   = color.NRGBA{R: 0x1d, G: 0x20, B: 0x28, A: 0xff}
	dialogBorder = color.NRGBA{R: 0x3a, G: 0x40, B: 0x50, A: 0xff}
	dialogText   = color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff}
	scrim        = color.NRGBA{A: 0x99}
)

const dialogPad = 20

// Layout implements [gunim.Node]. The dialog fills the space it is
// given and centres a fixed-size panel inside it, so the scrim covers
// the window while the panel keeps its own size.
func (d *Dialog) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	panel := d.panel(size)

	// Buttons sit along the bottom right of the panel, laid out from the
	// right so the primary action lands outermost.
	x := panel.Max.X - dialogPad
	y := panel.Max.Y - dialogPad
	for i := kids.Len() - 1; i >= 0; i-- {
		kid := kids.At(i)
		s := kid.Layout(gunim.Loose(panel.Size()))
		x -= s.W
		kid.Place(geom.Pt(x, y-s.H))
		x -= 10
	}
	return size
}

// panel returns the dialog's own rectangle, centred in size.
func (d *Dialog) panel(size geom.Size) geom.Rect {
	const w, h = 420, 200
	return geom.Rc((size.W-w)/2, (size.H-h)/2, w, h)
}

// Paint implements [gunim.Node].
func (d *Dialog) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	t := d.in.Value()
	if t <= 0 {
		return
	}

	// The scrim darkens whatever is behind the dialog and blurs it. The
	// blur radius is tied to the same value as the fade, so the
	// background comes back into focus as the dialog leaves.
	dim := scrim
	dim.A = uint8(float32(scrim.A) * t)
	full := geom.Rect{Max: box.Point()}
	closeScrim := p.Layer(paint.LayerOpts{Bounds: full, Opacity: t, Backdrop: 14 * t})
	p.RRect(full, 0, paint.Solid(dim))
	closeScrim()

	panel := d.panel(box)

	// The panel fades and grows into place together. Starting at 0.94
	// makes it read as arriving; starting nearer 0 would make it read
	// as being inflated.
	defer p.Layer(paint.LayerOpts{Bounds: panel, Opacity: t})()
	defer p.Push(paint.Scale(0.94+0.06*t, panel.Center()))()

	p.ShadowRRect(panel, 14, paint.Solid(dialogFill), paint.Shadow{
		Offset: geom.Pt(0, 8*t),
		Blur:   32 * t,
		Color:  color.NRGBA{A: uint8(0x80 * t)},
	})
	p.RRectStroke(panel, 14, paint.Fill{}, paint.Stroke{Width: 1, Color: dialogBorder})
	p.Text(nil, dialogText, panel)

	for kid := range kids.All {
		kid.Paint(p)
	}
}
