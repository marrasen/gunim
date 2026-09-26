package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Echo sends rings out from the window's edges onto the desktop around
// it, like a sonar's ping: a flash along the edge, then three rings
// that grow outwards, thin and fade. Red for a mistake and green for a
// success say so past the window's own content.
//
// The rings are drawn in a popup laid over the window, larger than it,
// which lets the pointer through and paints nothing over the window
// itself. Its edges are the window's content's, so it suits a
// chromeless window, whose content reaches its edges.
//
// A ping shows where the display server blends windows, and while the
// window is maximized or full screen it is skipped: the rings would
// reach onto the monitor beside the window.
type Echo struct {
	// Reach is how far past the window's edges the rings travel; zero
	// takes 64.
	Reach float32
	// Radius is the window's corner radius, which the rings follow;
	// zero takes 8, the round corners of a window on Windows 11.
	Radius float32

	pop  *gunim.Popup
	view *echoView
}

// echoLife is how long a ring takes to reach its end, and echoFlash
// how long the flash along the edge lasts.
const (
	echoLife  = 1100 * time.Millisecond
	echoFlash = 420 * time.Millisecond
)

// Ping sends a ping out, in colour c. A ping while the last is still
// travelling joins it, in the same popup.
func (e *Echo) Ping(u *gunim.UI, c color.NRGBA) {
	if !u.Blends() || u.Maximized() || u.FullScreen() {
		return
	}
	win, ok := u.Bounds(u.Root())
	if !ok || win.Empty() {
		return
	}
	reach, radius := e.Reach, e.Radius
	if reach <= 0 {
		reach = 64
	}
	if radius <= 0 {
		radius = 8
	}
	// The popup reaches past the rings' last, widest glow.
	m := reach + 16
	if e.pop == nil || !e.pop.Open() {
		e.view = &echoView{}
		e.pop = u.OpenPopup(u.Root(), e.view, gunim.PopupOptions{Passthrough: true, Over: true})
		e.close(u)
	}
	v := e.view
	v.size = geom.Sz(win.Size().W+2*m, win.Size().H+2*m)
	v.inner = geom.Rc(m, m, win.Size().W, win.Size().H)
	v.reach, v.radius = reach, radius
	e.pop.Move(geom.Rect{Min: geom.Pt(win.Min.X-m, win.Min.Y-m), Max: geom.Pt(win.Max.X+m, win.Max.Y+m)})
	v.rings = append(v.rings,
		echoRing{at: v.age, c: c, strength: 1, flash: true},
		echoRing{at: v.age, c: c, strength: 1},
		echoRing{at: v.age + 140*time.Millisecond, c: c, strength: 0.7},
		echoRing{at: v.age + 280*time.Millisecond, c: c, strength: 0.45},
	)
	u.Invalidate()
}

// close closes the popup once its rings have all travelled.
func (e *Echo) close(u *gunim.UI) {
	pop, v := e.pop, e.view
	u.After(echoLife, func(u *gunim.UI) {
		if len(v.rings) > 0 {
			e.close(u)
			return
		}
		pop.Close()
	})
}

// echoView is the popup's content: the rings, around the window.
type echoView struct {
	// size is the popup's, and inner the window's place in it.
	size          geom.Size
	inner         geom.Rect
	reach, radius float32
	// age is how long the view has shown, and rings the rings still
	// travelling, each started at an age.
	age   time.Duration
	rings []echoRing
}

// echoRing is one ring, or the flash along the edge.
type echoRing struct {
	at       time.Duration
	c        color.NRGBA
	strength float32
	flash    bool
}

// life is how long r shows.
func (r echoRing) life() time.Duration {
	if r.flash {
		return echoFlash
	}
	return echoLife
}

// Step implements [gunim.Animator].
func (v *echoView) Step(dt time.Duration) bool {
	v.age += dt
	live := v.rings[:0]
	for _, r := range v.rings {
		if v.age-r.at < r.life() {
			live = append(live, r)
		}
	}
	v.rings = live
	return len(v.rings) > 0
}

// Layout implements [gunim.Node].
func (v *echoView) Layout(gunim.Constraints, gunim.Frame, gunim.Children) geom.Size { return v.size }

// Paint implements [gunim.Node]. Each ring is a stroke with two wider,
// fainter ones under it, for a glow, all kept outside the window.
func (v *echoView) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, _ gunim.Children) {
	for _, r := range v.rings {
		age := v.age - r.at
		if age < 0 {
			continue
		}
		t := float32(age) / float32(r.life())
		fade := (1 - t) * (1 - t) * r.strength
		if r.flash {
			v.ring(p, 1+5*t, 3, r.c, 0.8*fade)
			continue
		}
		w := 1 + 4*(1-t)
		travel := 1 - float32(math.Pow(float64(1-t), 3))
		v.ring(p, 2*w+1+travel*v.reach, w, r.c, fade)
	}
}

// ring paints a ring d past the window's edges, w wide, with its glow.
func (v *echoView) ring(p *paint.Painter, d, w float32, c color.NRGBA, alpha float32) {
	d = max(d, 2*w)
	rect := geom.Rect{Min: geom.Pt(v.inner.Min.X-d, v.inner.Min.Y-d), Max: geom.Pt(v.inner.Max.X+d, v.inner.Max.Y+d)}
	for _, l := range [...]struct{ w, a float32 }{{4 * w, 0.14}, {2 * w, 0.3}, {w, 1}} {
		a := c
		a.A = uint8(float32(c.A) * min(1, alpha*l.a))
		p.RRectStroke(rect, v.radius+d, paint.Fill{}, paint.Stroke{Width: l.w, Color: a})
	}
}
