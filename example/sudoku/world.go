package main

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// registerViews is the window half: the world's view, the map and the
// game in it.
func registerViews(w *gunim.Window, s *sfx) {
	gunim.RegisterView(w, "world",
		func(World) *worldRoot { return newWorldRoot(s) },
		func(r *worldRoot, s World, u *gunim.UI) { r.show(s, u) })
}

// worldRoot is the whole window: the map, or the game, and the zoom
// between them, into the level picked and back out of it.
type worldRoot struct {
	anim.Group
	state World
	m     *mapView
	game  *gameRoot
	// onMap is 1 with the map showing and 0 with the game; between,
	// the map zooms about at, the level going in or coming out.
	onMap *anim.Float
	at    geom.Point
	size  geom.Size
	// shown says the world has been shown once.
	shown bool
}

func newWorldRoot(s *sfx) *worldRoot {
	if s == nil {
		s = newSFX(nil, nil)
	}
	r := &worldRoot{onMap: anim.NewFloat(1)}
	r.Add(r.onMap)
	r.game = newGameRoot(s)
	r.game.world = r
	r.m = newMapView(r, s)
	return r
}

// show takes the world as it is now.
func (r *worldRoot) show(s World, u *gunim.UI) {
	was := r.state
	r.state = s
	if s.Game.Round != 0 {
		r.game.show(s.Game, u)
	}
	r.m.show(was, s)
	if s.Map && !was.Map {
		// Pac-Man stops eating as the game zooms away.
		r.game.board.sendPacmanAway()
	}
	if s.Sound != was.Sound || !r.shown {
		r.shown = true
		r.game.sfx.setMode(s.Sound)
	}
	to := float32(0)
	if s.Map {
		to = 1
	}
	if r.onMap.Target() != to {
		// The zoom is about the level going in, or coming out.
		l := s.Game.Level
		if l == 0 {
			l = s.Unlocked
		}
		r.m.reveal(l)
		r.at = r.m.nodeOnScreen(l)
		if s.Map {
			r.m.cameBack()
		}
		r.onMap.Animate(to, anim.Spring{Response: 0.55, Damping: 0.9})
	}
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (r *worldRoot) Children() []gunim.Node { return []gunim.Node{r.game, r.m} }

// Focusable implements [gunim.Focusable]: the window's keys come here.
func (r *worldRoot) Focusable() bool { return true }

// Handle implements [gunim.Handler]: keys go to the game while it shows,
// and Escape goes from it to the map.
func (r *worldRoot) Handle(e input.Event, u *gunim.UI) bool {
	switch e.(type) {
	case input.WindowHidden:
		// Gone to the background: the music and the sounds stop until
		// the game shows again.
		r.game.sfx.setAway(true)
		return true
	case input.WindowShown:
		r.game.sfx.setAway(false)
		return true
	}
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	if r.state.Map {
		return r.m.key(k, u)
	}
	if k.Key == input.KeyEscape && r.game.selected < 0 && r.game.armed == 0 {
		u.Send(r, ShowMap{})
		return true
	}
	return r.game.Handle(e, u)
}

// Layout implements [gunim.Node].
func (r *worldRoot) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	r.size = c.Max
	for k := range kids.All {
		k.Layout(gunim.Tight(c.Max))
		k.Place(geom.Point{})
	}
	return c.Max
}

// Paint implements [gunim.Node]: the game fading and growing in as the
// map zooms into the level and fades.
func (r *worldRoot) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	m := min(max(r.onMap.Value(), 0), 1)
	whole := geom.Rect{Max: box.Point()}
	if m < 0.999 && r.state.Game.Round != 0 {
		g := 1 - m
		mid := geom.Pt(box.W/2, box.H/2)
		func() {
			// A layer fades the game as one; at full strength it needs
			// none, and drawing offscreen costs the whole screen again.
			if g < 0.999 {
				end := p.Layer(paint.LayerOpts{Bounds: whole, Opacity: g})
				defer end()
			}
			defer p.Push(paint.Scale(0.85+0.15*g, mid))()
			kids.At(0).Paint(p)
		}()
	}
	if m > 0.001 {
		func() {
			if m < 0.999 {
				end := p.Layer(paint.LayerOpts{Bounds: whole, Opacity: m})
				defer end()
				defer p.Push(paint.Scale(1+2.2*(1-m), r.at))()
			}
			kids.At(1).Paint(p)
		}()
	}
}

// Step implements [gunim.Animator].
func (r *worldRoot) Step(dt time.Duration) bool { return r.Group.Step(dt) }
