package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
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
	// Icon shows before the title, in the ink, or in [DialogDangerInk] for a danger dialog. A danger dialog
	// without one shows icon.TriangleAlert.
	Icon *icon.Icon
	// Accept is the intent sent when the user confirms, and Dismiss the
	// one sent when they back out. Both travel as data.
	//
	// The dialog closes itself either way, on the frame the button is
	// released. Waiting for the application to say so would put a round
	// trip between the click and the animation, which is the thin
	// client feeling this design exists to avoid.
	Accept  gunim.Intent
	Dismiss gunim.Intent
	// OnAccept, when set, makes the intent sent on confirming from what
	// the dialog holds, such as a form's fields, in place of Accept.
	OnAccept func() gunim.Intent
	// Danger marks the dialog's OK as an action that destroys, such as
	// Delete, and shows it in red; otherwise it shows as the action the
	// dialog expects. A danger dialog opens with Cancel focused, so
	// Enter cancels and the red button takes a deliberate press.
	Danger bool
	// Careful opens the dialog with Cancel focused, as Danger does, and
	// leaves OK its usual colour: for a question where yes by reflex is
	// the answer that cannot be taken back, such as trusting a server's
	// new key.
	Careful bool
	// Check, when set, runs as the user confirms, and says what stands
	// in the way, or nothing. With something in the way the dialog stays
	// open, gives a shake, and says it under the body.
	Check func() string
	// Body is what the dialog shows under its title, such as a [Form].
	// Set it before mounting the dialog, which grows to fit it. Enter
	// in the body confirms, and Tab moves through the body's fields and
	// the buttons.
	Body gunim.Node

	// in runs from 0 (gone) to 1 (fully present) and drives every visual
	// property. One value for the whole transition keeps the fade, the
	// scale and the backdrop blur locked together through any retuning
	// of the motion.
	in *anim.Float

	ok        *Button
	cancel    *Button
	extra     []*Button
	titleText laidText
	// width is the panel's width at the last layout.
	width float32
	// height is the panel's height, from the last layout, and focused
	// is set once the keyboard has gone to the body's first field.
	height  float32
	focused bool
	// problem is what Check last said, and shake the shake it set off.
	problem *Label
	shake   *anim.Float
}

// NewDialog returns a dialog with an OK and a Cancel button. Mount it
// from a view and it animates itself in.
func NewDialog(title string) *Dialog {
	d := &Dialog{Title: title, in: anim.NewFloat(0), shake: anim.NewFloat(0)}
	d.problem = NewLabel("")
	d.problem.Color = DialogProblem
	d.Add(d.in, d.shake)

	d.ok = NewButton("OK")
	d.ok.OnActivate(d.accept)
	d.cancel = NewButton("Cancel")
	d.cancel.OnActivate(func(u *gunim.UI) { d.finish(u, d.Dismiss) })
	return d
}

// SetTitle changes the title. Call it from a view's update function.
func (d *Dialog) SetTitle(title string) { d.Title = title }

// SetButtons names the dialog's buttons, such as "Connect" and
// "Cancel". An empty cancel leaves the dialog with OK alone, for one
// that only tells; Escape still closes it.
func (d *Dialog) SetButtons(ok, cancel string) {
	d.ok.Label, d.cancel.Label = ok, cancel
}

// AddButton adds a button left of Cancel, which closes the dialog and
// sends the intent what makes, as a third answer to a question: Leave
// It beside Replace and Stop. Add buttons before mounting the dialog.
func (d *Dialog) AddButton(label string, what func() gunim.Intent) {
	b := NewButton(label)
	b.OnActivate(func(u *gunim.UI) { d.finish(u, what()) })
	d.extra = append(d.extra, b)
}

// AddAction adds a button left of Cancel that runs do and leaves the
// dialog open, for help with the form, such as generating a password.
// Add buttons before mounting the dialog.
func (d *Dialog) AddAction(label string, do func(u *gunim.UI)) {
	b := NewButton(label)
	b.OnActivate(do)
	d.extra = append(d.extra, b)
}

// Close closes the dialog and sends nothing, for an action that leads
// on to another dialog, such as a Remove button in a form that asks
// first.
func (d *Dialog) Close(u *gunim.UI) { d.finish(u, nil) }

func (d *Dialog) accept(u *gunim.UI) {
	if d.Check != nil {
		if msg := d.Check(); msg != "" {
			d.problem.SetText(msg)
			// A kick sideways that a springy motion rings down to rest.
			d.shake.Jump(1)
			d.shake.Animate(0, anim.Spring{Response: 0.18, Damping: 0.12})
			u.Invalidate()
			return
		}
	}
	d.problem.SetText("")
	what := d.Accept
	if d.OnAccept != nil {
		what = d.OnAccept()
	}
	d.finish(u, what)
}

// focusables returns what Tab moves through: the body's fields, then
// the buttons.
func (d *Dialog) focusables() []gunim.Node {
	var out []gunim.Node
	if b, ok := d.Body.(interface{ Focusables() []gunim.Node }); ok {
		out = append(out, b.Focusables()...)
	}
	for _, b := range d.extra {
		out = append(out, b)
	}
	return append(out, d.buttons()...)
}

// buttons are Cancel, when it has a label, and OK.
func (d *Dialog) buttons() []gunim.Node {
	if d.cancel.Label == "" {
		return []gunim.Node{d.ok}
	}
	return []gunim.Node{d.cancel, d.ok}
}

// Children implements [gunim.Composite], so mounting the dialog brings
// its buttons with it.
//
// Layout places them from the right, so the last one, OK, lands
// outermost.
func (d *Dialog) Children() []gunim.Node {
	var out []gunim.Node
	if d.Body != nil {
		out = append(out, d.Body, d.problem)
	}
	for _, b := range d.extra {
		out = append(out, b)
	}
	return append(out, d.buttons()...)
}

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
func (d *Dialog) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok {
		switch k.Key {
		case input.KeyEscape:
			d.finish(u, d.Dismiss)
			return true
		case input.KeyEnter, input.KeyKPEnter:
			d.accept(u)
			return true
		case input.KeyTab:
			// A modal keeps focus among its own buttons.
			d.cycle(u, !k.Mods.Has(input.ModShift))
			return true
		default:
		}
	}
	// Given the keyboard as it opens, the dialog hands it to its first
	// field, or to Cancel when it is a danger dialog.
	if _, ok := e.(input.FocusGained); ok && !d.focused {
		d.focused = true
		if d.Danger || d.Careful {
			u.Focus(d.buttons()[0])
			return true
		}
		if b, ok := d.Body.(interface{ Focusables() []gunim.Node }); ok {
			if f := b.Focusables(); len(f) > 0 {
				u.Focus(f[0])
			}
		}
		return true
	}
	// A modal swallows the pointer events that reach it, keeping clicks
	// off whatever lies behind, and keeps the window's menus shut.
	switch e := e.(type) {
	case input.PointerDown, input.PointerUp, input.Scroll, input.AltTapped:
		return true
	case input.KeyPress:
		return e.Key == input.KeyF10 || e.Mods.Has(input.ModAlt)
	}
	return false
}

// Focusable implements [gunim.Focusable]. A click on the dialog's
// empty space focuses the dialog itself, so Escape and Tab still reach
// it.
func (d *Dialog) Focusable() bool { return true }

// cycle moves focus to the next of the dialog's buttons, or the
// previous, wrapping around.
func (d *Dialog) cycle(u *gunim.UI, forward bool) {
	kids := d.focusables()
	at := -1
	for i, k := range kids {
		if k == u.Focused() {
			at = i
		}
	}
	next := 0
	switch {
	case at < 0 && !forward:
		next = len(kids) - 1
	case at >= 0 && forward:
		next = (at + 1) % len(kids)
	case at >= 0:
		next = (at - 1 + len(kids)) % len(kids)
	}
	u.Focus(kids[next])
}

// Layout implements [gunim.Node]. The dialog fills the space it is
// given and centres a panel inside it, so the scrim covers the window
// while the panel keeps its own size.
func (d *Dialog) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	th := f.Theme
	pad := DialogPadding.Get(th)
	d.ok.Kind = ButtonPrimary
	if d.Danger {
		d.ok.Kind = ButtonDanger
	}
	// As wide as the theme says, or as the buttons need in a row,
	// whichever is wider, and no wider than the window.
	first := 0
	if d.Body != nil {
		first = 2
	}
	gap := DialogGap.Get(th)
	row := -gap
	for i := first; i < kids.Len(); i++ {
		row += kids.At(i).Layout(gunim.Loose(size)).W + gap
	}
	width := min(max(DialogWidth.Get(th), row+2*pad), size.W)
	d.width = width
	// With a body, the panel grows to fit the title, the body and the
	// buttons.
	d.height = DialogHeight.Get(th)
	var body, problem gunim.Child
	hasBody := d.Body != nil
	var bs, ps geom.Size
	if hasBody {
		body, problem = kids.At(0), kids.At(1)
		title := d.title(th, width)
		bs = body.Layout(gunim.Constraints{Max: geom.Sz(width-2*pad, 0)})
		ps = problem.Layout(gunim.Constraints{Max: geom.Sz(width-2*pad, 0)})
		extra := float32(0)
		if d.problem.Text != "" {
			extra = pad/2 + ps.H
		}
		d.height = pad + title.Size.H + pad + bs.H + extra + pad + ButtonHeight.Get(th) + pad
	}
	panel := d.panel(size, f)
	if hasBody {
		title := d.title(th, width)
		at := panel.Min.Add(geom.Pt(pad, pad+title.Size.H+pad))
		body.Place(at)
		problem.Place(at.Add(geom.Pt(0, bs.H+pad/2)))
	}

	// Buttons sit along the bottom right of the panel, laid out from the
	// right so the primary action lands outermost.
	x := panel.Max.X - pad
	y := panel.Max.Y - pad
	for i := kids.Len() - 1; i >= first; i-- {
		kid := kids.At(i)
		s := kid.Layout(gunim.Loose(panel.Size()))
		x -= s.W
		kid.Place(geom.Pt(x, y-s.H))
		x -= DialogGap.Get(f.Theme)
	}
	return size
}

// panel returns the dialog's own rectangle, centred in size.
func (d *Dialog) panel(size geom.Size, f gunim.Frame) geom.Rect {
	w, h := d.width, d.height
	if w <= 0 {
		w = DialogWidth.Get(f.Theme)
	}
	if h <= 0 {
		h = DialogHeight.Get(f.Theme)
	}
	return geom.Rc((size.W-w)/2, (size.H-h)/2, w, h)
}

// Transition implements [gunim.Transitioner]. The dialog springs in
// with a slight overshoot, and settles out without one.
func (d *Dialog) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		d.in.Animate(1, Bounce.Get(f.Theme))
	case gunim.Exiting:
		d.in.Animate(0, Settle.Get(f.Theme))
	case gunim.Present:
		// Settled in.
	}
	return !d.in.Active()
}

// Paint implements [gunim.Node].
func (d *Dialog) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	t := d.in.Value()
	if t <= 0 {
		return
	}
	// The spring overshoots on the way in. The scale takes the
	// overshoot, and everything that fades stops at fully opaque.
	fade := min(t, 1)

	// The scrim darkens whatever is behind the dialog and blurs it. The
	// blur radius is tied to the same value as the fade, so the
	// background comes back into focus as the dialog leaves.
	dim := Scrim.Get(th)
	dim.A = uint8(float32(dim.A) * fade)
	full := geom.Rect{Max: box.Point()}
	closeScrim := p.Layer(paint.LayerOpts{Bounds: full, Opacity: fade, Backdrop: DialogBackdrop.Get(th) * fade})
	p.RRect(full, 0, paint.Solid(dim))
	closeScrim()

	panel := d.panel(box, f)
	radius, pad := DialogRadius.Get(th), DialogPadding.Get(th)
	// A shake swings the panel from side to side.
	defer p.Push(paint.Translate(geom.Pt(14*d.shake.Value(), 0)))()

	// The panel fades and grows into place together. Starting at 0.94
	// makes it read as arriving; starting nearer 0 would make it read
	// as being inflated.
	defer p.Layer(paint.LayerOpts{Bounds: panel, Opacity: fade})()
	defer p.Push(paint.Scale(0.94+0.06*t, panel.Center()))()

	shadow := DialogShadow.Get(th)
	shadow.A = uint8(float32(shadow.A) * fade)
	p.ShadowRRect(panel, radius, paint.Solid(DialogFill.Get(th)), paint.Shadow{
		Offset: geom.Pt(0, 8*fade),
		Blur:   32 * fade,
		Color:  shadow,
	})
	p.RRectStroke(panel, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: DialogBorder.Get(th)})
	title := d.title(th, panel.Size().W)
	if ic, ink := d.mark(); ic != nil {
		s := IconSize.Get(th)
		paintIcon(p, th, ic, geom.Rc(panel.Min.X+pad, panel.Min.Y+pad+(title.LineHeight-s)/2, s, s), ink.Get(th), 1)
	}
	title.Paint(p, panel.Min.Add(geom.Pt(pad+d.iconRoom(th), pad)), Ink.Get(th))

	for kid := range kids.All {
		kid.Paint(p)
	}
}

// mark returns the icon before the title and its colour, or nil.
func (d *Dialog) mark() (*icon.Icon, theme.Token[color.NRGBA]) {
	ic, ink := d.Icon, Ink
	if d.Danger {
		ink = DialogDangerInk
		if ic == nil {
			ic = icon.TriangleAlert
		}
	}
	return ic, ink
}

// iconRoom is the room the icon takes before the title, with its gap.
func (d *Dialog) iconRoom(th *theme.Live) float32 {
	if ic, _ := d.mark(); ic == nil {
		return 0
	}
	return IconSize.Get(th) + IconGap.Get(th)
}

// title lays the title out for a panel width wide, after the icon.
func (d *Dialog) title(th *theme.Live, width float32) text.Paragraph {
	style := text.Style{Size: DialogTitleSize.Get(th), MaxLines: 2}
	return d.titleText.layout(faceIn(Font, th), d.Title, style, width-2*DialogPadding.Get(th)-d.iconRoom(th))
}
