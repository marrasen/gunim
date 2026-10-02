package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Echo tokens: the tones a ping comes in, how strong every ring is, and
// how far past the window's edges the rings travel. A strength of 0
// turns echoes off; 2 draws them twice as strong.
var (
	EchoProblem  = theme.Color("echo.problem", color.NRGBA{R: 0xff, G: 0x4d, B: 0x4d, A: 0xff})
	EchoDone     = theme.Color("echo.done", color.NRGBA{R: 0x3d, G: 0xe0, B: 0x7a, A: 0xff})
	EchoCall     = theme.Color("echo.call", color.NRGBA{R: 0xff, G: 0xb3, B: 0x2e, A: 0xff})
	EchoWait     = theme.Color("echo.wait", color.NRGBA{R: 0xa0, G: 0xa6, B: 0xb4, A: 0xff})
	EchoStrength = theme.Number("echo.strength", 1)
	EchoReach    = theme.Length("echo.reach", 64)
)

// Echo sends rings out from the window's edges onto the desktop around
// it, like a sonar's ping: a flash along the edge, then three rings
// that grow outwards, thin and fade. EchoProblem for a failure and
// EchoDone for a success say so past the window's own content, and
// EchoCall asks for the user. While something is on its way, Wait sends
// out one faint ring after another, like a phone's dial tone.
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
	// Radius is the window's corner radius, which the rings follow;
	// zero takes 8, the round corners of a window on Windows 11.
	Radius float32

	pop  *gunim.Popup
	view *echoView
	// waiting is set while Wait's rings go out, and beating while the
	// timer sending them runs; tone is their colour.
	waiting, beating bool
	tone             theme.Token[color.NRGBA]
}

// echoLife is how long a ring takes to reach its end, echoFlash how
// long the flash along the edge lasts, and echoBeat how often Wait
// sends a ring.
const (
	echoLife  = 1100 * time.Millisecond
	echoFlash = 420 * time.Millisecond
	echoGlow  = 700 * time.Millisecond
	echoBeat  = 1400 * time.Millisecond
)

// Ping sends a ping out, in tone: EchoProblem, EchoDone, EchoCall, or
// a token of the application's own. A ping while the last is still
// travelling joins it, in the same popup.
func (e *Echo) Ping(u *gunim.UI, tone theme.Token[color.NRGBA]) {
	e.send(u,
		echoRing{tone: tone, strength: 1, flash: true},
		echoRing{tone: tone, strength: 1},
		echoRing{at: 140 * time.Millisecond, tone: tone, strength: 0.7},
		echoRing{at: 280 * time.Millisecond, tone: tone, strength: 0.45},
	)
}

// Glow lights the window's edges in tone, softly, and lets them go
// dark again: a quieter sign than Ping, for something that happened in
// the window the user is looking at, such as a bell in the terminal in
// front.
func (e *Echo) Glow(u *gunim.UI, tone theme.Token[color.NRGBA]) {
	e.send(u, echoRing{tone: tone, strength: 1, glow: true})
}

// Wait sends one faint ring after another in tone, EchoWait usually,
// while on is set, as it is while a connection is being made. Setting
// it again with on changes the tone.
func (e *Echo) Wait(u *gunim.UI, tone theme.Token[color.NRGBA], on bool) {
	e.waiting, e.tone = on, tone
	if on && !e.beating {
		e.beating = true
		e.beat(u)
	}
}

// beat sends Wait's ring, and the next one a beat later, until Wait is
// turned off.
func (e *Echo) beat(u *gunim.UI) {
	if !e.waiting {
		e.beating = false
		return
	}
	e.send(u, echoRing{tone: e.tone, strength: 0.35})
	u.After(echoBeat, e.beat)
}

// send sends rings out, each r.at after now.
func (e *Echo) send(u *gunim.UI, rings ...echoRing) {
	th := u.Theme()
	if EchoStrength.Get(th) <= 0 || !u.Blends() || u.Maximized() || u.FullScreen() {
		return
	}
	win, ok := u.Bounds(u.Root())
	if !ok || win.Empty() {
		return
	}
	reach, radius := max(EchoReach.Get(th), 0), e.Radius
	if radius <= 0 {
		radius = 8
	}
	// The popup reaches past the rings' last, widest glow.
	m := reach + 16
	if e.pop == nil || !e.pop.Open() {
		e.view = &echoView{}
		e.pop = u.OpenPopup(u.Root(), e.view, gunim.PopupOptions{Passthrough: true, Over: true, Owned: true})
		e.close(u)
	}
	v := e.view
	v.size = geom.Sz(win.Size().W+2*m, win.Size().H+2*m)
	v.inner = geom.Rc(m, m, win.Size().W, win.Size().H)
	v.reach, v.radius = reach, radius
	e.pop.Move(geom.Rect{Min: geom.Pt(win.Min.X-m, win.Min.Y-m), Max: geom.Pt(win.Max.X+m, win.Max.Y+m)})
	for _, r := range rings {
		r.at += v.age
		v.rings = append(v.rings, r)
	}
	u.Invalidate()
}

// close closes the popup once its rings have all travelled and Wait is
// off.
func (e *Echo) close(u *gunim.UI) {
	pop, v := e.pop, e.view
	u.After(echoLife, func(u *gunim.UI) {
		if len(v.rings) > 0 || e.waiting && e.pop == pop {
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

// echoRing is one ring, or the flash along the edge. Its colour is
// looked up as it paints, so it follows a theme switch.
type echoRing struct {
	at       time.Duration
	tone     theme.Token[color.NRGBA]
	strength float32
	flash    bool
	// glow lights the edges and lets them go dark, travelling nowhere.
	glow bool
}

// life is how long r shows.
func (r echoRing) life() time.Duration {
	switch {
	case r.flash:
		return echoFlash
	case r.glow:
		return echoGlow
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
func (v *echoView) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	strength := EchoStrength.Get(f.Theme)
	for _, r := range v.rings {
		age := v.age - r.at
		if age < 0 {
			continue
		}
		c := r.tone.Get(f.Theme)
		t := float32(age) / float32(r.life())
		fade := (1 - t) * (1 - t) * r.strength * strength
		if r.glow {
			// Up quickly, and down slowly, as a breath.
			rise := float32(math.Sin(math.Pi * math.Pow(float64(t), 0.6)))
			v.glow(p, c, rise*r.strength*strength)
			continue
		}
		if r.flash {
			v.ring(p, 1+5*t, 3, c, 0.8*fade)
			continue
		}
		w := 1 + 4*(1-t)
		travel := 1 - float32(math.Pow(float64(1-t), 3))
		v.ring(p, 2*w+1+travel*v.reach, w, c, fade)
	}
}

// glow paints a soft light round the window's edges, strongest at the
// edge and fading out over glowReach, like a shadow in colour.
func (v *echoView) glow(p *paint.Painter, c color.NRGBA, alpha float32) {
	const step = 1.5
	for d := float32(step / 2); d < glowReach; d += step {
		x := 1 - d/glowReach
		a := c
		a.A = uint8(float32(c.A) * min(1, max(0, alpha*0.55*x*x)))
		rect := geom.Rect{Min: geom.Pt(v.inner.Min.X-d, v.inner.Min.Y-d), Max: geom.Pt(v.inner.Max.X+d, v.inner.Max.Y+d)}
		p.RRectStroke(rect, v.radius+d, paint.Fill{}, paint.Stroke{Width: step, Color: a})
	}
}

// glowReach is how far past the window's edges a glow shows.
const glowReach = 22

// ring paints a ring d past the window's edges, w wide, with its glow.
func (v *echoView) ring(p *paint.Painter, d, w float32, c color.NRGBA, alpha float32) {
	d = max(d, 2*w)
	rect := geom.Rect{Min: geom.Pt(v.inner.Min.X-d, v.inner.Min.Y-d), Max: geom.Pt(v.inner.Max.X+d, v.inner.Max.Y+d)}
	for _, l := range [...]struct{ w, a float32 }{{4 * w, 0.14}, {2 * w, 0.3}, {w, 1}} {
		a := c
		a.A = uint8(float32(c.A) * min(1, max(0, alpha*l.a)))
		p.RRectStroke(rect, v.radius+d, paint.Fill{}, paint.Stroke{Width: l.w, Color: a})
	}
}
