package gunim

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Main starts the platform event loop and runs fn alongside it.
//
// Call it from main, on the main goroutine: X11, Win32 and Cocoa all
// insist that windows are created and events pumped there, and no
// amount of Go can wish that away. fn runs on its own goroutine and
// opens windows through the [App] it is given. Main returns when fn
// returns, when the last window closes, or when ctx is cancelled.
//
//	func main() {
//	    err := gunim.Main(context.Background(), func(a *gunim.App) error {
//	        w, err := a.NewWindow(gunim.WindowOptions{Title: "hello"})
//	        if err != nil {
//	            return err
//	        }
//	        for range w.Client().Intents() {
//	        }
//	        return nil
//	    })
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// The error Main returns is the one fn returned, joined with any error
// the platform event loop ended on.
func Main(ctx context.Context, fn func(*App) error) error {
	drv, err := driver.Open()
	if err != nil {
		return fmt.Errorf("gunim: open display: %w", err)
	}
	return runApp(ctx, drv, fn)
}

// runApp is Main after the display is open, split out so a test can
// hand it a driver of its own.
func runApp(ctx context.Context, drv driver.Driver, fn func(*App) error) error {
	app := &App{drv: drv}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// fn's result goes into errc before cancel, so a Run that returns
	// because fn finished always finds it there.
	errc := make(chan error, 1)
	runErr := drv.Run(ctx, func() {
		go func() {
			errc <- fn(app)
			cancel()
		}()
	})
	select {
	case err := <-errc:
		return errors.Join(err, runErr)
	default:
		// The last window closed or ctx ended while fn was still
		// running, so there is no result from fn to report.
		return runErr
	}
}

// An App is the process's connection to the display server. It is the
// only way to open a window.
type App struct {
	drv driver.Driver
}

// Monitors lists the attached displays, so an application can put a
// window on a chosen one.
func (a *App) Monitors() []driver.Monitor { return a.drv.Monitors() }

// WindowOptions describes a window to open.
type WindowOptions struct {
	Title   string
	Size    geom.Size
	Monitor *driver.Monitor
	// Kind selects an ordinary window, a tool window or a free-floating
	// popup. A popup is a real window in the display server, so a menu
	// or a dragged-out modal can leave its parent's bounds.
	Kind driver.Kind
	// Parent and Anchor place a popup or utility window relative to an
	// existing one.
	Parent *Window
	Anchor geom.Point
	// Root is the node at the top of the tree. A nil Root gets an empty
	// [Box], and views can be mounted into it later through
	// [Client.Mount].
	Root Node
}

// NewWindow opens a window and starts its UI goroutine.
func (a *App) NewWindow(o WindowOptions) (*Window, error) {
	if o.Size == (geom.Size{}) {
		o.Size = geom.Sz(800, 600)
	}
	do := driver.Options{
		Title: o.Title, Size: o.Size, Monitor: o.Monitor,
		Kind: o.Kind, Anchor: o.Anchor,
	}
	if o.Parent != nil {
		do.Parent, do.Share = o.Parent.dw, o.Parent.dw
	}
	dw, err := a.drv.NewWindow(do)
	if err != nil {
		return nil, fmt.Errorf("gunim: open window %q: %w", o.Title, err)
	}

	w := newWindow(dw, o.Root)
	go w.loop()
	return w, nil
}

// newWindow builds a window and its tree, leaving the UI goroutine to
// the caller, so tests can drive the engine a frame at a time.
func newWindow(dw driver.Window, root Node) *Window {
	if root == nil {
		root = &Box{}
	}
	w := &Window{
		dw:    dw,
		out:   make(chan Envelope, 256),
		done:  make(chan struct{}),
		wake:  make(chan struct{}, 1),
		clock: time.Now(),
	}
	rootState := &state{node: root, presence: Present, id: Root}
	w.ui = &UI{
		w:      w,
		root:   rootState,
		index:  map[Node]*state{root: rootState},
		ids:    map[ID]*state{Root: rootState},
		topics: map[string][]*state{},
	}
	return w
}

// Client returns the application's handle to this window.
//
// Everything the application can do goes through it, and everything it
// can say is a plain value. In one process those values cross as they
// are; a socket transport encodes them with [MarshalCommand] and
// changes nothing on either side.
func (w *Window) Client() Client { return Client{w: w} }

// ErrWindowClosed is returned by [Client] methods once the window has
// gone. Match it with [errors.Is].
var ErrWindowClosed = errors.New("gunim: window is closed")

// A Client is the application side of a window.
//
// Its methods queue commands and return straight away, so application
// code never waits on a frame and never runs on the goroutine that
// draws one.
type Client struct{ w *Window }

// Send queues a command and returns at once.
//
// It never blocks on the window. Push state as fast as your application
// produces it: the queue coalesces superseded [Publish] commands as
// they arrive, and the window applies whatever is left once per display
// refresh. Backpressure from a slow frame would land on the goroutine
// doing the real work, which is the opposite of what this split is for.
//
// Send hands the command over, state and all; see [Command] for what
// that asks of you. It is safe from any goroutine.
func (c Client) Send(cmd Command) error {
	w := c.w
	select {
	case <-w.done:
		return ErrWindowClosed
	default:
	}

	w.inMu.Lock()
	w.queueLocked(cmd)
	w.inMu.Unlock()

	select {
	case w.wake <- struct{}{}:
	default:
	}
	return nil
}

// Mount builds the named view with the given state and inserts it under
// parent, where it animates in.
//
// watch names the topics the view follows, so one [Client.Publish]
// reaches every view showing the same data.
func (c Client) Mount(parent, id ID, view string, state any, watch ...string) error {
	return c.Send(Mount{Parent: parent, ID: id, View: view, Watch: watch, State: state})
}

// Update hands fresh state to one mounted view. It is [Client.Publish]
// to the topic named after the view's own ID.
func (c Client) Update(id ID, state any) error {
	return c.Send(Update{ID: id, State: state})
}

// Publish hands fresh state to every view watching key.
//
// This is where an aprot refresh trigger lands: the handler fires the
// trigger, the transport pushes the result, and every view showing that
// data animates the difference.
func (c Client) Publish(key string, state any) error {
	return c.Send(Publish{Key: key, State: state})
}

// Patch hands a typed partial change to every view watching key.
//
// Use it when a value moved and the shape stayed put, so the change
// lands as a spring retargeting rather than as a reconciled list. The
// views that care need a [RegisterPatch] handler for p's type.
func (c Client) Patch(key string, p any) error {
	if p == nil {
		return errors.New("gunim: Patch needs a value")
	}
	return c.Send(Patch{Key: key, Data: p})
}

// Unmount starts a view's exit and returns at once, with the view still
// on screen animating away.
func (c Client) Unmount(id ID) error { return c.Send(Unmount{ID: id}) }

// Focus moves keyboard focus. An empty id drops it.
func (c Client) Focus(id ID) error { return c.Send(Focus{ID: id}) }

// Intents returns the window's outbound stream. It closes when the
// window does, so ranging over it is a reasonable main loop.
func (c Client) Intents() <-chan Envelope { return c.w.out }

// Err returns the error that ended the window. Read it once Intents has
// closed.
func (c Client) Err() error { return c.w.err }

// Close shuts the window down.
func (c Client) Close() { c.w.Close() }

// NewOffscreen returns a window backed by no display, with the frame
// loop left to the caller.
//
// Drive it with [Window.Frame] and read the result from the driver's
// op list. It is how a test steps an interface a frame at a time, and
// how a build machine renders one with no display attached.
func NewOffscreen(size geom.Size, root Node) *Window {
	return newWindow(driver.Offscreen(size), root)
}

// Frame advances the window by delta and draws once.
//
// It is the body of the window's own loop, exposed so that a caller
// holding an offscreen window can step time. Commands queued through
// [Client] are applied first, the same way the loop applies them.
func (w *Window) Frame(delta time.Duration) {
	w.applyPending()
	w.clock = w.clock.Add(delta)
	w.ui.frame(w.clock, delta)
}

// A Window is one on-screen window and the UI goroutine that drives it.
//
// Everything inside a window — the tree, the focus, what the pointer is
// over — belongs to that goroutine. Application code sends commands in
// through [Window.Client] and hears back on [Client.Intents]. Because the
// goroutine alone decides when a node dies, the engine can keep a
// removed dialog alive for as long as its exit animation needs.
type Window struct {
	dw   driver.Window
	ui   *UI
	out  chan Envelope
	done chan struct{}
	// wake nudges the loop when a command arrives. It holds one token,
	// because one nudge is as good as a hundred.
	wake chan struct{}

	closeOnce sync.Once
	// clock is the synthetic frame time used by [Window.Frame], so an
	// offscreen window steps at whatever rate the caller chooses.
	clock time.Time

	// inMu guards the inbound queue, which application goroutines fill
	// and the UI goroutine drains once a frame.
	inMu sync.Mutex
	// pending is the batch the next frame will apply, and barrier is
	// where coalescing may start: everything before it was queued ahead
	// of a command that changes what exists.
	pending []Command
	barrier int
	stats   windowStats

	mu    sync.RWMutex
	views map[string]*view
	// err holds whatever ended the window. The UI goroutine writes it
	// before closing events, and [Window.Err] reads it afterwards, so
	// closing the channel carries the handover.
	err error
}

// Err returns the error that ended the window. Read it once
// [Client.Intents] has closed.
func (w *Window) Err() error { return w.err }

// RefreshRate is the rate of the monitor the window is currently on.
func (w *Window) RefreshRate() float64 { return w.dw.RefreshRate() }

// Close shuts the window down at once and drops any queued work. For an
// animated goodbye, unmount the root's views first and call Close once
// they have gone.
func (w *Window) Close() { w.closeOnce.Do(func() { close(w.done) }) }

// loop is the window's whole life.
//
// It sleeps until there is a reason to draw, waits for the display, and
// draws once. Every path to a frame goes through the display, so the
// window draws at most once per refresh whatever the application does
// with [Client].
func (w *Window) loop() {
	defer close(w.out)
	defer func() {
		if err := w.dw.Close(); err != nil {
			w.err = errors.Join(w.err, fmt.Errorf("gunim: close window: %w", err))
		}
	}()

	last := time.Now()
	for {
		// Sleep until something needs drawing. An idle window costs a
		// blocked goroutine and nothing else.
		for !w.wants() {
			if !w.awaitWork() {
				return
			}
		}

		// Wait for the display. Commands that arrive meanwhile join the
		// batch this frame is about to apply.
		if !w.awaitVsync() {
			return
		}

		w.applyPending()
		now := time.Now()
		delta := now.Sub(last)
		last = now
		w.ui.frame(now, delta)
	}
}

// wants reports whether anything is waiting to be drawn: queued
// commands, running animations, or a node that asked for one more
// frame.
func (w *Window) wants() bool {
	w.inMu.Lock()
	queued := len(w.pending)
	w.inMu.Unlock()
	return queued > 0 || w.ui.needsFrame()
}

// awaitWork blocks until a command or an input event arrives.
func (w *Window) awaitWork() bool {
	select {
	case <-w.done:
		return false
	case <-w.wake:
	case ev, ok := <-w.dw.Input():
		if !ok {
			return false
		}
		w.ui.handlePlatform(ev)
	}
	return true
}

// awaitVsync blocks until the display is ready for the next frame,
// taking commands and input while it waits.
//
// This is where an application pushing faster than the screen refreshes
// gets slowed to the screen. Input is handled as it arrives, because a
// click and the release after it mean different things in different
// orders. Commands queue, because the last state for a topic is the
// only one worth drawing.
func (w *Window) awaitVsync() bool {
	for {
		select {
		case <-w.done:
			return false
		case <-w.dw.Frames():
			return true
		case <-w.wake:
		case ev, ok := <-w.dw.Input():
			if !ok {
				return false
			}
			w.ui.handlePlatform(ev)
		}
	}
}

// queue adds cmd to the batch for the next frame.
//
// A [Publish] that a later one supersedes is replaced where it stands,
// so an application pushing state at 1 kHz costs one view update per
// frame rather than one per push. The work saved is the reconciliation
// behind that update, which is the expensive half.
//
// [Patch] stays out of it. Two patches on one topic may aim at
// different rows, so each carries something the other lacks.
func (w *Window) queueLocked(cmd Command) {
	w.stats.commands.Add(1)
	if key, ok := coalesceKey(cmd); ok {
		for i := w.barrier; i < len(w.pending); i++ {
			if k, ok := coalesceKey(w.pending[i]); ok && k == key {
				w.pending[i] = cmd
				w.stats.coalesced.Add(1)
				return
			}
		}
	} else {
		// Mount, Unmount and Focus change what exists, so state queued
		// ahead of one of them keeps its place and gets applied.
		w.barrier = len(w.pending) + 1
	}
	w.pending = append(w.pending, cmd)
}

// coalesceKey returns the topic a command replaces whole state on.
func coalesceKey(c Command) (string, bool) {
	switch c := c.(type) {
	case Publish:
		return c.Key, true
	case Update:
		return string(c.ID), true
	default:
		return "", false
	}
}

// applyPending takes the queue and runs it in order.
//
// The batch is lifted out under the lock and applied outside it, so an
// application goroutine can carry on queueing while the frame it
// triggered is still being built.
func (w *Window) applyPending() {
	w.inMu.Lock()
	batch := w.pending
	w.pending = nil
	w.barrier = 0
	w.inMu.Unlock()

	for _, cmd := range batch {
		w.ui.apply(cmd)
	}
}

// Stats reports what a window has done since it opened.
//
// Read it to check that an application pushing hard is still costing
// one frame per refresh: Commands counts what arrived, Coalesced counts
// what was superseded before it ever reached a view.
type Stats struct {
	Frames    int64
	Commands  int64
	Coalesced int64
}

// Stats returns a snapshot. It is safe from any goroutine.
func (w *Window) Stats() Stats {
	return Stats{
		Frames:    w.stats.frames.Load(),
		Commands:  w.stats.commands.Load(),
		Coalesced: w.stats.coalesced.Load(),
	}
}

type windowStats struct {
	frames    atomic.Int64
	commands  atomic.Int64
	coalesced atomic.Int64
}

// UI is the engine's handle to one window's tree.
//
// It is valid on the UI goroutine, inside a view's update or patch
// function or inside a node's own methods, and only for the length of
// that call.
// Use it and let it go.
type UI struct {
	w     *Window
	root  *state
	index map[Node]*state
	ids   map[ID]*state
	// topics maps a key to the mounted views watching it. One Publish
	// then reaches the list, the sidebar count and the status line
	// together.
	topics map[string][]*state

	focus *state
	hover *state

	now       time.Time
	painter   paint.Painter
	animating bool
	invalid   bool
	// pending holds intents the application has yet to take. See
	// [UI.post] for why it grows instead of blocking or dropping.
	pending []Envelope
}

// Root returns the node at the top of the tree.
func (u *UI) Root() Node { return u.root.node }

// Now is the current frame's timestamp.
func (u *UI) Now() time.Time { return u.now }

// Invalidate asks for one more frame, whatever the animation state. Use
// it when something changed that the engine can see no other way.
func (u *UI) Invalidate() { u.invalid = true }

// Send reports an intent from n to the application.
//
// The intent travels as a value, so the widget stays ignorant of the
// application and the application stays off this goroutine. From is
// filled in with the ID of the nearest mounted view above n, which is
// how the application knows which of three open dialogs answered.
//
// Send returns at once.
func (u *UI) Send(n Node, v Intent) {
	u.post(Envelope{From: u.idOf(n), Intent: v})
}

// report sends a gunim-generated intent.
func (u *UI) report(v Intent) { u.post(Envelope{Intent: v}) }

// post queues an envelope for the application.
//
// The queue is unbounded because intents are generated at human speed,
// so it stays short in practice, and because the alternatives are both
// worse: blocking here would freeze every animation in the window, and
// dropping would lose the click that mattered.
func (u *UI) post(e Envelope) {
	u.pending = append(u.pending, e)
	u.flush()
}

// flush hands as much of the queue to the application as it will take
// right now.
func (u *UI) flush() {
	for len(u.pending) > 0 {
		select {
		case u.w.out <- u.pending[0]:
			u.pending = u.pending[1:]
		default:
			return
		}
	}
}

// idOf finds the ID of the nearest mounted view at or above n.
func (u *UI) idOf(n Node) ID {
	for s := u.index[n]; s != nil; s = s.parent {
		if s.id != "" {
			return s.id
		}
	}
	return ""
}

// Insert adds child to the end of parent's children and starts its
// entrance.
//
// Inserting a node that is already in the tree leaves it in place when
// parent already holds it, and moves it to the end of parent's children
// otherwise. Either way, a node that is leaving reverses: it goes back
// to [Entering] and keeps the animation it was part way through, and so
// does everything beneath it that was leaving with it. Because springs
// carry their velocity, a dialog dismissed and immediately reopened
// swings smoothly back. Getting that for free is the point of the whole
// design.
func (u *UI) Insert(parent, child Node) {
	u.InsertAt(parent, -1, child)
}

// InsertAt adds child at index i, or at the end when i is negative.
//
// A child already in the tree moves to index i under parent, where i
// counts parent's children with child taken out. With a negative i it
// keeps its place when parent already holds it. Inserting a node into
// its own subtree panics.
func (u *UI) InsertAt(parent Node, i int, child Node) {
	ps, ok := u.index[parent]
	if !ok {
		panic("gunim: Insert into a node that is not in the tree")
	}
	assertAddressable(child)
	if cs, ok := u.index[child]; ok {
		wasLeaving := cs.leaving()
		if u.move(cs, ps, i) {
			u.invalid = true
		}
		if wasLeaving {
			reenter(cs)
			u.invalid = true
		}
		return
	}
	cs := &state{node: child, parent: ps, presence: Entering}
	ps.kids = insertKid(ps.kids, i, cs)
	u.index[child] = cs
	u.invalid = true

	// A composite arrives whole, so a view can return one node and get
	// the widget it describes.
	if c, ok := child.(Composite); ok {
		for _, k := range c.Children() {
			u.Insert(child, k)
		}
	}
}

// move puts s at index i under ps and reports whether it moved. A
// negative i leaves s where it is when ps already holds it.
func (u *UI) move(s, ps *state, i int) bool {
	if s.parent == ps && i < 0 {
		return false
	}
	if ps.within(s) {
		panic("gunim: Insert would put a node inside its own subtree")
	}
	s.parent.kids = slices.DeleteFunc(s.parent.kids, func(k *state) bool { return k == s })
	s.parent = ps
	ps.kids = insertKid(ps.kids, i, s)
	return true
}

func insertKid(kids []*state, i int, s *state) []*state {
	if i < 0 || i > len(kids) {
		return append(kids, s)
	}
	return slices.Insert(kids, i, s)
}

// reenter turns s and the part of its subtree that was leaving with it
// back to [Entering]. A descendant removed in its own right stays
// [Exiting].
func reenter(s *state) {
	s.presence = Entering
	for _, k := range s.kids {
		if k.presence != Exiting {
			reenter(k)
		}
	}
}

// Remove starts n's exit.
//
// n stays in the tree. It moves to [Exiting], keeps laying out and
// painting, and leaves once it and everything beneath it reports
// settled. A plain node goes at once; one with a [Transitioner] gets
// exactly as long as it asks for.
//
// Input skips a leaving node, so focus and hover inside n end here,
// with [FocusLost] and [PointerLeave] sent as usual.
func (u *UI) Remove(n Node) {
	s, ok := u.index[n]
	if !ok || s == u.root {
		return
	}
	s.presence = Exiting
	u.invalid = true
	if u.focus != nil && u.focus.within(s) {
		u.Focus(nil)
	}
	if u.hover != nil && u.hover.within(s) {
		u.send(u.hover, PointerLeave{Time: u.now})
		u.hover = nil
	}
}

// Presence reports where n is in its lifecycle.
func (u *UI) Presence(n Node) Presence {
	if s, ok := u.index[n]; ok {
		return s.presence
	}
	return Exiting
}

// Focus moves keyboard focus to n, sending [FocusLost] and
// [FocusGained] to the nodes concerned. Pass nil to drop focus.
//
// A node that is leaving takes no input, so focusing one does nothing.
func (u *UI) Focus(n Node) {
	var next *state
	if n != nil {
		next = u.index[n]
		if next != nil && next.leaving() {
			return
		}
	}
	if next == u.focus {
		return
	}
	if u.focus != nil {
		u.send(u.focus, FocusLost{Time: u.now})
	}
	u.focus = next
	if next != nil {
		u.send(next, FocusGained{Time: u.now})
	}
	u.invalid = true
}

// needsFrame reports whether there is anything to draw.
func (u *UI) needsFrame() bool { return u.animating || u.invalid }

// frame runs one complete cycle: advance time, settle the tree, lay
// out, paint, present.
func (u *UI) frame(now time.Time, delta time.Duration) {
	u.now = now
	u.invalid = false
	u.w.stats.frames.Add(1)
	u.flush()
	f := Frame{Now: now, Delta: delta, Scale: u.w.dw.Scale()}

	// 1. Advance every animated value by the real elapsed time.
	animating := u.step(u.root, delta)

	// 2. Let entering and exiting nodes run their transitions, then
	//    unlink the ones that have finished leaving.
	if !u.settle(u.root, Present) {
		animating = true
	}
	u.reap(u.root)

	u.animating = animating

	// 3. Lay the tree out at the window's current size.
	size := u.w.dw.Size()
	u.root.size = u.root.node.Layout(Tight(size), f, Children{ns: u.root.kids, f: f})

	// 4. Record the frame and hand it to the driver.
	u.painter.Reset()
	u.root.node.Paint(&u.painter, f, u.root.size, Children{ns: u.root.kids, f: f})
	if err := u.w.dw.Present(u.painter.Ops(), u.painter.Damage()); err != nil {
		u.w.err = fmt.Errorf("gunim: present frame: %w", err)
		u.w.Close()
	}
}

// step advances animated values depth-first and reports whether
// anything is still moving.
func (u *UI) step(s *state, dt time.Duration) bool {
	animating := false
	if a, ok := s.node.(Animator); ok && a.Step(dt) {
		animating = true
	}
	for _, k := range s.kids {
		if u.step(k, dt) {
			animating = true
		}
	}
	return animating
}

// settle runs each node's transition and records whether it has
// finished. A node inherits Exiting from its ancestors: when a panel
// leaves, everything inside it is leaving too, and the panel waits for
// its children to agree they are done.
func (u *UI) settle(s *state, inherited Presence) bool {
	p := s.presence
	if inherited == Exiting {
		p = Exiting
	}
	settled := true
	for _, k := range s.kids {
		if !u.settle(k, p) {
			settled = false
		}
	}
	if t, ok := s.node.(Transitioner); ok && p != Present {
		if !t.Transition(p) {
			settled = false
		}
	}
	if p == Entering && settled {
		s.presence = Present
	}
	s.settled = settled
	return settled
}

// reap unlinks exiting subtrees that have finished animating out.
func (u *UI) reap(s *state) {
	kept := s.kids[:0]
	for _, k := range s.kids {
		if k.presence == Exiting && k.settled {
			u.forget(k)
			continue
		}
		u.reap(k)
		kept = append(kept, k)
	}
	clear(s.kids[len(kept):])
	s.kids = kept
}

// forget drops a subtree from the index and gives up any focus or
// hover it held.
func (u *UI) forget(s *state) {
	if u.focus == s {
		u.focus = nil
	}
	if u.hover == s {
		u.hover = nil
	}
	if s.id != "" && u.ids[s.id] == s {
		delete(u.ids, s.id)
	}
	u.unsubscribe(s)
	delete(u.index, s.node)
	for _, k := range s.kids {
		u.forget(k)
	}
	s.kids = nil
	s.parent = nil
}

// assertAddressable requires nodes the tree can tell apart.
//
// The tree is keyed on the Node interface value, which is what lets
// UI.Remove take the node itself and spares the caller a handle to
// keep. That holds as long as distinct nodes are distinct values. Go
// promises that for pointers to types of non-zero size; two separate
// &Box{} values may legitimately share an address, at which point one
// silently becomes the other. That is a baffling bug to meet at frame
// 400, so it is caught here, once, at the point of the mistake.
func assertAddressable(n Node) {
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Pointer && t.Elem().Size() == 0 {
		panic("gunim: " + t.String() + " points at a zero-size type, so two of them can share an address and collide in the tree; give it at least one field")
	}
}
