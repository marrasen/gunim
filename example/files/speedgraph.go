package main

import (
	"errors"
	"image/color"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// speedEvery is how often an operation's speed is sampled, and
// speedSpan how many samples its graph shows.
const (
	speedEvery = 200 * time.Millisecond
	speedSpan  = 50
)

// cheerFor is how long a finished operation stays in the panel, showing
// it went well.
const cheerFor = 1300 * time.Millisecond

// Success is the colour of something that went well.
var Success = theme.Color("files.success", color.NRGBA{R: 0x4f, G: 0xd6, B: 0x9c, A: 0xff})

// speedometer turns the bytes an operation has moved, as it goes, into
// its speed.
type speedometer struct {
	at    time.Time
	bytes int64
	// rate is the last sample, and smooth the speed over the last few.
	rate, smooth float64
}

// add takes the bytes moved by now, and reports whether that made a new
// sample.
func (s *speedometer) add(now time.Time, bytes int64) bool {
	if s.at.IsZero() {
		s.at, s.bytes = now, bytes
		return false
	}
	dt := now.Sub(s.at).Seconds()
	if dt < speedEvery.Seconds() {
		return false
	}
	s.rate = float64(bytes-s.bytes) / dt
	if s.smooth == 0 {
		s.smooth = s.rate
	} else {
		s.smooth = 0.7*s.smooth + 0.3*s.rate
	}
	s.at, s.bytes = now, bytes
	return true
}

// left is how many seconds the rest of total takes from done at the
// smoothed speed, or 0 when that is unknown.
func (s *speedometer) left(done, total int64) float64 {
	if s.smooth <= 0 || total <= done {
		return 0
	}
	return float64(total-done) / s.smooth
}

// pace waits so the bytes a copy moves keep to env.limit a second, n more
// having just gone.
func (r *runner) pace(n int) error {
	if r.env.limit <= 0 {
		return nil
	}
	if r.paceStart.IsZero() {
		r.paceStart = time.Now()
	}
	r.paced += int64(n)
	ahead := time.Duration(float64(r.paced)/r.env.limit*float64(time.Second)) - time.Since(r.paceStart)
	if ahead <= 0 {
		return nil
	}
	select {
	case <-time.After(ahead):
		return nil
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// bigCopy copies a 2 GB file in the folder showing beside itself, at
// mbps megabytes a second or 150, making the file first when it is not
// there.
func (a *app) bigCopy(mbps string) {
	rate := 150.0
	if mbps != "" {
		v, err := strconv.ParseFloat(mbps, 64)
		if err != nil || v <= 0 {
			a.fail("big-copy takes megabytes a second, such as big-copy:80")
			return
		}
		rate = v
	}
	src := filepath.Join(a.nav.path, "Big video.mp4")
	_, err := os.Lstat(src)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if merr := makeBig(src, 2<<30); merr != nil {
			a.fail("Making the big file: " + merr.Error())
			return
		}
	case err != nil:
		a.fail("Looking for the big file: " + err.Error())
		return
	}
	a.ops.limit = rate * (1 << 20)
	a.startOp(job{kind: OpCopy, srcs: []string{src}, dest: a.nav.path}, "Copying Big video.mp4")
	a.ops.limit = 0
}

func registerSpeed(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, s OpSpeed, u *gunim.UI) { b.ops.speed(s, u) })
	gunim.RegisterPatch(w, "browser", func(b *browser, d OpDone, u *gunim.UI) { b.ops.done(d, u) })
}

// speed adds a sample to an operation's graph.
func (p *opsPanel) speed(s OpSpeed, u *gunim.UI) {
	if r, ok := widget.RowOf[*opRow](p.list, widget.Key(strconv.Itoa(s.ID))); ok {
		r.graph.SetRunning(true)
		r.graph.Add(s.Rate)
		r.samples++
		u.Invalidate()
	}
}

// done shows an operation ended: one that went well cheers.
func (p *opsPanel) done(d OpDone, u *gunim.UI) {
	r, ok := widget.RowOf[*opRow](p.list, widget.Key(strconv.Itoa(d.ID)))
	if !ok {
		return
	}
	r.graph.SetRunning(false)
	if d.OK {
		r.bar.Indeterminate = false
		r.bar.Set(1, u)
		r.detail.SetText("Done")
		r.badge.cheer(u)
	}
	u.Invalidate()
}

// newSpeedGraph returns the graph of an operation's speed.
func newSpeedGraph() *widget.LiveGraph {
	g := widget.NewLiveGraph(speedEvery, speedSpan)
	g.Label = func(v float64) string { return humanBytes(int64(v)) + "/s" }
	return g
}

// doneBadge holds an operation's Cancel button, and once the operation
// has gone well, a check mark that draws itself in its place.
type doneBadge struct {
	anim.Group
	cancel *widget.Button
	// pop grows the disc behind the check, stroke draws the check, and
	// since counts the seconds from the cheer.
	pop, stroke *anim.Float
	since       float64
	cheering    bool
}

func newDoneBadge(cancel *widget.Button) *doneBadge {
	b := &doneBadge{cancel: cancel, pop: anim.NewFloat(0), stroke: anim.NewFloat(0)}
	b.Add(b.pop, b.stroke)
	return b
}

// cheer starts the check mark.
func (b *doneBadge) cheer(u *gunim.UI) {
	if b.cheering {
		return
	}
	b.cheering = true
	b.cancel.On = nil
	b.pop.Animate(1, widget.Bounce.Get(u.Theme()))
	b.stroke.Animate(1, anim.Tween{Duration: 420 * time.Millisecond, Ease: anim.EaseInOut})
	u.Invalidate()
}

// glow is how bright the cheer is now, rising and falling once.
func (b *doneBadge) glow() float32 {
	if !b.cheering {
		return 0
	}
	t := min(b.since/0.9, 1)
	return float32(math.Sin(math.Pi * t))
}

// Step implements [gunim.Animator].
func (b *doneBadge) Step(dt time.Duration) bool {
	moving := b.Group.Step(dt)
	if b.cheering && b.since < 1 {
		b.since += dt.Seconds()
		moving = true
	}
	return moving
}

// Children implements [gunim.Composite].
func (b *doneBadge) Children() []gunim.Node { return []gunim.Node{b.cancel} }

// Layout implements [gunim.Node].
func (b *doneBadge) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(c)
	kid.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node]: the button shrinks away as the disc
// pops up in its place and the check draws itself on it.
func (b *doneBadge) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pop := b.pop.Value()
	if gone := min(max(pop*1.6, 0), 1); gone < 1 {
		func() {
			mid := geom.Pt(box.W/2, box.H/2)
			defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1 - gone})()
			defer p.Push(paint.Scale(1-0.3*gone, mid))()
			kids.At(0).Paint(p)
		}()
	}
	if pop <= 0.01 {
		return
	}
	th := f.Theme
	d := min(box.H, 28) * max(pop, 0)
	mid := geom.Pt(box.W-min(box.H, 28)/2, box.H/2)
	disc := geom.Rc(mid.X-d/2, mid.Y-d/2, d, d)
	green := Success.Get(th)
	glow := green
	glow.A = uint8(0x90 * b.glow())
	p.ShadowRRect(disc, d/2, paint.Solid(green), paint.Shadow{Blur: 14 * b.glow(), Color: glow})
	// The check: a short stroke down, then a long one up, drawn in turn.
	s := d / 28
	pts := []geom.Point{mid.Add(geom.Pt(-6.5*s, 0.5*s)), mid.Add(geom.Pt(-2*s, 5*s)), mid.Add(geom.Pt(7*s, -5*s))}
	drawTrail(p, pts, min(max(b.stroke.Value(), 0), 1), 2.6*s, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
}

// drawTrail draws the line through pts, as far as share of its length.
func drawTrail(p *paint.Painter, pts []geom.Point, share, width float32, c color.NRGBA) {
	total := float32(0)
	for i := 1; i < len(pts); i++ {
		total += dist(pts[i-1], pts[i])
	}
	left := total * share
	for i := 1; i < len(pts) && left > 0; i++ {
		a, b := pts[i-1], pts[i]
		l := dist(a, b)
		n := min(l, left)
		left -= n
		angle := float32(math.Atan2(float64(b.Y-a.Y), float64(b.X-a.X)))
		func() {
			defer p.Push(paint.Rotate(angle, a))()
			p.RRect(geom.Rc(a.X-width/2, a.Y-width/2, n+width, width), width/2, paint.Solid(c))
		}()
	}
}

func dist(a, b geom.Point) float32 {
	d := b.Sub(a)
	return float32(math.Hypot(float64(d.X), float64(d.Y)))
}

// glowBar is an operation's progress bar, which glows as the operation
// cheers.
type glowBar struct {
	bar   *widget.ProgressBar
	badge *doneBadge
}

// Children implements [gunim.Composite].
func (g *glowBar) Children() []gunim.Node { return []gunim.Node{g.bar} }

// Layout implements [gunim.Node].
func (g *glowBar) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(c)
	kid.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (g *glowBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	if glow := g.badge.glow(); glow > 0.01 {
		c := Success.Get(f.Theme)
		halo := c
		halo.A = uint8(0xb0 * glow)
		c.A = uint8(0xff * glow)
		p.ShadowRRect(geom.Rect{Max: box.Point()}, box.H/2, paint.Solid(c), paint.Shadow{Blur: 12 * glow, Color: halo})
	}
	kids.At(0).Paint(p)
}

// paintCheer sweeps a band of light across an operation's card as it
// cheers.
func paintCheer(p *paint.Painter, th *theme.Live, b *doneBadge, box geom.Size) {
	if !b.cheering || b.since >= 0.9 {
		return
	}
	t := float32(b.since / 0.9)
	c := Success.Get(th)
	edge := c
	edge.A = 0
	c.A = 0x38
	w := box.W * 0.35
	x := -w + (box.W+w)*t
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1, Clip: true,
		Radius: widget.CardRadius.Get(th)})()
	half := &paint.Gradient{From: geom.Pt(x, 0), To: geom.Pt(x+w/2, 0), Start: edge, End: c}
	p.RRect(geom.Rc(x, 0, w/2, box.H), 0, paint.Fill{Gradient: half})
	back := &paint.Gradient{From: geom.Pt(x+w/2, 0), To: geom.Pt(x+w, 0), Start: c, End: edge}
	p.RRect(geom.Rc(x+w/2, 0, w/2, box.H), 0, paint.Fill{Gradient: back})
}
