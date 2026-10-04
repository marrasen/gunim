package main

import (
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// mapView is the level map: a path of candies winding up through
// lands of each difficulty, the levels won showing their stars, the
// locked ones grey, and a heart on the level to play next. It scrolls
// with the wheel and the finger.
type mapView struct {
	anim.Group
	root *worldRoot
	sfx  *sfx
	fx   *fx
	// top is the map's place at the window's top, in the map's space,
	// where level 1 is at the bottom.
	top    *anim.Float
	topSet bool
	// token is the level the heart stands on, a fraction between as it
	// hops.
	token *anim.Float
	// opening is a level being opened, and open how far: its lock
	// bursts and its candy grows in. hopWait holds the heart's hop
	// until the level is open.
	opening  int
	open     *anim.Float
	hopWait  float32
	openWait float32
	// press squashes the level pressed.
	pressed int
	press   *anim.Float
	pulse   float64
	size    geom.Size
}

// The map's spacing: between levels, and above and below them.
const (
	levelGap   = 112
	mapMargin  = 170
	nodeRadius = 34
)

func newMapView(r *worldRoot, s *sfx) *mapView {
	m := &mapView{root: r, sfx: s, fx: newFX(), top: anim.NewFloat(0), token: anim.NewFloat(1),
		open: anim.NewFloat(1), press: anim.NewFloat(0), pressed: -1}
	m.Add(m.top, m.token, m.open, m.press)
	return m
}

// height is the map's height.
func (m *mapView) height() float32 { return float32(levelCount-1)*levelGap + 2*mapMargin }

// node returns where level l's candy is, in the map's space.
func (m *mapView) node(l float32) geom.Point {
	amp := min(m.size.W*0.27, 150)
	x := m.size.W/2 + amp*float32(math.Sin(float64(l-1)*0.95))
	y := m.height() - mapMargin - (l-1)*levelGap
	return geom.Pt(x, y)
}

// nodeOnScreen returns where level l's candy is in the window.
func (m *mapView) nodeOnScreen(l int) geom.Point {
	return m.node(float32(l)).Sub(geom.Pt(0, m.top.Value()))
}

// show takes the world: a level newly opened opens on the map.
func (m *mapView) show(was, s World) {
	if !m.topSet || s.Unlock.ID == 0 && was.Unlocked != s.Unlocked {
		m.token.Jump(float32(s.Unlocked))
	}
	if s.Unlock.ID != was.Unlock.ID && s.Unlock.Level > 1 {
		// The heart waits on the level won while the next opens.
		m.opening = s.Unlock.Level
		m.open.Jump(0)
		m.token.Jump(float32(s.Unlock.Level - 1))
	}
}

// cameBack starts the opening of a level waiting to open, once the map
// has zoomed back out.
func (m *mapView) cameBack() {
	if m.opening > 0 && m.open.Value() < 1 {
		m.openWait = 0.7
	}
}

// reveal scrolls the map to show level l, at once.
func (m *mapView) reveal(l int) {
	if m.size.H == 0 {
		return
	}
	m.top.Jump(m.clampTop(m.node(float32(l)).Y - m.size.H*0.6))
	m.topSet = true
}

func (m *mapView) clampTop(t float32) float32 {
	return max(0, min(t, m.height()-m.size.H))
}

// Step implements [gunim.Animator].
func (m *mapView) Step(dt time.Duration) bool {
	s := float32(dt.Seconds())
	moving := false
	if m.openWait > 0 {
		moving = true
		m.openWait -= s
		if m.openWait <= 0 {
			m.open.Animate(1, anim.Spring{Response: 0.5, Damping: 0.45})
			at := m.nodeOnScreen(m.opening)
			m.fx.burst(at, 30, starBit, 420, gold, white, candyOf(int8((m.opening-1)%9+1)).color)
			m.fx.burst(at, 18, dot, 300, rgb(0x9a, 0x9a, 0xb0), white)
			m.sfx.unlock()
			m.hopWait = 0.7
		}
	}
	if m.hopWait > 0 {
		moving = true
		m.hopWait -= s
		if m.hopWait <= 0 {
			m.token.Animate(float32(m.opening), anim.Tween{Duration: 750 * time.Millisecond, Ease: anim.EaseInOut})
			m.sfx.hop()
			m.top.Animate(m.clampTop(m.node(float32(m.opening)).Y-m.size.H*0.6), anim.Spring{Response: 0.9, Damping: 1})
		}
	}
	m.pulse += dt.Seconds()
	m.fx.step(dt)
	if m.Group.Step(dt) {
		moving = true
	}
	// The heart bobs, and the next level breathes, while the map shows.
	return moving || m.root.state.Map
}

// levelAt returns the level whose candy is under p, in the window, or 0.
func (m *mapView) levelAt(p geom.Point) int {
	for l := 1; l <= levelCount; l++ {
		at := m.nodeOnScreen(l)
		d := at.Sub(p)
		if d.X*d.X+d.Y*d.Y <= (nodeRadius+8)*(nodeRadius+8) {
			return l
		}
	}
	return 0
}

// Covers implements [gunim.Shaped]: the map takes the pointer while it
// shows, and lets the game have it while that does.
func (m *mapView) Covers(geom.Point) bool { return m.root.state.Map }

// DragsTouch implements [gunim.TouchDragger]: a finger that moves
// scrolls the map, as the engine sends it scrolls.
func (m *mapView) DragsTouch() bool { return false }

// Handle implements [gunim.Handler].
func (m *mapView) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.Scroll:
		m.top.Jump(m.clampTop(m.top.Value() - e.Delta.Y))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		m.pressed = m.levelAt(e.Pos)
		if m.pressed == 0 || m.pressed > m.root.state.Unlocked {
			if m.pressed > m.root.state.Unlocked {
				// A locked level wobbles no.
				m.sfx.locked()
			}
			m.pressed = -1
			return true
		}
		m.press.Animate(1, anim.Spring{Response: 0.1, Damping: 1})
	case input.PointerUp:
		if m.pressed < 0 {
			return false
		}
		l := m.pressed
		m.pressed = -1
		m.press.Animate(0, anim.Spring{Response: 0.4, Damping: 0.35})
		if m.levelAt(e.Pos) == l {
			m.play(l, u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// play starts level l.
func (m *mapView) play(l int, u *gunim.UI) {
	m.sfx.pop(m.root.game.pan(m.nodeOnScreen(l)))
	u.Send(m.root, Start{Level: l})
}

func (m *mapView) key(k input.KeyPress, u *gunim.UI) bool {
	switch k.Key {
	case input.KeyEnter, input.KeyKPEnter, input.KeySpace:
		m.play(m.root.state.Unlocked, u)
	case input.KeyUp, input.KeyPageUp:
		m.top.Animate(m.clampTop(m.top.Target()-m.size.H*0.5), anim.Spring{Response: 0.4, Damping: 1})
	case input.KeyDown, input.KeyPageDown:
		m.top.Animate(m.clampTop(m.top.Target()+m.size.H*0.5), anim.Spring{Response: 0.4, Damping: 1})
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (m *mapView) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	first := m.size.H == 0
	m.size = c.Max
	if first || !m.topSet {
		m.reveal(m.root.state.Unlocked)
	}
	return c.Max
}

// lands are the map's lands: where each starts, its name, and its sky
// from top to bottom.
var lands = []struct {
	from     int
	name     string
	top, low color.NRGBA
}{
	{1, "Easy", rgb(0xff, 0x9e, 0xc7), rgb(0xff, 0xc8, 0xa8)},
	{7, "Medium", rgb(0x5f, 0xd8, 0xb8), rgb(0xa8, 0xf0, 0xc8)},
	{19, "Hard", rgb(0x5c, 0x9c, 0xff), rgb(0x8c, 0xd8, 0xff)},
	{37, "Expert", rgb(0x2a, 0x16, 0x6e), rgb(0x6a, 0x3c, 0xd9)},
}

// Paint implements [gunim.Node].
func (m *mapView) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	top := m.top.Value()
	whole := geom.Rect{Max: box.Point()}
	p.RRect(whole, 0, paint.Solid(lands[0].low))
	func() {
		defer p.Push(paint.Translate(geom.Pt(0, -top)))()
		m.paintLands(p, top, box)
	}()
	// Sweets hang at three depths, each scrolling at its own speed: far
	// ones slower than the path, near ones faster and over it.
	m.paintFar(p, top, box)
	m.paintSweets(p, top, box)
	func() {
		defer p.Push(paint.Translate(geom.Pt(0, -top)))()
		m.paintRibbons(p, top, box)
		m.paintPath(p, top, box)
		m.paintNodes(p, f, top, box)
		m.paintToken(p)
	}()
	m.paintNear(p, f, top, box)
	m.fx.paint(p)
	// The title, over the map, under a phone's status bar; its shade
	// runs up under the bar to the top.
	top, left, right := f.Safe.Top, f.Safe.Left, f.Safe.Right
	bar := geom.Rc(0, 0, box.W, top+64)
	p.RRect(bar, 0, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(0, 0), To: geom.Pt(0, top+64),
		Start: faded(plum, 0.75), End: faded(plum, 0)}})
	title := shaped("Candy Sudoku", 24, true)
	paintLabel(p, title, geom.Pt(left+18, top+16), white)
	total := 0
	for _, s := range m.root.state.Stars {
		total += s
	}
	n := shaped(strconv.Itoa(total), 18, true)
	pill := geom.Rc(box.W-right-n.Advance-62, top+14, n.Advance+46, 34)
	p.RRect(pill, 17, paint.Solid(faded(plum, 0.55)))
	p.Mask(candyMask{shape: shapeStar}, geom.Rc(pill.Min.X+6, pill.Min.Y+4, 26, 26), gold)
	n.Paint(p, geom.Pt(pill.Min.X+36, pill.Min.Y+7), white)
}

// paintLands draws each land's sky behind its levels, blending into
// the next.
func (m *mapView) paintLands(p *paint.Painter, top float32, box geom.Size) {
	for i, land := range lands {
		start := m.node(float32(land.from)).Y + levelGap/2
		if i == 0 {
			start = m.height()
		}
		end := float32(0)
		if i+1 < len(lands) {
			end = m.node(float32(lands[i+1].from)).Y + levelGap/2
		}
		r := geom.Rc(0, end, box.W, start-end)
		if r.Max.Y < top || r.Min.Y > top+box.H {
			continue
		}
		p.RRect(r, 0, paint.Fill{Gradient: &paint.Gradient{
			From: geom.Pt(0, r.Min.Y), To: geom.Pt(0, r.Max.Y),
			Start: land.top, End: land.low,
		}})
	}
	// Each land fades into the next over two levels.
	for i := 1; i < len(lands); i++ {
		at := m.node(float32(lands[i].from)).Y + levelGap/2
		r := geom.Rc(0, at-levelGap, box.W, 2*levelGap)
		if r.Max.Y < top || r.Min.Y > top+box.H {
			continue
		}
		p.RRect(r, 0, paint.Fill{Gradient: &paint.Gradient{
			From: geom.Pt(0, r.Min.Y), To: geom.Pt(0, r.Max.Y),
			Start: lands[i].low, End: lands[i-1].top,
		}})
	}
}

// paintRibbons draws each land's name on a ribbon at its start.
func (m *mapView) paintRibbons(p *paint.Painter, top float32, box geom.Size) {
	for _, land := range lands {
		// The land's name on a ribbon at its start.
		at := m.node(float32(land.from))
		name := shaped(land.name, 16, true)
		rb := geom.Rc(box.W/2-name.Advance/2-18, at.Y+levelGap*0.55, name.Advance+36, 30)
		p.ShadowRRect(rb, 15, paint.Solid(faded(plum, 0.8)), paint.Shadow{Blur: 8, Offset: geom.Pt(0, 3), Color: faded(plum, 0.3)})
		name.Paint(p, geom.Pt(rb.Min.X+18, rb.Min.Y+6), white)
	}
}

// layer calls draw for each thing a layer of the map holds, gap apart,
// with where it is in the window: the layer scrolls at depth times the
// map's speed, its bottom with the map's bottom.
func (m *mapView) layer(top, depth, gap float32, box geom.Size, draw func(i int, y float32)) {
	h := (m.height()-box.H)*depth + box.H
	for i := 0; ; i++ {
		y := h - float32(i)*gap - gap/2
		if y < -gap {
			return
		}
		if at := y - top*depth; at > -160 && at < box.H+160 {
			draw(i, at)
		}
	}
}

// paintFar draws the layer furthest back: big pale candy shapes,
// scrolling at under half the path's speed.
func (m *mapView) paintFar(p *paint.Painter, top float32, box geom.Size) {
	m.layer(top, 0.4, 130, box, func(i int, y float32) {
		f := float32(i)
		x := box.W * float32(math.Mod(float64(f)*0.618+0.1, 1))
		size := 50 + 80*float32(math.Mod(float64(f)*0.37, 1))
		// Every size shares one mask, scaled.
		shapeAt(p, i%9, geom.Rc(x-20, y-20, 40, 40), size/40, geom.Point{}, faded(white, 0.13))
	})
}

// paintSweets draws the lollipops and candies along the edges, a few
// each screen, scrolling a little slower than the path.
func (m *mapView) paintSweets(p *paint.Painter, top float32, box geom.Size) {
	m.layer(top, 0.75, levelGap*0.9, box, func(i int, y float32) {
		side := float32(1)
		if i%2 == 0 {
			side = -1
		}
		x := box.W/2 + side*(box.W*0.5-28-12*float32(i%3))
		k := candyOf(int8(i%9 + 1))
		if i%3 == 0 {
			// A lollipop: a stick and a round candy.
			p.RRect(geom.Rc(x-2, y, 4, 46), 2, paint.Solid(faded(white, 0.7)))
			p.Mask(candyMask{shape: shapeCircle}, geom.Rc(x-20, y-36, 40, 40), faded(k.color, 0.75))
			shapeAt(p, shapeCircle, geom.Rc(x-20, y-36, 40, 40), 0.6, geom.Point{}, faded(white, 0.4))
			return
		}
		p.Mask(candyMask{shape: k.shape}, geom.Rc(x-14, y-14, 28, 28), faded(k.color, 0.45))
	})
}

// paintNear draws the layer nearest: big candies half off the edges,
// over everything, scrolling faster than the path.
func (m *mapView) paintNear(p *paint.Painter, f gunim.Frame, top float32, box geom.Size) {
	m.layer(top, 1.6, 340, box, func(i int, y float32) {
		x := float32(-8)
		if i%2 == 1 {
			x = box.W + 8
		}
		const s = 96
		r := geom.Rc(x-s/2, y-s/2, s, s)
		defer p.Push(paint.Rotate(0.4*float32(i%3-1), geom.Pt(x, y)))()
		paintCandy(p, int8((i*4)%9+1), r, 0.7, false, f.Scale)
	})
}

// paintPath draws the dotted path between the levels, gold as far as
// the levels open.
func (m *mapView) paintPath(p *paint.Painter, top float32, box geom.Size) {
	open := float32(m.root.state.Unlocked)
	for l := float32(1); l < levelCount; l += 0.1 {
		at := m.node(l)
		if at.Y < top-20 || at.Y > top+box.H+20 {
			continue
		}
		// The dots stop short of each level's candy and its stars.
		if near := l - float32(math.Round(float64(l))); near > -0.38 && near < 0.38 {
			continue
		}
		c := faded(white, 0.55)
		r := float32(4)
		if l < open {
			c, r = gold, 5
		}
		p.RRect(geom.Rc(at.X-r, at.Y-r+3, 2*r, 2*r), r, paint.Solid(faded(plum, 0.2)))
		p.RRect(geom.Rc(at.X-r, at.Y-r, 2*r, 2*r), r, paint.Solid(c))
	}
}

// paintNodes draws each level's candy, its number and its stars.
func (m *mapView) paintNodes(p *paint.Painter, f gunim.Frame, top float32, box geom.Size) {
	s := m.root.state
	for l := 1; l <= levelCount; l++ {
		at := m.node(float32(l))
		if at.Y < top-80 || at.Y > top+box.H+80 {
			continue
		}
		d := int8((l-1)%9 + 1)
		r := geom.Rc(at.X-nodeRadius, at.Y-nodeRadius, 2*nodeRadius, 2*nodeRadius)
		scale := float32(1)
		if l == s.Unlocked {
			scale += 0.06 * float32(math.Sin(m.pulse*3))
			p.ShadowRRect(geom.Rc(at.X-nodeRadius*0.8, at.Y-nodeRadius*0.8, nodeRadius*1.6, nodeRadius*1.6), nodeRadius,
				paint.Solid(faded(white, 0)), paint.Shadow{Blur: 26, Color: faded(rgb(0xff, 0xf3, 0x9a), 0.9)})
		}
		if l == m.pressed {
			scale *= 1 - 0.12*m.press.Value()
		}
		locked := l > s.Unlocked
		opening := l == m.opening && m.open.Value() < 1
		func() {
			defer p.Push(paint.Scale(scale, at))()
			if locked || opening {
				// A locked level: a grey candy and a lock. One opening
				// grows its colours in over it.
				p.Mask(candyMask{shape: candyOf(d).shape}, r.Add(geom.Pt(0, 4)), faded(plum, 0.25))
				p.Mask(candyMask{shape: candyOf(d).shape}, r, rgb(0xb8, 0xb4, 0xc8))
				shapeAt(p, candyOf(d).shape, r, (r.Size().W-14)/r.Size().W, geom.Pt(0, -2), rgb(0xd4, 0xd0, 0xe0))
				if !opening {
					widget.PaintIcon(p, f.Theme, icon.Lock, geom.Rc(at.X-11, at.Y-11, 22, 22), rgb(0x6a, 0x66, 0x80))
					return
				}
			}
			o := float32(1)
			if opening {
				o = m.open.Value()
			}
			func() {
				defer p.Push(paint.Scale(o, at))()
				paintCandy(p, d, r, min(1, o*1.5), false, f.Scale)
				n := shaped(strconv.Itoa(l), 22, true)
				paintLabel(p, n, geom.Pt(at.X-n.Advance/2, at.Y-13), white)
			}()
		}()
		// The stars won, under the candy.
		if !locked {
			for i := range 3 {
				sr := geom.Rc(at.X-30+float32(i)*20-9, at.Y+nodeRadius-4, 18, 18)
				if i == 1 {
					sr = sr.Add(geom.Pt(0, 4))
				}
				c := faded(white, 0.45)
				if i < s.Stars[l-1] {
					c = gold
				}
				p.Mask(candyMask{shape: shapeStar}, sr.Add(geom.Pt(0, 1.5)), faded(plum, 0.3))
				p.Mask(candyMask{shape: shapeStar}, sr, c)
			}
		}
	}
}

// paintToken draws the heart on its level: bobbing, or hopping along
// the path to the next.
func (m *mapView) paintToken(p *paint.Painter) {
	l := m.token.Value()
	at := m.node(l)
	frac := l - float32(math.Floor(float64(l)))
	hop := float32(math.Sin(math.Pi*float64(frac))) * 46
	bob := float32(math.Abs(math.Sin(m.pulse*3.2))) * 6
	y := at.Y - nodeRadius - 30 - hop - bob
	r := geom.Rc(at.X-20, y-20, 40, 40)
	p.RRect(geom.Rc(at.X-14, at.Y-nodeRadius-6, 28, 8), 4, paint.Solid(faded(plum, 0.25)))
	shapeAt(p, shapeHeart, r, 1.1, geom.Point{}, white)
	p.Mask(candyMask{shape: shapeHeart}, r, rgb(0xff, 0x3b, 0x6b))
	shapeAt(p, shapeHeart, r, 0.7, geom.Pt(0, -2), rgb(0xff, 0x80, 0xa0))
}
