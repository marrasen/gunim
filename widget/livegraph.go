package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// minAcross is the fewest samples a live graph's width shows, so the
// first few are not drawn a width apart.
const minAcross = 20

// LiveGraphHeight is a live graph's height.
var LiveGraphHeight = theme.Length("livegraph.height", 56)

// LiveGraph draws a value as it runs, such as the speed of a copy: a
// smooth curve over a glowing fill, sliding left every frame as samples
// come in, with a bright head on the newest and what it says beside it.
// The scale glides to the highest value in view rather than jumping.
//
// Samples come with [LiveGraph.Add], one every Every. The graph slides a
// sample's width in that time, so it moves at the frame rate whatever
// the rate samples come at, and rests while it is not running.
type LiveGraph struct {
	// Every is how often a sample comes.
	Every time.Duration
	// Span is how many samples fit across.
	Span int
	// Label is what the head says for a value, such as "12 MB/s"; nil
	// says nothing.
	Label func(v float64) string

	samples []float64
	running bool
	// since is how long ago the newest sample came, in the graph's own
	// time, and glow the head's pulse.
	since time.Duration
	glow  float64
	top   *anim.Float
	// across is how many samples the width shows, gliding from few,
	// which fill it while there are few, to Span.
	across *anim.Float
	text   shapedText
}

// NewLiveGraph returns a graph of span samples, one every every.
func NewLiveGraph(every time.Duration, span int) *LiveGraph {
	return &LiveGraph{Every: every, Span: span, top: anim.NewFloat(0), across: anim.NewFloat(float32(minAcross))}
}

// Add adds a sample, which slides in from the right.
func (g *LiveGraph) Add(v float64) {
	g.samples = append(g.samples, max(0, v))
	if n := len(g.samples) - g.Span - 2; n > 0 {
		g.samples = g.samples[n:]
	}
	g.since = 0
	g.across.Animate(float32(min(max(len(g.samples), minAcross), g.Span)), anim.Gentle)
	most := 0.0
	for _, s := range g.samples {
		most = max(most, s)
	}
	if most > 0 {
		g.top.Animate(float32(most*1.15), anim.Gentle)
	}
}

// SetRunning starts the graph sliding, or rests it.
func (g *LiveGraph) SetRunning(on bool) { g.running = on }

// Step implements [gunim.Animator]: while running, the graph moves on
// every frame.
func (g *LiveGraph) Step(dt time.Duration) bool {
	moving := g.top.Step(dt)
	if g.across.Step(dt) {
		moving = true
	}
	if g.running {
		g.since += dt
		g.glow += dt.Seconds()
		moving = true
	}
	return moving
}

// Layout implements [gunim.Node]: nothing until there are two samples.
func (g *LiveGraph) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	if len(g.samples) < 2 {
		return c.Constrain(geom.Size{})
	}
	return c.Constrain(geom.Sz(c.Max.W, LiveGraphHeight.Get(f.Theme)))
}

// Paint implements [gunim.Node].
func (g *LiveGraph) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	n := len(g.samples)
	top := float64(g.top.Value())
	if box.H <= 0 || n < 2 || top <= 0 || g.Span < 2 {
		return
	}
	th := f.Theme
	accent := Accent.Get(th)
	// The plot sits below room for the head's label.
	plotTop := TextSize.Get(th) + 4
	plotH := box.H - plotTop - 1
	step := box.W / max(float32(g.across.Value())-1, 1)
	// How far the newest sample has slid in from the right edge.
	slid := float32(1)
	if g.Every > 0 {
		slid = min(float32(g.since)/float32(g.Every), 1)
	}
	xOf := func(i float64) float32 {
		return box.W - (float32(float64(n-1)-i)+slid)*step
	}
	yOf := func(v float64) float32 {
		return plotTop + plotH*(1-float32(min(v/top, 1)))
	}
	// A Catmull-Rom curve through the samples, looked at every 2
	// pixels, so the line bends smoothly between them.
	valueAt := func(t float64) float64 {
		i := int(math.Floor(t))
		u := t - float64(i)
		// Each sample weighed with its neighbours, so a rate taken ten
		// times a second reads as a line rather than as noise.
		raw := func(k int) float64 { return g.samples[min(max(k, 0), n-1)] }
		at := func(k int) float64 { return (raw(k-1) + 2*raw(k) + raw(k+1)) / 4 }
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		v := 0.5 * (2*p1 + (-p0+p2)*u + (2*p0-5*p1+4*p2-p3)*u*u + (-p0+3*p1-3*p2+p3)*u*u*u)
		return max(0, v)
	}
	first := max(0, float64(n-1)-float64(box.W/step)-1)
	var pts []geom.Point
	for x := max(0, xOf(first)); ; x += 2 {
		head := xOf(float64(n - 1))
		if x > head {
			x = head
		}
		t := float64(n-1) - float64((head-x)/step)
		pts = append(pts, geom.Pt(x, yOf(valueAt(t))))
		if x >= head {
			break
		}
	}
	// The fill, fading to nothing at the foot: columns two device
	// pixels wide, whose edges fall on device pixels, so they meet
	// without a seam or an overlap either would show as a stripe.
	fill := accent
	fill.A = 0x60
	faded := accent
	faded.A = 0
	grad := &paint.Gradient{From: geom.Pt(0, plotTop), To: geom.Pt(0, box.H), Start: fill, End: faded}
	// t maps the graph's x to the window's, in logical pixels, and dev
	// is device pixels per logical one.
	t := p.Transform()
	sx, dev := max(t.A, 1e-3), max(f.Scale, 1)
	snap := func(x float32) float32 {
		wx := sx*x + t.C
		return (float32(math.Round(float64(wx*dev)))/dev - t.C) / sx
	}
	head := xOf(float64(n - 1))
	colW := 2 / (dev * sx)
	for x := snap(max(0, xOf(first))); x < head; {
		next := min(snap(x+colW), head)
		if next <= x {
			next = x + colW
		}
		mid := (x + next) / 2
		y := yOf(valueAt(float64(n-1) - float64((head-mid)/step)))
		p.RRect(geom.Rect{Min: geom.Pt(x, y), Max: geom.Pt(next, box.H)}, 0, paint.Fill{Gradient: grad})
		x = next
	}
	// The line, a short stroke between each point and the next.
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		d := b.Sub(a)
		length := float32(math.Hypot(float64(d.X), float64(d.Y)))
		if length <= 0 {
			continue
		}
		angle := float32(math.Atan2(float64(d.Y), float64(d.X)))
		func() {
			defer p.Push(paint.Rotate(angle, a))()
			p.RRect(geom.Rc(a.X, a.Y-0.75, length+0.5, 1.5), 0.75, paint.Solid(accent))
		}()
	}
	// The foot.
	foot := Ink.Get(th)
	foot.A = 0x20
	p.RRect(geom.Rc(0, box.H-1, box.W, 1), 0, paint.Solid(foot))
	// The head: a dot on the newest, and a ring breathing out of it
	// while the graph runs.
	headAt := pts[len(pts)-1]
	if g.running {
		pulse := float32(math.Mod(g.glow, 1.2) / 1.2)
		ring := accent
		ring.A = uint8(0x90 * (1 - pulse))
		r := 3 + 7*pulse
		p.RRect(geom.Rc(headAt.X-r, headAt.Y-r, 2*r, 2*r), r, paint.Solid(ring))
	}
	p.RRect(geom.Rc(headAt.X-3, headAt.Y-3, 6, 6), 3, paint.Solid(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}))
	p.RRect(geom.Rc(headAt.X-2, headAt.Y-2, 4, 4), 2, paint.Solid(accent))
	if g.Label != nil {
		// Over the plot, at the end the samples come in at.
		run := g.text.shape(faceIn(Font, th), g.Label(g.recent()), TextSize.Get(th)*0.85)
		ink := Ink.Get(th)
		ink.A = 0xc0
		run.Paint(p, geom.Pt(max(0, box.W-run.Advance), 0), ink)
	}
}

// recent returns the mean of the last second's samples, which holds
// still enough to read.
func (g *LiveGraph) recent() float64 {
	n := len(g.samples)
	if n == 0 {
		return 0
	}
	k := n
	if g.Every > 0 {
		k = min(n, max(1, int(time.Second/g.Every)))
	}
	sum := 0.0
	for _, v := range g.samples[n-k:] {
		sum += v
	}
	return sum / float64(k)
}
