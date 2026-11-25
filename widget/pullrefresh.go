package widget

import (
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// PullRefresh tokens.
var (
	// PullDisc is the size of the disc that holds the spinner, PullReach how far down a pull must bring it to
	// refresh, and PullRest where it stands while the refresh is under way, each from the top edge to the disc's
	// bottom.
	PullDisc  = theme.Length("pull.disc", 40)
	PullReach = theme.Length("pull.reach", 88)
	PullRest  = theme.Length("pull.rest", 64)
	// PullRefresh names a screen reader's action that refreshes, PullRefreshing is what it hears as a refresh
	// starts, PullDone as it ends, and PullTimedOut as one ends unanswered.
	PullRefreshWords = theme.Choice("pull.refresh", "Refresh")
	PullRefreshing   = theme.Choice("pull.refreshing", "Refreshing")
	PullDone         = theme.Choice("pull.done", "Refreshed")
	PullTimedOut     = theme.Choice("pull.timedout", "The refresh took too long")
)

// pullFailShows is how long a failed refresh shows its mark, and pullTurn how long the spinner takes to go round.
const (
	pullFailShows = 1500 * time.Millisecond
	pullTurn      = 900 * time.Millisecond
	pullTimeout   = 30 * time.Second
)

// PullRefresh holds a list or a page that scrolls, and refreshes it as the user pulls it down from its top.
//
// As a finger drags the content down past its top, a disc comes down from the top edge, its arc growing as it
// comes, and the further it goes the less it gives. Let go past PullReach, the disc springs to PullRest and spins,
// and OnRefresh runs. The program answers with Done when the refresh is done, and the disc slides away, or with
// Fail, and the disc shows a warning for a moment first. A refresh unanswered for Timeout fails. A pull while a
// refresh is under way starts no other. With less motion the spinner stands still.
//
// The mouse pulls the same way from a part of the content that takes no press, while the content is at its top. F5
// and Ctrl+R refresh from the keyboard where nothing focused takes them first, and a screen reader has a Refresh
// action.
type PullRefresh struct {
	anim.Group
	// Label names the content for a screen reader, such as "Messages".
	Label string
	// OnRefresh runs on the UI goroutine as the user asks for a refresh, with the refresh's number, counting from 1;
	// a non-nil result is sent to the application as the intent. The program answers with Done or Fail, with the
	// number.
	OnRefresh func(n int, u *gunim.UI) gunim.Intent
	// Timeout is how long a refresh may go unanswered before it fails, and zero waits for ever. NewPullRefresh sets
	// it to 30 seconds.
	Timeout time.Duration
	// NoRefreshKeys leaves F5 and Ctrl+R to the program, for one that uses them for something else.
	NoRefreshKeys bool

	child gunim.Node
	// pull is how far down the disc's bottom stands, from the top edge.
	pull *anim.Float
	// count is how many refreshes there have been, and current the number of the one under way, or 0.
	count, current int
	// own says the refresh under way is the program's own, from SetRefreshing.
	own bool
	// since is when the refresh under way began, for the spinner's turn.
	since time.Time
	// failed says the disc shows the warning of a failed refresh.
	failed       bool
	stopTimeout  func()
	stopFailShow func()
	// drag follows a pull under way: by a finger's scrolls, or by the mouse from start; raw is how far the pointer
	// has gone down, and ready says it has gone far enough.
	drag struct {
		on, mouse, armed, ready bool
		start, raw              float32
	}
	painted bool
	pts     []paint.RibbonPoint
	// th is the window's live theme, kept from Layout for Access.
	th *theme.Live
	u  *gunim.UI
}

// NewPullRefresh returns child, which scrolls, ready to refresh as it is pulled down.
func NewPullRefresh(child gunim.Node) *PullRefresh {
	r := &PullRefresh{child: child, pull: anim.NewFloat(0), Timeout: pullTimeout}
	r.Add(r.pull)
	return r
}

// Children implements [gunim.Composite].
func (r *PullRefresh) Children() []gunim.Node { return []gunim.Node{r.child} }

// Refreshing reports whether a refresh is under way.
func (r *PullRefresh) Refreshing() bool { return r.current > 0 }

// Current returns the number of the refresh under way, or 0.
func (r *PullRefresh) Current() int { return r.current }

// SetRefreshing shows a refresh of the program's own under way, as when a page opens, with the disc spinning at
// PullRest, or ends one with no warning, and sends no intent. A refresh the user started that is under way stays.
func (r *PullRefresh) SetRefreshing(on bool, u *gunim.UI) {
	switch {
	case on && r.current == 0:
		r.begin(true, u)
	case !on && r.current > 0:
		r.end(r.current, false, "", u)
	}
}

// Done ends refresh n: the disc slides away. An answer to a refresh that has ended does nothing.
func (r *PullRefresh) Done(n int, u *gunim.UI) { r.end(n, false, "", u) }

// Fail ends refresh n as failed: the disc shows a warning for a moment and then slides away, and a screen reader
// hears why. An answer to a refresh that has ended does nothing.
func (r *PullRefresh) Fail(n int, why string, u *gunim.UI) { r.end(n, true, why, u) }

// begin starts a refresh, the program's own or the user's, which runs OnRefresh.
func (r *PullRefresh) begin(own bool, u *gunim.UI) {
	if r.current > 0 {
		return
	}
	r.count++
	r.current, r.own, r.failed = r.count, own, false
	if r.stopFailShow != nil {
		r.stopFailShow()
		r.stopFailShow = nil
	}
	n := r.current
	if u == nil {
		r.pull.Jump(0)
		return
	}
	th := u.Theme()
	r.since = u.Now()
	r.pull.Animate(PullRest.Get(th), Settle.Get(th))
	u.Announce(PullRefreshing.Get(th))
	if r.Timeout > 0 {
		r.stopTimeout = u.After(r.Timeout, func(u *gunim.UI) { r.Fail(n, PullTimedOut.Get(u.Theme()), u) })
	}
	if !own {
		act(u, r, gunim.CuePress, r.OnRefresh, n)
	}
	u.Invalidate()
}

// end ends refresh n, as failed with why or as done.
func (r *PullRefresh) end(n int, failed bool, why string, u *gunim.UI) {
	if n <= 0 || n != r.current {
		return
	}
	r.current, r.own = 0, false
	if r.stopTimeout != nil {
		r.stopTimeout()
		r.stopTimeout = nil
	}
	if u == nil {
		r.pull.Jump(0)
		return
	}
	th := u.Theme()
	if !failed {
		u.Announce(PullDone.Get(th))
		r.retract(th)
		u.Invalidate()
		return
	}
	r.failed = true
	if why != "" {
		u.Announce(why)
	}
	r.stopFailShow = u.After(pullFailShows, func(u *gunim.UI) {
		r.stopFailShow = nil
		r.failed = false
		r.retract(u.Theme())
		u.Invalidate()
	})
	u.Invalidate()
}

// retract slides the disc away, unless a pull holds it.
func (r *PullRefresh) retract(th *theme.Live) {
	if !r.drag.on {
		r.pull.Animate(0, Settle.Get(th))
	}
}

// Transition implements [gunim.Transitioner]: a pull that leaves stops its timers, so a refresh under way neither
// fails nor speaks once it has gone.
func (r *PullRefresh) Transition(p gunim.Presence, _ gunim.Frame) bool {
	if p == gunim.Exiting {
		r.stopTimers()
	}
	return true
}

// stopTimers stops the timeout and the warning's wait.
func (r *PullRefresh) stopTimers() {
	if r.stopTimeout != nil {
		r.stopTimeout()
		r.stopTimeout = nil
	}
	if r.stopFailShow != nil {
		r.stopFailShow()
		r.stopFailShow = nil
	}
}

// dropPull lets go of a finger's pull that its ScrollEnd missed, as when the pull was hidden mid-pull: the disc
// springs back as for a pull that went too short.
func (r *PullRefresh) dropPull(th *theme.Live) {
	if !r.drag.on || r.drag.mouse {
		return
	}
	r.drag.on, r.drag.ready = false, false
	to := float32(0)
	if r.current > 0 || r.failed {
		to = PullRest.Get(th)
	}
	r.pull.Animate(to, Settle.Get(th))
}

// Overhear implements [gunim.Overhearer]: a finger coming down starts a new gesture, which ends any pull whose end
// went unheard.
func (r *PullRefresh) Overhear(e input.Event, u *gunim.UI) {
	if d, ok := e.(input.PointerDown); ok && d.Touch {
		r.dropPull(u.Theme())
	}
}

// give is how far down the disc stands for the pointer gone raw down: less the further it goes.
func give(raw, reach float32) float32 {
	if raw <= 0 {
		return 0
	}
	most := reach * 1.6
	return most * float32(1-math.Exp(-float64(raw/most)))
}

// atTop reports whether the content is scrolled to its top, where it says.
func (r *PullRefresh) atTop() bool {
	o, ok := r.child.(interface{ Offset() float32 })
	return ok && o.Offset() <= 0.5
}

// ClaimsPointer implements [gunim.PointerClaimer]: while a finger pulls, its scrolls are the pull's before the
// content's, so a pull pushed back up ends before the content scrolls.
func (r *PullRefresh) ClaimsPointer(geom.Point) bool { return r.drag.on && !r.drag.mouse }

// Handle implements [gunim.Handler].
func (r *PullRefresh) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.Scroll:
		if !e.Touch || e.Fling || r.drag.mouse {
			return false
		}
		if !r.drag.on {
			if e.Delta.Y <= 0 || r.current > 0 {
				return false
			}
			r.startPull(0, false)
		}
		r.pullTo(r.drag.raw+e.Delta.Y, u)
		return true
	case input.ScrollEnd:
		if r.drag.on && !r.drag.mouse {
			r.letGo(u)
		}
		return false
	case input.PointerDown:
		if e.Touch || e.Button != input.ButtonPrimary || r.current > 0 || !r.atTop() {
			return false
		}
		r.startPull(e.Pos.Y, true)
		return true
	case input.PointerMove:
		if !r.drag.mouse {
			return false
		}
		r.pullTo(e.Pos.Y-r.drag.start, u)
		return true
	case input.PointerUp:
		if !r.drag.mouse {
			return false
		}
		r.letGo(u)
		return true
	}
	return false
}

// CatchKey implements [gunim.KeyCatcher]: F5 and Ctrl+R refresh where nothing focused, nor anything round it,
// takes them, unless NoRefreshKeys is set.
func (r *PullRefresh) CatchKey(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	return ok && !r.NoRefreshKeys && r.refreshKey(k, u)
}

// refreshKey refreshes on F5 and Ctrl+R, and reports whether k was one.
func (r *PullRefresh) refreshKey(k input.KeyPress, u *gunim.UI) bool {
	if k.Key == input.KeyF5 && k.Mods == 0 || k.Key == input.KeyR && k.Mods == input.ModControl {
		r.begin(false, u)
		return true
	}
	return false
}

// startPull starts a pull, by the mouse pressed at y or by a finger's scrolls.
func (r *PullRefresh) startPull(y float32, mouse bool) {
	r.drag.on, r.drag.mouse, r.drag.ready = !mouse, mouse, false
	r.drag.armed = mouse
	r.drag.start, r.drag.raw = y, 0
}

// pullTo follows the pointer gone raw down: the disc comes down with it, giving, and a pull pushed back past
// where it began ends.
func (r *PullRefresh) pullTo(raw float32, u *gunim.UI) {
	th := u.Theme()
	r.drag.raw = raw
	if r.drag.mouse && !r.drag.on {
		// The mouse pulls once it has gone a few pixels down, so a click stays a click.
		if raw < 4 {
			return
		}
		r.drag.on = true
	}
	if raw <= 0 && !r.drag.mouse {
		r.drag = struct {
			on, mouse, armed, ready bool
			start, raw              float32
		}{}
		r.pull.Jump(0)
		u.Invalidate()
		return
	}
	at := give(raw, PullReach.Get(th))
	r.pull.Jump(at)
	if ready := at >= PullReach.Get(th); ready != r.drag.ready {
		r.drag.ready = ready
		if ready {
			u.Cue(gunim.CueTick, r)
		}
	}
	u.Invalidate()
}

// letGo ends a pull: one that went far enough refreshes, and the disc springs back otherwise.
func (r *PullRefresh) letGo(u *gunim.UI) {
	ready := r.drag.on && r.drag.ready
	r.drag.on, r.drag.mouse, r.drag.armed, r.drag.ready = false, false, false, false
	th := u.Theme()
	switch {
	case ready && r.current == 0:
		r.begin(false, u)
	case r.current > 0 || r.failed:
		r.pull.Animate(PullRest.Get(th), Settle.Get(th))
	default:
		r.pull.Animate(0, Settle.Get(th))
	}
	u.Invalidate()
}

// spinning reports whether the spinner turns: while a refresh is under way, with motion on.
func (r *PullRefresh) spinning(th *theme.Live) bool {
	return r.current > 0 && th != nil && theme.Scale.Get(th) > 0
}

// Step implements [anim.Stepper]: the spinner keeps drawing while it turns and is painted.
func (r *PullRefresh) Step(dt time.Duration) bool {
	turning := r.spinning(r.th) && r.painted
	if dt > 0 {
		r.painted = false
	}
	return r.Group.Step(dt) || turning
}

// Layout implements [gunim.Node]: the content fills the room.
func (r *PullRefresh) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	r.th, r.u = f.Theme, f.UI()
	size := fillOr(c, geom.Sz(390, 600))
	kid := kids.At(0)
	kid.Layout(gunim.Tight(size))
	kid.Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node]: the content, and over it the disc as far down as it has come.
func (r *PullRefresh) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	r.paintDisc(p, f.Theme, box, f.Now)
}

// paintDisc paints the disc in box as far down as it has come, its spinner turned as at now.
func (r *PullRefresh) paintDisc(p *paint.Painter, th *theme.Live, box geom.Size, now time.Time) {
	at := r.pull.Value()
	if at <= 0.5 {
		return
	}
	r.painted = true
	d := PullDisc.Get(th)
	reach := PullReach.Get(th)
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Clip: true, Opacity: min(at/d, 1)})()
	disc := geom.Rc(box.W/2-d/2, at-d, d, d)
	p.RRect(disc, d/2, paint.Solid(MenuFill.Get(th)))
	p.RRectStroke(disc, d/2, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	if r.failed {
		s := IconSize.Get(th)
		PaintIcon(p, th, icon.TriangleAlert, geom.Rc(disc.Center().X-s/2, disc.Center().Y-s/2, s, s),
			ToastErrorInk.Get(th))
		return
	}
	// The arc grows as the disc comes down, and turns while the refresh is under way.
	var turn, frac float32
	ink := Accent.Get(th)
	switch {
	case r.current > 0:
		frac = 0.75
		if r.spinning(th) {
			turn = 2 * math.Pi * float32(now.Sub(r.since)%pullTurn) / float32(pullTurn)
		}
	default:
		t := min(at/reach, 1)
		frac, turn = 0.8*t, math.Pi*t/2
		if t < 1 {
			ink = anim.Mix(anim.ColorCodec, CaptionInk.Get(th), Accent.Get(th), t*0.6)
		}
	}
	if frac <= 0.01 {
		return
	}
	c := disc.Center()
	r.pts = arc(r.pts[:0], c, d*0.25, max(d/16, 2), frac, ink)
	defer p.Push(paint.Rotate(turn, c))()
	p.Ribbon(r.pts)
}

// Access implements [gunim.Accessible]: a group named by Label, which says when it is refreshing, with a Refresh
// action.
func (r *PullRefresh) Access() access.Info {
	info := access.Info{Role: access.RoleGroup, Name: r.Label}
	if r.th == nil {
		return info
	}
	if r.current > 0 {
		info.Description = PullRefreshing.Get(r.th)
	} else {
		info.Actions = []string{PullRefreshWords.Get(r.th)}
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: Refresh refreshes.
func (r *PullRefresh) AccessAct(req access.Request, u *gunim.UI) bool {
	if req.Action != PullRefreshWords.Get(u.Theme()) || r.current > 0 {
		return false
	}
	r.begin(false, u)
	return true
}
