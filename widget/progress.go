package widget

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Progress bar tokens.
var (
	ProgressTrack  = theme.Color("progress.track", color.NRGBA{R: 0x3a, G: 0x40, B: 0x50, A: 0xff})
	ProgressHeight = theme.Length("progress.height", 6)
)

// ProgressBar shows how far some work has got, as a fill along a
// track in the accent colour.
//
// Each new value glides the fill there, so progress that arrives in
// bursts still reads as steady movement. Until the work knows how much
// there is, the bar is Indeterminate: a stripe slides along it, to say
// the work is alive.
type ProgressBar struct {
	anim.Group
	// Indeterminate is set while the amount of work is unknown.
	Indeterminate bool

	value *anim.Float
	// since is when the stripe began, so its place follows the frame
	// clock.
	since time.Time
}

// NewProgressBar returns an empty bar.
func NewProgressBar() *ProgressBar {
	b := &ProgressBar{value: anim.NewFloat(0)}
	b.Add(b.value)
	return b
}

// Set moves the fill to v, from 0 to 1, gliding there.
func (b *ProgressBar) Set(v float32, u *gunim.UI) {
	v = min(max(v, 0), 1)
	if v == b.value.Target() {
		return
	}
	b.value.Animate(v, Settle.Get(u.Theme()))
}

// Value is where the fill is now, from 0 to 1.
func (b *ProgressBar) Value() float32 { return min(max(b.value.Value(), 0), 1) }

// Step implements [anim.Animator]. An indeterminate bar keeps
// drawing, as its stripe is always moving.
func (b *ProgressBar) Step(dt time.Duration) bool {
	return b.Group.Step(dt) || b.Indeterminate
}

// Layout implements [gunim.Node]: as wide as it may be, and as tall as
// the theme says.
func (b *ProgressBar) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Max.W, ProgressHeight.Get(f.Theme)))
}

// stripe is the share of the track the indeterminate stripe covers,
// and sweep how long it takes to cross.
const (
	stripe = 0.3
	sweep  = 1400 * time.Millisecond
)

// Paint implements [gunim.Node].
func (b *ProgressBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	r := geom.Rect{Max: box.Point()}
	radius := box.H / 2
	p.RRect(r, radius, paint.Solid(ProgressTrack.Get(f.Theme)))
	fill := Accent.Get(f.Theme)
	if b.Indeterminate {
		if b.since.IsZero() {
			b.since = f.Now
		}
		// The stripe enters from the left and leaves at the right.
		t := float32(f.Now.Sub(b.since)%sweep) / float32(sweep)
		x := (t*(1+stripe) - stripe) * box.W
		defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true})()
		p.RRect(geom.Rect{Min: geom.Pt(x, 0), Max: geom.Pt(x+stripe*box.W, box.H)}, radius, paint.Solid(fill))
		return
	}
	b.since = time.Time{}
	if w := b.Value() * box.W; w > 0 {
		p.RRect(geom.Rect{Max: geom.Pt(max(w, box.H), box.H)}, radius, paint.Solid(fill))
	}
}
