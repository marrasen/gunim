package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Fit is how an [Image] fills its box.
type Fit uint8

const (
	// FitContain shows the whole picture, as large as the box allows,
	// centred, leaving bars where the shapes differ.
	FitContain Fit = iota
	// FitCover fills the whole box, cropping the picture's edges where
	// the shapes differ.
	FitCover
	// FitFill stretches the picture to the box.
	FitFill
)

// Image shows a picture, and crossfades to a new one with the theme's
// [Crossfade] motion. It arrives and leaves with what holds it, so a
// view that fades in fades its pictures with it, and a [Hero] flies a
// picture that is whole from its first frame.
type Image struct {
	anim.Group

	Fit Fit
	// Radius rounds the corners of the drawn picture.
	Radius float32
	// Size is the size the image asks for. Zero asks for the picture's
	// own size, a logical pixel for each of its pixels.
	Size geom.Size

	src, prev *paint.Image
	// mix runs from 0, showing prev, to 1, showing src.
	mix *anim.Float
}

// NewImage returns an image showing src.
func NewImage(src *paint.Image) *Image {
	i := &Image{src: src, mix: anim.NewFloat(1)}
	i.Add(i.mix)
	return i
}

// Source returns the picture the image shows.
func (i *Image) Source() *paint.Image { return i.src }

// SetSource crossfades to src. Call it from a view's update function.
// A picture already showing stays as it is.
func (i *Image) SetSource(src *paint.Image, u *gunim.UI) {
	if src == i.src {
		return
	}
	i.prev, i.src = i.src, src
	i.mix.Jump(0)
	i.mix.Animate(1, Crossfade.Get(u.Theme()))
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (i *Image) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	size := i.Size
	if size == (geom.Size{}) && i.src != nil {
		w, h := i.src.Size()
		size = geom.Sz(float32(w), float32(h))
	}
	return c.Constrain(size)
}

// Paint implements [gunim.Node].
func (i *Image) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	t := min(max(i.mix.Value(), 0), 1)
	// The old picture stays nearly whole until the new one is nearly
	// there, so an opaque picture never lets the background through
	// halfway, and goes by the end, so a transparent one leaves nothing
	// behind.
	if i.prev != nil && t < 1 {
		i.draw(p, i.prev, box, 1-t*t*t)
	}
	if i.src != nil {
		i.draw(p, i.src, box, t)
	}
	if t >= 1 {
		i.prev = nil
	}
}

func (i *Image) draw(p *paint.Painter, m *paint.Image, box geom.Size, opacity float32) {
	dst, src := fitRects(i.Fit, box, m)
	p.Image(m, dst, paint.ImageOpts{Src: src, Radius: i.Radius, Opacity: opacity})
}

// fitRects returns where m goes in box, and the part of m it shows.
func fitRects(fit Fit, box geom.Size, m *paint.Image) (dst, src geom.Rect) {
	w, h := m.Size()
	iw, ih := float32(w), float32(h)
	dst = geom.Rect{Max: box.Point()}
	src = geom.Rect{Max: geom.Pt(iw, ih)}
	if iw == 0 || ih == 0 || box.W <= 0 || box.H <= 0 {
		return dst, src
	}
	switch fit {
	case FitContain:
		s := min(box.W/iw, box.H/ih)
		dw, dh := iw*s, ih*s
		dst = geom.Rc((box.W-dw)/2, (box.H-dh)/2, dw, dh)
	case FitCover:
		s := max(box.W/iw, box.H/ih)
		sw, sh := box.W/s, box.H/s
		src = geom.Rc((iw-sw)/2, (ih-sh)/2, sw, sh)
	case FitFill:
	}
	return dst, src
}
