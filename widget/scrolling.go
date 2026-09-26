package widget

import (
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// barLinger is how long the scrollbar stays after the content stops
// moving, before it fades.
const barLinger = 800 * time.Millisecond

// flingDecay is how a fling slows: its velocity falls to a third in a
// third of a second, much as a finger's fling does on a phone.
var flingDecay = anim.Decay{Tau: 0.33}

// scrolling is the scrolling a [Scroll] and a [VirtualList] share: the
// offset, the wheel and the keys, dragging and flinging, the stretch
// at either end, and the bar.
//
// The wheel and the keys set a target, and a spring carries the
// content there. A drag moves the content with the pointer, and a
// release flings it on at the pointer's speed, slowing to a stop. Past
// either end a drag stretches, with less give the further it goes, and
// the content springs back on release; a fling that runs past an end
// carries on a little and springs back the same way.
type scrolling struct {
	// DragScroll lets the pointer drag the content and fling it. A
	// press the content's own nodes take, such as a button's, is
	// theirs.
	DragScroll bool

	offset *anim.Float
	target float32
	bar    *anim.Float
	idle   time.Duration
	// th is the window's live theme, kept from Layout for Step, which
	// has no frame. It is a handle to the live values, never a copy.
	th *theme.Live

	content, viewport float32

	// held is set while the pointer drags the content. grabY and
	// grabAt are where the drag started and the offset then, and
	// trail the latest pointer positions, for the fling's speed.
	held   bool
	grabY  float32
	grabAt float32
	trail  [8]sample
	trails int
	// flinging is set while the content coasts from a fling.
	flinging bool
}

type sample struct {
	y float32
	t time.Time
}

func newScrolling() scrolling {
	return scrolling{offset: anim.NewFloat(0), bar: anim.NewFloat(0)}
}

// Offset returns how far the content is scrolled right now.
func (s *scrolling) Offset() float32 { return s.offset.Value() }

// ScrollTo sets the target offset, clamped to the content, and carries
// the content there with motion.
func (s *scrolling) ScrollTo(y float32, motion anim.Motion) {
	s.flinging = false
	s.target = s.clamp(y)
	s.offset.Retarget(s.target, motion)
	s.showBar()
}

func (s *scrolling) showBar() {
	s.bar.Animate(1, Quick.Get(s.th))
	s.idle = 0
}

// end returns the largest offset.
func (s *scrolling) end() float32 { return max(0, s.content-s.viewport) }

func (s *scrolling) clamp(y float32) float32 { return max(0, min(y, s.end())) }

// stretch returns the offset a drag to y shows: y itself inside the
// content, and past an end, a part of the way there, less the further
// it goes.
func (s *scrolling) stretch(y float32) float32 {
	give := func(over float32) float32 {
		d := max(s.viewport, 1) * 0.55
		return d * (1 - 1/(over/d+1))
	}
	switch {
	case y < 0:
		return -give(-y)
	case y > s.end():
		return s.end() + give(y-s.end())
	}
	return y
}

// Step implements [gunim.Animator] for the widget. A fling that runs
// past an end springs back to it, keeping its speed, and once the
// content has rested for a moment the bar fades.
func (s *scrolling) Step(dt time.Duration) bool {
	moving := s.offset.Step(dt)
	if s.flinging {
		at := s.offset.Value()
		switch {
		case !moving:
			s.flinging = false
			s.target = s.clamp(at)
		case at < 0 || at > s.end():
			s.flinging = false
			s.target = s.clamp(at)
			s.offset.Retarget(s.target, Settle.Get(s.th))
		}
	}
	if moving || s.held {
		s.idle = 0
	} else if s.bar.Target() > 0 {
		s.idle += dt
		if s.idle >= barLinger {
			s.bar.Animate(0, Settle.Get(s.th))
		}
	}
	barMoving := s.bar.Step(dt)
	return moving || barMoving || s.bar.Target() > 0
}

// handle takes the wheel, the keys, and, with DragScroll, drags.
func (s *scrolling) handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	var to float32
	switch e := e.(type) {
	case input.PointerDown:
		if !s.DragScroll || e.Button != input.ButtonPrimary {
			return false
		}
		s.held, s.flinging = true, false
		s.grabY, s.grabAt = e.Pos.Y, s.offset.Value()
		s.trails = 0
		s.track(e.Pos.Y, e.Time)
		s.offset.Jump(s.grabAt)
		s.showBar()
		return true
	case input.PointerMove:
		if !s.held {
			return false
		}
		s.track(e.Pos.Y, e.Time)
		s.offset.Jump(s.stretch(s.grabAt + s.grabY - e.Pos.Y))
		s.showBar()
		u.Invalidate()
		return true
	case input.PointerUp:
		if !s.held {
			return false
		}
		s.held = false
		s.release(th)
		u.Invalidate()
		return true
	case input.Scroll:
		to = s.base() - e.Delta.Y
	case input.KeyPress:
		switch e.Key {
		case input.KeyUp:
			to = s.base() - ScrollLine.Get(th)
		case input.KeyDown:
			to = s.base() + ScrollLine.Get(th)
		case input.KeyPageUp:
			to = s.base() - s.viewport*0.9
		case input.KeyPageDown:
			to = s.base() + s.viewport*0.9
		case input.KeyHome:
			to = 0
		case input.KeyEnd:
			to = s.content
		default:
			return false
		}
	default:
		return false
	}
	// At an end, the event is for whatever scrolls outside.
	if s.clamp(to) == s.base() {
		return false
	}
	s.ScrollTo(to, Quick.Get(th))
	u.Invalidate()
	return true
}

// A drag held near an edge scrolls, faster the nearer it is: from
// nothing at edgeZone inside, to edgeSpeed at the edge, and on to four
// times that as far again past it.
const (
	edgeZone  = 40
	edgeSpeed = 700
)

// edgeScroll scrolls for a drag held at p, in the widget's space, over
// a frame of dt, and returns how far the content moved.
func (s *scrolling) edgeScroll(p geom.Point, dt time.Duration, u *gunim.UI) geom.Point {
	if s.held || s.end() <= 0 {
		return geom.Point{}
	}
	zone := min(float32(edgeZone), s.viewport/4)
	var v float32
	switch {
	case p.Y < zone:
		t := min((zone-p.Y)/zone, 2)
		v = -edgeSpeed * t * t
	case p.Y > s.viewport-zone:
		t := min((p.Y-s.viewport+zone)/zone, 2)
		v = edgeSpeed * t * t
	default:
		return geom.Point{}
	}
	// A frame after a pause covers the pause; the scroll picks up from
	// where it was.
	secs := float32(min(dt, 50*time.Millisecond).Seconds())
	from := s.offset.Value()
	to := s.clamp(from + v*secs)
	if to == from {
		return geom.Point{}
	}
	s.flinging = false
	s.target = to
	s.offset.Jump(to)
	s.showBar()
	u.Invalidate()
	return geom.Pt(0, from-to)
}

// base is the offset the wheel and the keys count from: the target, or
// where a fling has got to.
func (s *scrolling) base() float32 {
	if s.flinging {
		return s.clamp(s.offset.Value())
	}
	return s.target
}

// track records where the pointer is, for the fling's speed.
func (s *scrolling) track(y float32, t time.Time) {
	if t.IsZero() {
		t = time.Now()
	}
	copy(s.trail[1:], s.trail[:len(s.trail)-1])
	s.trail[0] = sample{y, t}
	s.trails = min(s.trails+1, len(s.trail))
}

// speed returns how fast the pointer moved over the last tenth of a
// second, in pixels a second down the screen, or zero when it had come
// to rest before letting go.
func (s *scrolling) speed() float32 {
	if s.trails < 2 {
		return 0
	}
	last := s.trail[0]
	first := last
	for _, p := range s.trail[1:s.trails] {
		if last.t.Sub(p.t) > 100*time.Millisecond {
			break
		}
		first = p
	}
	dt := last.t.Sub(first.t).Seconds()
	if dt <= 0 || time.Since(last.t) > 100*time.Millisecond {
		return 0
	}
	return float32(float64(last.y-first.y) / dt)
}

// release ends a drag: flinging on at the pointer's speed, or springing
// back from past an end.
func (s *scrolling) release(th *theme.Live) {
	at := s.offset.Value()
	v := -s.speed()
	if at < 0 || at > s.end() || math.Abs(float64(v)) < 50 {
		s.target = s.clamp(at)
		s.offset.Retarget(s.target, Settle.Get(th))
		return
	}
	s.flinging = true
	anim.Fling(s.offset, v, flingDecay)
}

// shift moves the content by d along with everything aimed at it: the
// target, a drag's grip and a motion in flight. It is for content
// that moved under the view, such as rows measured above it, and
// leaves what is on screen where it is.
func (s *scrolling) shift(d float32) {
	s.target += d
	s.grabAt += d
	anim.Shift(s.offset, d)
}

// reveal scrolls just far enough to bring r, in the widget's own
// space, fully into view, with a little room around it.
func (s *scrolling) reveal(r geom.Rect, u *gunim.UI) {
	const room = 8
	offset := s.offset.Value()
	switch {
	case r.Min.Y < 0:
		s.ScrollTo(offset+r.Min.Y-room, Quick.Get(u.Theme()))
	case r.Max.Y > s.viewport:
		s.ScrollTo(offset+r.Max.Y-s.viewport+room, Quick.Get(u.Theme()))
	}
}

// revealContent is reveal for r in the content's space, measured from
// its top rather than from the top of the view.
func (s *scrolling) revealContent(r geom.Rect, u *gunim.UI) {
	off := s.offset.Value()
	s.reveal(geom.Rect{Min: geom.Pt(r.Min.X, r.Min.Y-off), Max: geom.Pt(r.Max.X, r.Max.Y-off)}, u)
}

// fit takes the content's height and the view's from a layout, and
// brings the offset back inside when the content has shrunk under it.
func (s *scrolling) fit(content, viewport float32, th *theme.Live) {
	s.th = th
	s.content, s.viewport = content, viewport
	if s.held || s.flinging {
		return
	}
	if t := s.clamp(s.target); t != s.target {
		s.target = t
		s.offset.Animate(t, Settle.Get(th))
	}
}

// paintBar draws the bar, fading with the content's motion.
func (s *scrolling) paintBar(p *paint.Painter, f gunim.Frame, box geom.Size) {
	t := s.bar.Value()
	if t <= 0 || s.content <= s.viewport {
		return
	}
	w := ScrollbarWidth.Get(f.Theme)
	length := max(box.H*s.viewport/s.content, 2*w)
	at := max(0, min(s.offset.Value(), s.end()))
	top := (box.H - length) * at / s.end()
	col := ScrollbarColor.Get(f.Theme)
	col.A = uint8(float32(col.A) * min(t, 1))
	p.RRect(geom.Rc(box.W-w-2, top, w, length), w/2, paint.Solid(col))
}
