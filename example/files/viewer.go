package main

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The viewer's motion and timing.
var (
	zoomMotion  = anim.Spring{Response: 0.32, Damping: 0.86}
	slideMotion = anim.Spring{Response: 0.38, Damping: 0.9}
)

const (
	// captionLinger is how long the caption stays once the pointer stops.
	captionLinger = 1600 * time.Millisecond
	// slideBy is how far a picture slides as the viewer steps, as a share of the window's width.
	slideBy = 0.35
	// viewerMargin is the room around the picture, and captionRoom the room under it for the caption.
	viewerMargin = 40
	captionRoom  = 64
)

func registerViewer(w *gunim.Window) {
	gunim.RegisterView(w, "viewer", func(Viewing) *viewer { return newViewer() }, (*viewer).set)
}

// viewer shows a picture large over the window. The picture flies up out of its tile as it opens and back into it
// as it closes; the wheel zooms it about the pointer, a drag pans it, and the arrow keys slide to the next.
type viewer struct {
	anim.Group
	state   Viewing
	hero    *widget.Hero
	pic     *viewerPic
	name    *widget.Label
	info    *widget.Label
	in      *anim.Float
	fit     *anim.Rect
	zoom    *anim.Float
	pan     *anim.Point
	capIn   *anim.Float
	bump    *anim.Float
	slideIn *anim.Float
	// old is the picture sliding out, from oldRect, as slideOut runs from 0 to 1 the way oldDir says.
	old      *paint.Image
	oldRect  geom.Rect
	oldDir   float32
	slideOut *anim.Float
	box      geom.Size
	scale    float32
	asked    [2]int
	held     bool
	dragged  bool
	outside  bool
	grab     geom.Point
	grabPan  geom.Point
	closing  bool
	still    func()
}

func newViewer() *viewer {
	v := &viewer{in: anim.NewFloat(0), fit: anim.NewRect(geom.Rect{}), zoom: anim.NewFloat(1),
		pan: anim.NewPoint(geom.Point{}), capIn: anim.NewFloat(1), bump: anim.NewFloat(0), slideIn: anim.NewFloat(0),
		slideOut: anim.NewFloat(1), scale: 1}
	v.Add(v.in, v.fit, v.zoom, v.pan, v.capIn, v.bump, v.slideIn, v.slideOut)
	v.pic = newViewerPic()
	v.hero = widget.NewHero("", v.pic)
	v.name = widget.NewLabel("")
	v.name.Face, v.name.MaxLines, v.name.Align = widget.BoldFont, 1, text.AlignCenter
	v.name.Color = captionInk
	v.info = widget.NewLabel("")
	v.info.Size, v.info.MaxLines, v.info.Align = SmallText, 2, text.AlignCenter
	v.info.Color = captionFaint
	return v
}

// captionInk and captionFaint are the caption's colours, light on the dark backdrop in either theme.
var (
	captionInk   = theme.Color("files.viewer.ink", color.NRGBA{R: 0xf2, G: 0xf4, B: 0xf8, A: 0xff})
	captionFaint = theme.Color("files.viewer.faint", color.NRGBA{R: 0xb4, G: 0xbb, B: 0xc8, A: 0xff})
)

// set takes the viewer's state: a new picture slides in, and the same one takes its sharper decoding.
func (v *viewer) set(s Viewing, u *gunim.UI) {
	was := v.state
	v.state = s
	src := s.Image
	if src == nil {
		src = s.Thumb
	}
	switch {
	case was.Seq == 0:
		v.hero.Tag = s.Path
		v.pic.show(src, false)
		u.Focus(v)
	case was.Seq != s.Seq:
		v.old, v.oldRect, v.oldDir = v.pic.Source(), v.shown(), float32(s.Travel)
		v.slideOut.Jump(0)
		v.slideOut.Animate(1, slideMotion)
		v.slideIn.Jump(float32(s.Travel))
		v.slideIn.Animate(0, slideMotion)
		v.hero.Tag = s.Path
		v.pic.show(src, false)
		v.zoom.Jump(1)
		v.pan.Jump(geom.Point{})
		v.asked = [2]int{}
	case src != v.pic.Source():
		v.pic.show(src, true)
	}
	v.pic.broken = s.Err != ""
	v.name.SetText(s.Name)
	v.info.SetText(v.describe())
	v.info.Color = captionFaint
	if s.Err != "" {
		v.info.Color = ErrorInk
	}
	v.wake(u)
	u.Invalidate()
}

// describe says the picture's size and where it is in the folder, or why it cannot be shown.
func (v *viewer) describe() string {
	s := v.state
	at := fmt.Sprintf("%d of %d", s.Index, s.Count)
	switch {
	case s.Err != "":
		return s.Err
	case s.W > 0:
		return fmt.Sprintf("%d × %d  ·  %s", s.W, s.H, at)
	case s.Loading:
		return "Reading the picture…  ·  " + at
	}
	return at
}

// wake shows the caption, and fades it once the pointer has been still for a moment.
func (v *viewer) wake(u *gunim.UI) {
	v.capIn.Animate(1, widget.Quick.Get(u.Theme()))
	if v.still != nil {
		v.still()
	}
	v.still = u.After(captionLinger, func(u *gunim.UI) {
		v.still = nil
		if v.state.Err == "" {
			v.capIn.Animate(0, widget.Settle.Get(u.Theme()))
		}
	})
}

// close flies the picture back into its tile, and tells the application.
func (v *viewer) close(u *gunim.UI) {
	if v.closing {
		return
	}
	v.closing = true
	if v.still != nil {
		v.still()
	}
	u.Remove(v)
	u.Send(v, ViewerClosed{Seq: v.state.Seq})
}

// aspect is the picture's width over its height, from the best that is known of it.
func (v *viewer) aspect() float32 {
	if v.state.W > 0 && v.state.H > 0 {
		return float32(v.state.W) / float32(v.state.H)
	}
	if src := v.pic.Source(); src != nil {
		w, h := src.Size()
		return float32(w) / float32(max(h, 1))
	}
	return 1.5
}

// fitRect is where the picture goes at zoom 1: as large as the room allows, but no larger than its own pixels.
func (v *viewer) fitRect() geom.Rect {
	room := geom.Rc(viewerMargin, viewerMargin, v.box.W-2*viewerMargin, v.box.H-viewerMargin-captionRoom)
	a := v.aspect()
	w, h := room.Size().W, room.Size().W/a
	if h > room.Size().H {
		w, h = room.Size().H*a, room.Size().H
	}
	if v.state.W > 0 {
		if own := float32(v.state.W) / v.scale; w > own {
			w, h = own, own/a
		}
	}
	w, h = max(w, 1), max(h, 1)
	c := room.Center()
	return geom.Rc(c.X-w/2, c.Y-h/2, w, h)
}

// actual is the zoom that shows the picture a pixel for a pixel.
func (v *viewer) actual() float32 {
	if v.state.W == 0 {
		return 2
	}
	return max(float32(v.state.W)/v.scale/max(v.fit.Target().Size().W, 1), 1)
}

// zoomAt zooms to z about p, keeping the point of the picture under p where it is.
func (v *viewer) zoomAt(z float32, p geom.Point) {
	z = min(max(z, 1), max(8, v.actual()*4))
	fit := v.fit.Target()
	c := fit.Center()
	z0 := v.zoom.Target()
	pan := v.pan.Target()
	q := p.Sub(c)
	next := q.Sub(q.Sub(pan).Mul(z / z0))
	if z <= 1.001 {
		next = geom.Point{}
	}
	v.zoom.Animate(z, zoomMotion)
	v.pan.Animate(v.clampPan(next, z), zoomMotion)
}

// clampPan keeps a zoomed picture from being dragged off the screen.
func (v *viewer) clampPan(p geom.Point, z float32) geom.Point {
	fit := v.fit.Target()
	mx := max(0, (fit.Size().W*z-v.box.W)/2+viewerMargin)
	my := max(0, (fit.Size().H*z-v.box.H)/2+viewerMargin)
	return geom.Pt(min(max(p.X, -mx), mx), min(max(p.Y, -my), my))
}

// shown is where the picture is drawn now, in the viewer's space.
func (v *viewer) shown() geom.Rect {
	fit := v.fit.Value()
	z := v.zoom.Value()
	c := fit.Center().Add(v.pan.Value()).Add(geom.Pt(v.offset(), 0))
	w, h := fit.Size().W*z, fit.Size().H*z
	return geom.Rc(c.X-w/2, c.Y-h/2, w, h)
}

// offset is how far the picture is slid across, stepping or bumping at an end.
func (v *viewer) offset() float32 {
	return v.slideIn.Value()*v.box.W*slideBy + v.bump.Value()
}

// step goes to the next picture, or nudges the picture at an end.
func (v *viewer) step(dir int, u *gunim.UI) {
	s := v.state
	if s.Index+dir < 1 || s.Index+dir > s.Count {
		v.bump.Jump(float32(-dir) * 36)
		v.bump.Animate(0, anim.Bouncy)
		u.Invalidate()
		return
	}
	u.Send(v, ViewerStep{Dir: dir})
}

// Focusable implements [gunim.Focusable].
func (v *viewer) Focusable() bool { return !v.closing }

// ZoomsWithWheel implements [gunim.WheelZoomer].
func (v *viewer) ZoomsWithWheel() bool { return true }

// Transition implements [gunim.Transitioner].
func (v *viewer) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		v.in.Animate(1, Page.Get(f.Theme))
	case gunim.Exiting:
		v.in.Animate(0, widget.Settle.Get(f.Theme))
	case gunim.Present:
	}
	return !v.in.Active()
}

// Children implements [gunim.Composite].
func (v *viewer) Children() []gunim.Node { return []gunim.Node{v.hero, v.name, v.info} }

// Handle implements [gunim.Handler].
func (v *viewer) Handle(e input.Event, u *gunim.UI) bool {
	if v.closing {
		return false
	}
	switch e := e.(type) {
	case input.KeyPress:
		return v.key(e, u)
	case input.Scroll:
		n := e.Notches.Y
		if n == 0 {
			n = e.Delta.Y / 60
		}
		v.zoomAt(v.zoom.Target()*float32(math.Pow(1.25, float64(n))), e.Pos)
		v.wake(u)
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return true
		}
		if e.Clicks == 2 && v.shown().Contains(e.Pos) {
			if v.zoom.Target() > 1.01 {
				v.zoomAt(1, e.Pos)
			} else {
				v.zoomAt(max(v.actual(), 2), e.Pos)
			}
			v.held = false
			return true
		}
		v.held, v.dragged, v.outside = true, false, !v.shown().Contains(e.Pos)
		v.grab, v.grabPan = e.Pos, v.pan.Value()
	case input.PointerMove:
		v.wake(u)
		if !v.held {
			return true
		}
		d := e.Pos.Sub(v.grab)
		if !v.dragged && d.X*d.X+d.Y*d.Y > 16 {
			v.dragged = true
		}
		if v.dragged && v.zoom.Target() > 1.01 {
			v.pan.Jump(v.clampPan(v.grabPan.Add(d), v.zoom.Target()))
		}
	case input.PointerUp:
		if !v.held {
			return true
		}
		v.held = false
		if !v.dragged && v.outside {
			v.close(u)
		}
	case input.FocusGained, input.FocusLost:
	default:
		return false
	}
	u.Invalidate()
	return true
}

func (v *viewer) key(e input.KeyPress, u *gunim.UI) bool {
	if e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) {
		return false
	}
	mid := v.fit.Target().Center()
	switch {
	case e.Key == input.KeyEscape, e.Key == input.KeySpace, e.Key == input.KeyBackspace:
		v.close(u)
	case e.Key == input.KeyRight, e.Key == input.KeyDown, e.Key == input.KeyPageDown:
		v.step(1, u)
	case e.Key == input.KeyLeft, e.Key == input.KeyUp, e.Key == input.KeyPageUp:
		v.step(-1, u)
	case e.Char == '+' || e.Char == '=' || e.Key == input.KeyKPAdd:
		v.zoomAt(v.zoom.Target()*1.25, mid)
	case e.Char == '-' || e.Key == input.KeyKPSubtract:
		v.zoomAt(v.zoom.Target()/1.25, mid)
	case e.Char == '0' || e.Key == input.KeyKP0:
		v.zoomAt(1, mid)
	case e.Char == '1':
		v.zoomAt(v.actual(), mid)
	default:
		return true
	}
	v.wake(u)
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (v *viewer) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	v.box, v.scale = c.Max, max(f.Scale, 1)
	fit := v.fitRect()
	if v.fit.Target() == (geom.Rect{}) {
		v.fit.Jump(fit)
	} else {
		v.fit.Animate(fit, widget.Quick.Get(f.Theme))
	}
	hero := kids.At(0)
	r := v.fit.Value()
	hero.Layout(gunim.Tight(r.Size()))
	hero.Place(r.Min)
	w := min(c.Max.W-2*viewerMargin, 640)
	name, info := kids.At(1), kids.At(2)
	ns := name.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, 40)})
	is := info.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, 40)})
	top := c.Max.H - captionRoom/2 - (ns.H+is.H)/2 - 6
	name.Place(geom.Pt((c.Max.W-w)/2, top))
	info.Place(geom.Pt((c.Max.W-w)/2, top+ns.H+2))
	v.askSharper(f)
	return c.Max
}

// askSharper asks for the picture decoded at the size it shows at, when that is larger than asked before.
func (v *viewer) askSharper(f gunim.Frame) {
	if v.state.Err != "" || v.closing {
		return
	}
	fit := v.fit.Target().Size()
	z := v.zoom.Target() * v.scale
	w, h := int(math.Ceil(float64(fit.W*z)/64))*64, int(math.Ceil(float64(fit.H*z)/64))*64
	if w <= v.asked[0] && h <= v.asked[1] {
		return
	}
	v.asked = [2]int{w, h}
	f.Send(v, ViewerWants{Seq: v.state.Seq, W: w, H: h})
}

// Paint implements [gunim.Node].
func (v *viewer) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	in := min(max(v.in.Value(), 0), 1)
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(color.NRGBA{A: uint8(0xe8 * in)}))
	if out := v.slideOut.Value(); v.old != nil && out < 0.999 {
		r := v.oldRect.Add(geom.Pt(-v.oldDir*out*box.W*slideBy, 0))
		p.Image(v.old, r, paint.ImageOpts{Radius: 3, Opacity: (1 - out) * in})
	}
	func() {
		fit := v.fit.Value()
		c := fit.Center()
		fade := in * (1 - min(abs32(v.slideIn.Value()), 1))
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: max(fade, 0)})()
		defer p.Push(paint.Translate(v.pan.Value().Add(geom.Pt(v.offset(), 0))))()
		defer p.Push(paint.Scale(v.zoom.Value()*(0.9+0.1*in), c))()
		kids.At(0).Paint(p)
	}()
	v.paintChrome(p, f, box, kids, in)
}

// paintChrome draws the caption and the arrows either side, which fade together.
func (v *viewer) paintChrome(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children, in float32) {
	t := in * min(max(v.capIn.Value(), 0), 1)
	if t <= 0.01 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: t})()
	name, info := kids.At(1), kids.At(2)
	pill := geom.Rect{Min: geom.Pt(box.W/2-min(box.W/2-viewerMargin, 340), box.H-captionRoom+6),
		Max: geom.Pt(box.W/2+min(box.W/2-viewerMargin, 340), box.H-10)}
	p.RRect(pill, 14, paint.Solid(color.NRGBA{R: 0x1c, G: 0x1f, B: 0x26, A: 0xd8}))
	name.Paint(p)
	info.Paint(p)
	s := v.state
	ink := captionInk.Get(f.Theme)
	for _, side := range []struct {
		show bool
		x    float32
		turn float32
	}{{s.Index > 1, 18, -1}, {s.Index < s.Count, box.W - 18, 1}} {
		if !side.show {
			continue
		}
		y := box.H / 2
		c := ink
		c.A = 0xb0
		// A chevron: two short strokes meeting at its point.
		stroke := geom.Rc(side.x-10.5, y-1.5, 12, 3)
		if side.turn < 0 {
			stroke = geom.Rc(side.x-1.5, y-1.5, 12, 3)
		}
		for k := range 2 {
			dir := float32(1-2*k) * side.turn
			func() {
				defer p.Push(paint.Rotate(dir*math.Pi/4, geom.Pt(side.x, y)))()
				p.RRect(stroke, 1.5, paint.Solid(c))
			}()
		}
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// viewerPic is the picture in the viewer: shown at once, or crossfading from the one before when a sharper decoding
// arrives.
type viewerPic struct {
	anim.Group
	img, prev *paint.Image
	mix       *anim.Float
	// broken is set when the picture cannot be read, and shows a mark in its place.
	broken bool
}

func newViewerPic() *viewerPic {
	p := &viewerPic{mix: anim.NewFloat(1)}
	p.Add(p.mix)
	return p
}

// Source returns the picture showing.
func (p *viewerPic) Source() *paint.Image { return p.img }

// show shows img, crossfading from the picture before with fade.
func (p *viewerPic) show(img *paint.Image, fade bool) {
	p.prev, p.img = p.img, img
	p.mix.Jump(1)
	if fade && p.prev != nil {
		p.mix.Jump(0)
		p.mix.Animate(1, anim.Gentle)
	}
}

// Layout implements [gunim.Node].
func (p *viewerPic) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

// Paint implements [gunim.Node].
func (p *viewerPic) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rect{Max: box.Point()}
	t := min(max(p.mix.Value(), 0), 1)
	if p.prev != nil && t < 1 {
		pt.Image(p.prev, r, paint.ImageOpts{Radius: 3, Opacity: 1})
	}
	if p.img == nil {
		pt.RRect(r, 3, paint.Solid(color.NRGBA{R: 0x26, G: 0x29, B: 0x31, A: 0xff}))
		if p.broken {
			s := min(box.W, box.H, 56)
			defer pt.Push(paint.Translate(geom.Pt((box.W-s)/2, (box.H-s)/2)))()
			(&errorMark{on: true}).Paint(pt, f, geom.Sz(s, s), gunim.Children{})
		}
		return
	}
	pt.Image(p.img, r, paint.ImageOpts{Radius: 3, Opacity: t})
}
