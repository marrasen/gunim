// Package gunim is an animation-first user interface framework.
//
// Three ideas hold it together.
//
// First, the engine owns a presentation tree and runs each node through
// a lifecycle of its own. Remove a dialog and it moves to [Exiting],
// animates itself out, and the engine unlinks it once it reports that
// it has finished. That makes "fade and scale the dialog away when OK
// is clicked" a three-line widget.
//
// Second, animation runs on wall-clock time. A 60 Hz laptop panel, a
// 144 Hz monitor and a frame the compositor dropped all produce the
// same motion.
//
// Third, a window draws while something moves or someone types, and
// sleeps the rest of the time, so an idle interface leaves the GPU
// alone.
//
// Each window has its own goroutine, and that goroutine is the only
// thing that touches nodes. Application code reaches it through
// [Window.Client] and hears back on [Client.Intents].
package gunim

import (
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// A Node is one element of the presentation tree.
//
// These two methods are the whole required interface. Three further
// abilities — handling input, animating, transitioning in and out —
// come from optional interfaces a node may also implement. A static
// label gets by with Node alone.
//
// Node methods run on the window's UI goroutine.
type Node interface {
	// Layout measures this node within c and positions its children,
	// returning the size it wants. It calls Layout and Place on every
	// child it intends to paint.
	Layout(c Constraints, f Frame, kids Children) geom.Size

	// Paint records this node's appearance and paints the children it
	// wants drawn. The painter's origin is this node's top-left
	// corner and box is the size Layout settled on, so each pass hands
	// the node the geometry it needs.
	Paint(p *paint.Painter, f Frame, box geom.Size, kids Children)
}

// A Handler is a node that wants input.
//
// Handle is called with the event aimed at this node. Return true to
// consume it; return false and the engine offers it to the parent, and
// on up to the root. Pointer positions arrive already translated into
// this node's coordinate space.
type Handler interface {
	Node
	Handle(e input.Event, u *UI) (handled bool)
}

// An Animator is a node that owns animated values.
//
// Step advances them by dt and reports whether anything is still
// moving. The engine keeps drawing frames while any node in the tree
// says true, and lets the window go to sleep when they all say false.
// Embedding [anim.Group] satisfies this.
type Animator interface {
	Node
	Step(dt time.Duration) (animating bool)
}

// A Transitioner is a node that animates as it enters and leaves the
// tree.
//
// Transition is called every frame while the node is [Entering] or
// [Exiting], and should start or continue whatever animation suits that
// state. Because it is called repeatedly, it must be safe to call
// repeatedly: [anim.Animated.Animate] ignores a target it is already
// heading for, so the obvious implementation is already correct.
//
//	func (d *Dialog) Transition(p gunim.Presence) bool {
//	    switch p {
//	    case gunim.Entering:
//	        d.in.Animate(1, anim.Bouncy)
//	    case gunim.Exiting:
//	        d.in.Animate(0, anim.Gentle)
//	    }
//	    return !d.in.Active()
//	}
//
// Returning true means "I have finished settling into this state". The
// engine unlinks an Exiting node only once it and every Transitioner
// beneath it has returned true, so a panel stays put while a child is
// still animating.
//
// Leave Transitioner out and the node appears and disappears at once.
type Transitioner interface {
	Node
	Transition(p Presence) (settled bool)
}

// A Composite is a node built from child nodes it owns.
//
// The engine inserts them when the node is inserted, so a widget made
// of other widgets arrives whole. [widget.Dialog] uses it for its OK
// and Cancel buttons.
type Composite interface {
	Node
	// Children returns the nodes this node owns, in paint order.
	Children() []Node
}

// Presence is where a node is in its lifecycle.
type Presence uint8

const (
	// Entering means the node has been inserted and is animating in.
	Entering Presence = iota
	// Present means the node is settled and fully part of the tree.
	Present
	// Exiting means the node has been removed and is animating out. It
	// still lays out, still paints, and still receives frames. Input
	// skips it, so a click landing on a dialog mid-fade passes through
	// to whatever is behind.
	Exiting
)

func (p Presence) String() string {
	switch p {
	case Entering:
		return "entering"
	case Present:
		return "present"
	case Exiting:
		return "exiting"
	}
	return "invalid"
}

// Constraints bound the size a node may choose during layout.
type Constraints struct {
	Min, Max geom.Size
}

// Tight returns constraints that permit exactly one size.
func Tight(s geom.Size) Constraints { return Constraints{Min: s, Max: s} }

// Loose returns constraints that permit anything up to s.
func Loose(s geom.Size) Constraints { return Constraints{Max: s} }

// Constrain clamps s to fit.
func (c Constraints) Constrain(s geom.Size) geom.Size {
	return geom.Size{
		W: clamp(s.W, c.Min.W, c.Max.W),
		H: clamp(s.H, c.Min.H, c.Max.H),
	}
}

// Frame is the per-frame context handed to layout and paint.
type Frame struct {
	// Now is when this frame is predicted to reach the screen. Every
	// node in a frame sees the same value, so two animations started
	// together stay together.
	Now time.Time
	// Delta is how long since the previous frame.
	Delta time.Duration
	// Scale is device pixels per logical pixel, from the monitor this
	// window is on. It changes when the window is dragged between a
	// laptop screen and an external monitor.
	Scale float32

	// seq numbers the frame, so the engine can tell which nodes this
	// frame drew.
	seq uint64
}

// Children is a node's children, in order. It is valid only for the
// duration of the call it was passed to.
type Children struct {
	ns []*state
	f  Frame
}

// Len returns the number of children, including any that are exiting.
func (c Children) Len() int { return len(c.ns) }

// At returns the i'th child.
func (c Children) At(i int) Child { return Child{n: c.ns[i], f: c.f} }

// All ranges over the children in order.
func (c Children) All(yield func(Child) bool) {
	for _, n := range c.ns {
		if !yield(Child{n: n, f: c.f}) {
			return
		}
	}
}

// A Child is one child of the node currently being laid out or painted.
type Child struct {
	n *state
	f Frame
}

// Node returns the underlying node, for the rare case a container needs
// to ask it something directly.
func (c Child) Node() Node { return c.n.node }

// Presence reports where the child is in its lifecycle.
//
// A container should look at this. A list whose rows animate out can
// keep an [Exiting] row's slot open and collapse it as the row shrinks;
// a dialog stack can ignore exiting entries when deciding which dialog
// is on top.
func (c Child) Presence() Presence { return c.n.presence }

// Layout lays the child out within cs and returns the size it chose.
func (c Child) Layout(cs Constraints) geom.Size {
	c.n.size = c.n.node.Layout(cs, c.f, Children{ns: c.n.kids, f: c.f})
	return c.n.size
}

// Place positions the child's top-left corner in this node's
// coordinate space. It must be called after Layout.
func (c Child) Place(at geom.Point) {
	c.n.origin = at
}

// Size returns the size the child chose at its last Layout.
func (c Child) Size() geom.Size { return c.n.size }

// Paint draws the child's subtree at the position it was placed, under
// whatever transform the parent has pushed.
//
// That transform is what input follows. A child drawn scaled, slid or
// rotated receives pointer events where it appears on screen, with
// positions in its own space. A child receives them only where it is
// visible: inside every clipping layer around it, and only when its
// parent painted it at all.
func (c Child) Paint(p *paint.Painter) {
	defer p.Push(paint.Translate(c.n.origin))()
	c.n.toWindow = p.Transform()
	c.n.clip = p.Clip()
	c.n.drawn = c.f.seq
	c.n.node.Paint(p, c.f, c.n.size, Children{ns: c.n.kids, f: c.f})
}

// state is the engine's bookkeeping for one node. It stays inside the
// package.
type state struct {
	node     Node
	parent   *state
	kids     []*state
	presence Presence
	origin   geom.Point
	size     geom.Size
	// id and view are set when the node is the root of a mounted view,
	// so commands can address it and intents can say where they came
	// from.
	id     ID
	view   *view
	topics []string
	// settled records the result of the last transition pass, so reap
	// can unlink finished subtrees from a single walk.
	settled bool
	// toWindow is the transform the node was last painted under, from
	// its own space to the window's, and drawn is the frame that painted
	// it. Hit testing reads both, so input follows what is on screen.
	toWindow paint.Transform
	drawn    uint64
	// clip is the clipping the node was last painted under.
	clip *paint.Clip
}

// leaving reports whether s or any node above it is [Exiting].
func (s *state) leaving() bool {
	for a := s; a != nil; a = a.parent {
		if a.presence == Exiting {
			return true
		}
	}
	return false
}

// within reports whether s is anc or lies beneath it.
func (s *state) within(anc *state) bool {
	for a := s; a != nil; a = a.parent {
		if a == anc {
			return true
		}
	}
	return false
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if hi > 0 && v > hi {
		return hi
	}
	return v
}
