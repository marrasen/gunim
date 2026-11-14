package plugui

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Reduced reads how far a part of a plugin turns the sound heard down,
// in decibels, below zero: the sum of what the processor keeps by frame
// of its IDs, as [plug.Tracer] traces, for the very frames heard, so it
// moves with the sound. Between the chunks the sound comes in it holds,
// and once the sound stops it reads nothing.
type Reduced struct {
	h    plug.Host
	l    plug.Listener
	ids  []uint32
	last float32
}

// NewReduced returns a reader of how far ids turn the sound h gives
// down, together.
func NewReduced(h plug.Host, ids ...uint32) *Reduced {
	_, out := h.Sound()
	return &Reduced{h: h, l: plug.Listener{Sound: out}, ids: ids}
}

// Read reads it, a frame of the window, dt, on, and whether the sound
// plays.
func (r *Reduced) Read(dt time.Duration) (db float32, live bool) {
	frames, _, playing := r.l.Hear(4096, dt)
	switch {
	case len(frames) > 0:
		end := r.l.At()
		r.last = 0
		for f := end - int64(len(frames)/2); f < end; f += plug.TraceFrames {
			var sum float32
			for _, id := range r.ids {
				v, _ := r.h.Traced(id, f)
				sum += v
			}
			r.last = min(r.last, sum)
		}
	case !playing:
		r.last = 0
	}
	return r.last, playing
}

// The bar in a section's head: how tall it is, the least room a section
// keeps for it, and how wide it is at most.
const (
	sectionBarH   = 4
	sectionBarMin = 24
	sectionBarMax = 48
)

// sectionBar is the bar in a section's head of how far its part turns
// the sound down: growing from the right, by [ChipScale], full decibels
// filling it.
type sectionBar struct {
	r     *Reduced
	color theme.Token[color.NRGBA]
	full  float64
	shown *anim.Float
	group anim.Group
	// read says the bar has read the reduction once: the first reading
	// shows at once.
	read bool
}

func newSectionBar(r *Reduced, c theme.Token[color.NRGBA], full float64) *sectionBar {
	b := &sectionBar{r: r, color: c, full: full, shown: anim.NewFloat(0)}
	b.group.Add(b.shown)
	return b
}

// Layout implements [gunim.Node].
func (b *sectionBar) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return geom.Sz(min(c.Max.W, sectionBarMax), sectionBarH)
}

// Paint implements [gunim.Node].
func (b *sectionBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	if box.W < 1 {
		return
	}
	th := f.Theme
	p.RRect(geom.Rect{Max: box.Point()}, box.H/2, paint.Solid(paint.Faded(audioui.Ground.Get(th), 0.85)))
	if w := box.W * b.filled(); w > 0.5 {
		p.RRect(geom.Rc(box.W-w, 0, w, box.H), box.H/2, paint.Solid(b.color.Get(th)))
	}
}

// filled is how much of the bar is lit, from 0 to 1.
func (b *sectionBar) filled() float32 {
	return max(-ChipScale(float64(b.shown.Value()), b.full), 0)
}

// Step implements [gunim.Animator]: the bar follows the reduction.
func (b *sectionBar) Step(dt time.Duration) bool {
	db, live := b.r.Read(dt)
	if b.read {
		b.shown.Animate(db, anim.Snappy)
	} else {
		b.shown.Jump(db)
		b.read = true
	}
	return b.group.Step(dt) || live
}
