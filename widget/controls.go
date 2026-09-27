package widget

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// toggle is the behaviour a checkbox and a switch share: hover, press,
// focus, and flipping on a click or Space.
type toggle struct {
	anim.Group
	Label string
	On    bool
	// Disabled shows the control faint, and it takes no clicks, keys or
	// focus, for a choice that does not apply now.
	Disabled bool
	// OnChange turns the new state into an intent for the application.
	OnChange func(on bool) gunim.Intent
	// flipped is local behaviour, set by OnFlip.
	flipped func(on bool, u *gunim.UI)

	// lit runs from 0 to 1 as the control turns on.
	lit   *anim.Float
	hover *anim.Float
	press *anim.Float
	ring  *anim.Float
	held  bool
	size  geom.Size
	text  shapedText
	// laid is set by the first layout, which puts the control where On
	// says without animating.
	laid bool
}

func newToggle(label string) toggle {
	t := toggle{
		Label: label,
		lit:   anim.NewFloat(0),
		hover: anim.NewFloat(0),
		press: anim.NewFloat(0),
		ring:  anim.NewFloat(0),
	}
	t.Add(t.lit, t.hover, t.press, t.ring)
	return t
}

// SetOn sets the state without an intent. Call it from a view's update
// function; the control animates to it.
func (t *toggle) SetOn(on bool, u *gunim.UI) {
	if on == t.On {
		return
	}
	t.On = on
	t.lit.Animate(value(on), Bounce.Get(u.Theme()))
}

// OnFlip wires behaviour that runs inside the window when the user
// flips the control, such as a box that shows a password field's text.
func (t *toggle) OnFlip(fn func(on bool, u *gunim.UI)) { t.flipped = fn }

func (t *toggle) flip(n gunim.Node, u *gunim.UI) {
	t.On = !t.On
	t.lit.Animate(value(t.On), Bounce.Get(u.Theme()))
	if t.flipped != nil {
		t.flipped(t.On, u)
	}
	if t.OnChange != nil {
		u.Send(n, t.OnChange(t.On))
	}
}

// Focusable implements [gunim.Focusable].
func (t *toggle) Focusable() bool { return !t.Disabled }

// handle is the Handle both controls share; n is the control itself.
func (t *toggle) handle(n gunim.Node, e input.Event, u *gunim.UI) bool {
	if t.Disabled {
		return false
	}
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerEnter:
		t.hover.Animate(1, Quick.Get(th))
		if t.held {
			t.press.Animate(1, Quick.Get(th))
		}
	case input.PointerLeave:
		t.hover.Animate(0, Settle.Get(th))
		t.press.Animate(0, Bounce.Get(th))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		t.held = true
		t.press.Animate(1, Quick.Get(th))
	case input.PointerUp:
		if !t.held {
			return false
		}
		t.held = false
		t.press.Animate(0, Bounce.Get(th))
		if (geom.Rect{Max: t.size.Point()}).Contains(e.Pos) {
			t.flip(n, u)
		}
	case input.KeyPress:
		if e.Key != input.KeySpace {
			return false
		}
		t.press.Retarget(1, Quick.Get(th))
		t.press.Animate(0, Bounce.Get(th))
		t.flip(n, u)
	case input.FocusGained:
		t.ring.Animate(1, Quick.Get(th))
	case input.FocusLost:
		t.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	return true
}

// layout sizes the control: its mark, a gap, and its label.
func (t *toggle) layout(c gunim.Constraints, f gunim.Frame, mark geom.Size) geom.Size {
	if !t.laid {
		t.laid = true
		t.lit.Jump(value(t.On))
	}
	w, h := mark.W, mark.H
	if t.Label != "" {
		run := t.text.shape(faceIn(Font, f.Theme), t.Label, TextSize.Get(f.Theme))
		w += ControlGap.Get(f.Theme) + run.Advance
		h = max(h, run.Height())
	}
	t.size = c.Constrain(geom.Sz(w, max(h, ControlHeight.Get(f.Theme))))
	return t.size
}

// paintLabel draws the label after the control's mark, which is mark
// wide.
func (t *toggle) paintLabel(p *paint.Painter, f gunim.Frame, box geom.Size, mark float32) {
	if t.Label == "" {
		return
	}
	run := t.text.shape(faceIn(Font, f.Theme), t.Label, TextSize.Get(f.Theme))
	run.Paint(p, geom.Pt(mark+ControlGap.Get(f.Theme), (box.H-run.Height())/2), Ink.Get(f.Theme))
}

// Checkbox is a box that is ticked or not, with a label.
//
// The tick draws itself in, short stroke then long, as the box fills
// with the accent colour; hover warms the border, a press squashes the
// box, and focus grows a ring around it. A click anywhere on the
// checkbox or its label flips it, and so does Space.
type Checkbox struct{ toggle }

// NewCheckbox returns an unticked checkbox.
func NewCheckbox(label string) *Checkbox { return &Checkbox{newToggle(label)} }

// Handle implements [gunim.Handler].
func (c *Checkbox) Handle(e input.Event, u *gunim.UI) bool { return c.handle(c, e, u) }

// Layout implements [gunim.Node].
func (c *Checkbox) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	s := CheckSize.Get(f.Theme)
	return c.layout(cs, f, geom.Sz(s, s))
}

// Paint implements [gunim.Node].
func (c *Checkbox) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer faintIf(p, box, c.Disabled)()
	th := f.Theme
	s := CheckSize.Get(th)
	r := geom.Rc(0, (box.H-s)/2, s, s)
	radius := CheckRadius.Get(th)
	on := c.lit.Value()

	func() {
		defer p.Push(paint.Scale(1-0.1*c.press.Value(), r.Center()))()
		focusRing(p, r, radius, c.ring.Value(), th)
		border := anim.Mix(anim.ColorCodec, FieldBorder.Get(th), Ink.Get(th), 0.35*c.hover.Value())
		p.RRectStroke(r, radius, paint.Solid(FieldFill.Get(th)), paint.Stroke{Width: 1.5, Color: border})
		// The fill grows from the middle as the box turns on.
		if on > 0.01 {
			grow := min(on, 1)
			fill := Accent.Get(th)
			inner := geom.Rect{
				Min: r.Center().Sub(geom.Pt(s/2*grow, s/2*grow)),
				Max: r.Center().Add(geom.Pt(s/2*grow, s/2*grow)),
			}
			p.RRect(inner, radius*grow, paint.Solid(fill))
		}
		// The tick: a short stroke down, then a long one up.
		k := s / 18
		a := r.Min.Add(geom.Pt(4.5*k, 9.5*k))
		b := r.Min.Add(geom.Pt(7.5*k, 12.5*k))
		d := r.Min.Add(geom.Pt(13.5*k, 5.5*k))
		mark := CheckMark.Get(th)
		t := min(max(on, 0), 1)
		if short := min(t/0.35, 1); short > 0 {
			bar(p, a, lerpPt(a, b, short), 2*k, mark)
		}
		if long := (t - 0.35) / 0.65; long > 0 {
			bar(p, b, lerpPt(b, d, long), 2*k, mark)
		}
	}()
	c.paintLabel(p, f, box, s)
}

// Switch is a knob in a track that slides between off and on, with a
// label.
//
// The knob springs across and the track fills with the accent colour;
// pressing stretches the knob, the way a finger flattens it. A click or
// Space flips it.
type Switch struct{ toggle }

// NewSwitch returns a switch that is off.
func NewSwitch(label string) *Switch { return &Switch{newToggle(label)} }

// Handle implements [gunim.Handler].
func (s *Switch) Handle(e input.Event, u *gunim.UI) bool { return s.handle(s, e, u) }

// Layout implements [gunim.Node].
func (s *Switch) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return s.layout(cs, f, geom.Sz(SwitchWidth.Get(f.Theme), SwitchHeight.Get(f.Theme)))
}

// Paint implements [gunim.Node].
func (s *Switch) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer faintIf(p, box, s.Disabled)()
	th := f.Theme
	w, h := SwitchWidth.Get(th), SwitchHeight.Get(th)
	track := geom.Rc(0, (box.H-h)/2, w, h)
	on := s.lit.Value()
	focusRing(p, track, h/2, s.ring.Value(), th)
	off := anim.Mix(anim.ColorCodec, SwitchOff.Get(th), Ink.Get(th), 0.15*s.hover.Value())
	p.RRect(track, h/2, paint.Solid(anim.Mix(anim.ColorCodec, off, Accent.Get(th), min(max(on, 0), 1))))

	// The knob, inset from the track, widens under a press toward the
	// side it is heading for.
	inset := float32(3)
	d := h - 2*inset
	stretch := d * 0.3 * s.press.Value()
	x := track.Min.X + inset + (w-2*inset-d-stretch)*on
	knob := geom.Rc(x, track.Min.Y+inset, d+stretch, d)
	p.ShadowRRect(knob, d/2, paint.Solid(Knob.Get(th)), paint.Shadow{Offset: geom.Pt(0, 1), Blur: 2, Color: color.NRGBA{A: 0x50}})
	s.paintLabel(p, f, box, w)
}

// Slider picks a number between Min and Max by dragging a knob along a
// track, or with the arrow keys, Page Up and Page Down, Home and End.
//
// A click on the track sends the knob gliding there; a drag carries it
// along with the pointer. The knob grows while the pointer is over it
// or it is held.
type Slider struct {
	anim.Group
	Min, Max float32
	// Snap rounds the value to multiples of itself, counted from Min;
	// zero leaves it free.
	Snap float32
	// OnChange turns a new value into an intent for the application.
	OnChange func(v float32) gunim.Intent

	value float32
	// at is the knob's place, 0 to 1 along the track.
	at    *anim.Float
	hover *anim.Float
	ring  *anim.Float
	held  bool
	size  geom.Size
}

// NewSlider returns a slider from lo to hi, at lo.
func NewSlider(lo, hi float32) *Slider {
	s := &Slider{Min: lo, Max: hi, value: lo, at: anim.NewFloat(0), hover: anim.NewFloat(0), ring: anim.NewFloat(0)}
	s.Add(s.at, s.hover, s.ring)
	return s
}

// Value returns the slider's value.
func (s *Slider) Value() float32 { return s.value }

// SetValue sets the value without an intent. Call it from a view's
// update function; the knob glides to it.
func (s *Slider) SetValue(v float32, u *gunim.UI) {
	s.value = s.clamp(v)
	s.at.Animate(s.frac(), Quick.Get(u.Theme()))
}

func (s *Slider) clamp(v float32) float32 {
	if s.Snap > 0 {
		v = s.Min + float32(math.Round(float64((v-s.Min)/s.Snap)))*s.Snap
	}
	return max(s.Min, min(v, s.Max))
}

// frac returns the value as a fraction of the range.
func (s *Slider) frac() float32 {
	if s.Max <= s.Min {
		return 0
	}
	return (s.value - s.Min) / (s.Max - s.Min)
}

// set moves to v, the knob following with m, and tells the
// application when the value changed.
func (s *Slider) set(v float32, m anim.Motion, u *gunim.UI) {
	v = s.clamp(v)
	if v == s.value {
		return
	}
	s.value = v
	s.at.Animate(s.frac(), m)
	if s.OnChange != nil {
		u.Send(s, s.OnChange(v))
	}
}

// Focusable implements [gunim.Focusable].
func (s *Slider) Focusable() bool { return true }

// valueAt returns the value at x in the slider's space.
func (s *Slider) valueAt(x float32, th *theme.Live) float32 {
	k := KnobSize.Get(th) / 2
	w := max(s.size.W-2*k, 1)
	return s.Min + max(0, min((x-k)/w, 1))*(s.Max-s.Min)
}

// Handle implements [gunim.Handler].
func (s *Slider) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	step := s.Snap
	if step <= 0 {
		step = (s.Max - s.Min) / 100
	}
	switch e := e.(type) {
	case input.PointerEnter:
		s.hover.Animate(1, Quick.Get(th))
	case input.PointerLeave:
		if !s.held {
			s.hover.Animate(0, Settle.Get(th))
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		s.held = true
		s.set(s.valueAt(e.Pos.X, th), Quick.Get(th), u)
	case input.PointerMove:
		if !s.held {
			return false
		}
		// The knob keeps up with the pointer while it glides in from a
		// click, then follows it exactly.
		s.set(s.valueAt(e.Pos.X, th), Caret.Get(th), u)
	case input.PointerUp:
		if !s.held {
			return false
		}
		s.held = false
		if !(geom.Rect{Max: s.size.Point()}).Contains(e.Pos) {
			s.hover.Animate(0, Settle.Get(th))
		}
	case input.KeyPress:
		switch e.Key {
		case input.KeyLeft, input.KeyDown:
			s.set(s.value-step, Quick.Get(th), u)
		case input.KeyRight, input.KeyUp:
			s.set(s.value+step, Quick.Get(th), u)
		case input.KeyPageDown:
			s.set(s.value-(s.Max-s.Min)/10, Quick.Get(th), u)
		case input.KeyPageUp:
			s.set(s.value+(s.Max-s.Min)/10, Quick.Get(th), u)
		case input.KeyHome:
			s.set(s.Min, Quick.Get(th), u)
		case input.KeyEnd:
			s.set(s.Max, Quick.Get(th), u)
		default:
			return false
		}
	case input.FocusGained:
		s.ring.Animate(1, Quick.Get(th))
	case input.FocusLost:
		s.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node]. The slider fills the width it is
// given, or the theme's [FieldWidth] when the width is unbounded.
func (s *Slider) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	w := c.Max.W
	if w <= 0 {
		w = FieldWidth.Get(f.Theme)
	}
	s.size = c.Constrain(geom.Sz(w, ControlHeight.Get(f.Theme)))
	return s.size
}

// Paint implements [gunim.Node].
func (s *Slider) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	k := KnobSize.Get(th)
	th2 := SliderTrack.Get(th)
	y := box.H / 2
	x0, x1 := k/2, box.W-k/2
	at := s.at.Value()
	x := x0 + (x1-x0)*at
	p.RRect(geom.Rect{Min: geom.Pt(x0, y-th2/2), Max: geom.Pt(x1, y+th2/2)}, th2/2, paint.Solid(SwitchOff.Get(th)))
	p.RRect(geom.Rect{Min: geom.Pt(x0, y-th2/2), Max: geom.Pt(x, y+th2/2)}, th2/2, paint.Solid(Accent.Get(th)))

	grow := 1 + 0.2*max(s.hover.Value(), 0)
	d := k * grow
	knob := geom.Rc(x-d/2, y-d/2, d, d)
	focusRing(p, knob, d/2, s.ring.Value(), th)
	p.ShadowRRect(knob, d/2, paint.Solid(Knob.Get(th)), paint.Shadow{Offset: geom.Pt(0, 1), Blur: 3, Color: color.NRGBA{A: 0x60}})
}

// Tabs shows one of several pages under a row of titles. The line under
// the chosen title glides to the next one, and the new page slides in
// from the side it lies on while the old one fades.
//
// The row of titles takes focus, and then the arrow keys, Home and End
// choose a tab. A click on a title chooses it.
type Tabs struct {
	anim.Group
	Titles []string
	// Disabled lists the tabs that cannot be chosen now, in order, and may
	// be shorter than Titles. They are drawn faint.
	Disabled []bool
	// OnChange turns the chosen tab into an intent for the application.
	OnChange func(i int) gunim.Intent

	bar   *tabBar
	pages []gunim.Node
	// count is how many pages there are: those given, and those inserted
	// since, as the views mounted under the tabs' view are.
	count    int
	selected int
	// prev is the page leaving, or -1, and from is the side the new
	// page comes from: 1 from the right, -1 from the left.
	prev int
	from float32
	// line is the indicator's place, left and right edges; slide runs
	// from 0 to 1 as the new page arrives.
	line  *anim.Point
	slide *anim.Float
	ring  *anim.Float
	laid  bool

	shaped []shapedText
	// spans holds each title's left and right edge.
	spans [][2]float32
	head  float32
}

// NewTabs returns tabs showing pages under titles, the first chosen.
// The two lists pair up in order.
func NewTabs(titles []string, pages ...gunim.Node) *Tabs {
	t := &Tabs{Titles: titles, pages: pages, count: len(pages), prev: -1, line: anim.NewPoint(geom.Point{}), slide: anim.NewFloat(1), ring: anim.NewFloat(0)}
	t.bar = &tabBar{t: t}
	t.Add(t.line, t.slide, t.ring)
	return t
}

// Children implements [gunim.Composite]: the row of titles, then the
// pages.
func (t *Tabs) Children() []gunim.Node { return append([]gunim.Node{t.bar}, t.pages...) }

// Selected returns the chosen tab.
func (t *Tabs) Selected() int { return t.selected }

// Select chooses tab i without an intent. Call it from a view's update
// function.
func (t *Tabs) Select(i int, u *gunim.UI) {
	if i < 0 || i >= t.count || i == t.selected {
		return
	}
	t.prev, t.from = t.selected, 1
	if i < t.selected {
		t.from = -1
	}
	t.selected = i
	t.slide.Jump(0)
	t.slide.Animate(1, Settle.Get(u.Theme()))
	u.Invalidate()
}

func (t *Tabs) choose(i int, u *gunim.UI) {
	if i == t.selected || i < 0 || i >= t.count || flag(t.Disabled, i) {
		return
	}
	t.Select(i, u)
	if t.OnChange != nil {
		u.Send(t, t.OnChange(i))
	}
}

// tabBar is the row of titles.
type tabBar struct{ t *Tabs }

// Focusable implements [gunim.Focusable].
func (b *tabBar) Focusable() bool { return true }

// Handle implements [gunim.Handler]. It takes the clicks on the titles
// and the keys.
func (b *tabBar) Handle(e input.Event, u *gunim.UI) bool {
	t := b.t
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerDown:
		for i, s := range t.spans {
			if e.Pos.X >= s[0] && e.Pos.X < s[1] {
				t.choose(i, u)
			}
		}
	case input.KeyPress:
		switch e.Key {
		case input.KeyLeft:
			t.choose(t.enabled(t.selected-1, -1), u)
		case input.KeyRight:
			t.choose(t.enabled(t.selected+1, 1), u)
		case input.KeyHome:
			t.choose(t.enabled(0, 1), u)
		case input.KeyEnd:
			t.choose(t.enabled(t.count-1, -1), u)
		default:
			return false
		}
	case input.FocusGained:
		t.ring.Animate(1, Quick.Get(th))
	case input.FocusLost:
		t.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	return true
}

// Layout implements [gunim.Node].
func (b *tabBar) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	t := b.t
	th := f.Theme
	t.head = TabHeight.Get(th)
	pad := TabPadding.Get(th)
	if len(t.shaped) != len(t.Titles) {
		t.shaped = make([]shapedText, len(t.Titles))
	}
	t.spans = t.spans[:0]
	x := float32(0)
	for i, s := range t.Titles {
		w := t.shaped[i].shape(faceIn(Font, th), s, TextSize.Get(th)).Advance + 2*pad
		t.spans = append(t.spans, [2]float32{x, x + w})
		x += w
	}
	if t.selected < len(t.spans) {
		sp := t.spans[t.selected]
		to := geom.Pt(sp[0]+pad, sp[1]-pad)
		if t.laid {
			t.line.Animate(to, Bounce.Get(th))
		} else {
			t.line.Jump(to)
		}
	}
	t.laid = true
	w := c.Max.W
	if w <= 0 {
		w = x
	}
	return geom.Sz(w, t.head)
}

// Paint implements [gunim.Node].
func (b *tabBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	t := b.t
	th := f.Theme
	pad := TabPadding.Get(th)
	for i := range t.Titles {
		run := t.shaped[i].run
		ink := Placeholder.Get(th)
		switch {
		case flag(t.Disabled, i):
			ink.A /= 3
		case i == t.selected:
			ink = Ink.Get(th)
		}
		run.Paint(p, geom.Pt(t.spans[i][0]+pad, (t.head-run.Height())/2), ink)
	}
	p.RRect(geom.Rc(0, t.head-1, box.W, 1), 0, paint.Solid(FieldBorder.Get(th)))
	line := t.line.Value()
	p.RRect(geom.Rect{Min: geom.Pt(line.X, t.head-2.5), Max: geom.Pt(line.Y, t.head-0.5)}, 1, paint.Solid(Accent.Get(th)))
	if r := t.ring.Value(); r > 0.01 && t.selected < len(t.spans) {
		sp := t.spans[t.selected]
		focusRing(p, geom.Rc(sp[0]+3, 5, sp[1]-sp[0]-6, t.head-13), 6, r, th)
	}
}

// Layout implements [gunim.Node]. The titles sit in a row across the
// top; every page is laid out below them at the size left, so a page
// keeps its state and its scroll while another shows.
func (t *Tabs) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	t.count = kids.Len() - 1
	bar := kids.At(0)
	bs := bar.Layout(gunim.Constraints{Max: geom.Sz(c.Max.W, 0)})
	bar.Place(geom.Point{})
	size := c.Max
	if size.W <= 0 {
		size.W = bs.W
	}
	page := geom.Sz(size.W, max(0, size.H-t.head))
	var tallest float32
	for i := 1; i < kids.Len(); i++ {
		kid := kids.At(i)
		s := kid.Layout(gunim.Constraints{Min: geom.Sz(page.W, 0), Max: page})
		kid.Place(geom.Pt(0, t.head))
		tallest = max(tallest, s.H)
	}
	if size.H <= 0 {
		size.H = t.head + tallest
	}
	return c.Constrain(size)
}

// Paint implements [gunim.Node].
func (t *Tabs) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)

	// The pages, clipped below the titles: the old one fading where it
	// was, the new one sliding in over it.
	body := geom.Rect{Min: geom.Pt(0, t.head), Max: box.Point()}
	defer p.Layer(paint.LayerOpts{Bounds: body, Opacity: 1, Clip: true})()
	s := min(max(t.slide.Value(), 0), 1)
	if t.prev >= 0 && t.prev+1 < kids.Len() && s < 1 {
		closeOld := p.Layer(paint.LayerOpts{Bounds: body, Opacity: 1 - s})
		kids.At(t.prev + 1).Paint(p)
		closeOld()
	}
	if s >= 1 {
		t.prev = -1
	}
	if t.selected+1 < kids.Len() {
		shift := t.from * (1 - t.slide.Value()) * box.W * 0.15
		closeNew := p.Layer(paint.LayerOpts{Bounds: body, Opacity: s})
		defer closeNew()
		defer p.Push(paint.Translate(geom.Pt(shift, 0)))()
		kids.At(t.selected + 1).Paint(p)
	}
}

// focusRing draws the ring focus grows around r, t from 0 to 1.
func focusRing(p *paint.Painter, r geom.Rect, radius, t float32, th *theme.Live) {
	if t <= 0.01 {
		return
	}
	ring := Accent.Get(th)
	ring.A = uint8(float32(ring.A) * 0.56 * min(t, 1))
	grow := 3 * t
	p.RRectStroke(geom.Rect{Min: geom.Pt(r.Min.X-grow, r.Min.Y-grow), Max: geom.Pt(r.Max.X+grow, r.Max.Y+grow)},
		radius+grow, paint.Fill{}, paint.Stroke{Width: 2, Color: ring})
}

// bar draws a straight stroke from a to b, thick wide, with round ends.
func bar(p *paint.Painter, a, b geom.Point, thick float32, c color.NRGBA) {
	d := b.Sub(a)
	length := float32(math.Hypot(float64(d.X), float64(d.Y)))
	mid := geom.Pt((a.X+b.X)/2, (a.Y+b.Y)/2)
	angle := float32(math.Atan2(float64(d.Y), float64(d.X)))
	defer p.Push(paint.Rotate(angle, mid))()
	half := (length + thick) / 2
	p.RRect(geom.Rect{Min: geom.Pt(mid.X-half, mid.Y-thick/2), Max: geom.Pt(mid.X+half, mid.Y+thick/2)}, thick/2, paint.Solid(c))
}

func lerpPt(a, b geom.Point, t float32) geom.Point {
	return geom.Pt(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
}

func value(on bool) float32 {
	if on {
		return 1
	}
	return 0
}

// faintIf draws what follows faint while off is set, as a control that
// does not apply now is drawn, and returns what ends it.
func faintIf(p *paint.Painter, box geom.Size, off bool) func() {
	if !off {
		return func() {}
	}
	return p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}.Inset(geom.Uniform(-8)), Opacity: 0.4})
}

// enabled returns the first tab from i on, going by dir, that can be
// chosen, or -1.
func (t *Tabs) enabled(i, dir int) int {
	for ; i >= 0 && i < t.count; i += dir {
		if !flag(t.Disabled, i) {
			return i
		}
	}
	return -1
}
