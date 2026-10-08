package themeedit

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// swings is how many times the preview's dot crosses its track after a
// change, and rest how long it rests at each end.
const (
	swings = 4
	rest   = 350 * time.Millisecond
)

// preview shows a spring's motion: a dot that crosses a track and back
// with it, a few times after the spring changes and as the pointer
// comes over it. An instant spring makes the dot jump.
type preview struct {
	anim.Group
	label  string
	spring anim.Spring
	dot    *anim.Float
	// left is how many crossings are still to come, and waited how long
	// the dot has rested at the end it is at.
	left   int
	waited time.Duration
}

func newPreview(label string) *preview {
	p := &preview{label: label, dot: anim.NewFloat(0)}
	p.Add(p.dot)
	return p
}

// play moves the dot with s a few times.
func (p *preview) play(s anim.Spring, u *gunim.UI) {
	p.spring = s
	if p.left == 0 {
		p.waited = rest
	}
	p.left = swings
	u.Invalidate()
}

// Step implements [gunim.Animator]: once the dot settles at one end, it
// rests, then crosses to the other while crossings are left.
func (p *preview) Step(dt time.Duration) bool {
	moving := p.Group.Step(dt)
	if moving || p.left == 0 {
		return moving
	}
	p.waited += dt
	if p.waited >= rest {
		p.waited = 0
		p.left--
		p.dot.Animate(1-p.dot.Target(), p.spring)
	}
	return true
}

// Handle implements [gunim.Handler]: the pointer coming over the
// preview plays it.
func (p *preview) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.PointerEnter); ok {
		p.play(p.spring, u)
	}
	return false
}

// Layout implements [gunim.Node].
func (p *preview) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(PreviewWidth.Get(f.Theme), widget.ControlHeight.Get(f.Theme)))
}

// Paint implements [gunim.Node].
func (p *preview) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	d := PreviewDot.Get(th)
	track := widget.SliderTrack.Get(th)
	// The dot travels the middle eight tenths of the track, so a spring
	// that overshoots stays inside it.
	y, span := box.H/2, max(0, box.W-d)
	pt.RRect(geom.Rc(d/2, y-track/2, span, track), track/2, paint.Solid(widget.SwitchOff.Get(th)))
	x := d/2 + span*(0.1+0.8*max(-0.125, min(p.dot.Value(), 1.125)))
	pt.RRect(geom.Rc(x-d/2, y-d/2, d, d), d/2, paint.Solid(widget.Accent.Get(th)))
}

// Access implements [gunim.Accessible].
func (p *preview) Access() access.Info {
	return access.Info{Role: access.RoleImage, Name: "How " + p.label + " moves"}
}
