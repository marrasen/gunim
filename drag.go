package gunim

import (
	"slices"
	"sync"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A drag carries a value from the node it started at to a node that
// takes it, in the same window or another window of the application.
//
// The source window owns the drag: it keeps the pointer while the
// button is down, wherever the pointer goes, so it follows the pointer
// across the screen, finds the window under it, and tells that window's
// UI goroutine. The picture under the pointer rides in a popup window
// that lets the pointer through. Everything crosses between windows as
// messages, since each window's nodes belong to its own goroutine.

// A FileExporter is drag data that can leave the application as files,
// to be dropped on a file manager or another program. Once a drag of
// one leaves every window of the application, ExportFiles writes the
// files, where they need writing, and returns their paths, and the
// platform's own drag and drop carries them on.
type FileExporter interface {
	ExportFiles() ([]string, error)
}

// windows is the application's open windows, which drags look through
// for the one under the pointer.
type windows struct {
	mu   sync.Mutex
	list []*Window
}

func (ws *windows) add(w *Window) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.list = append(ws.list, w)
}

func (ws *windows) remove(w *Window) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.list = slices.DeleteFunc(ws.list, func(o *Window) bool { return o == w })
}

// at returns the window holding p, a screen point, or nil. The window
// asking comes first, so a drag inside a window stays there where
// windows overlap.
func (ws *windows) at(p geom.Point, first *Window) *Window {
	ws.mu.Lock()
	list := slices.Clone(ws.list)
	ws.mu.Unlock()
	if first != nil && holds(first, p) {
		return first
	}
	for _, w := range list {
		if w != first && holds(w, p) {
			return w
		}
	}
	return nil
}

// farFrom reports whether p, a screen point, is farther than reach
// from every window.
func (ws *windows) farFrom(p geom.Point, reach float32) bool {
	ws.mu.Lock()
	list := slices.Clone(ws.list)
	ws.mu.Unlock()
	for _, w := range list {
		sc, ok := w.dw.(driver.Screener)
		if !ok {
			continue
		}
		q := sc.FromScreen(p)
		s := w.dw.Size()
		dx := max(0, -q.X, q.X-s.W)
		dy := max(0, -q.Y, q.Y-s.H)
		if dx*dx+dy*dy <= reach*reach {
			return false
		}
	}
	return true
}

// holds reports whether w covers p, a screen point.
func holds(w *Window, p geom.Point) bool {
	sc, ok := w.dw.(driver.Screener)
	if !ok {
		return false
	}
	return (geom.Rect{Max: w.dw.Size().Point()}).Contains(sc.FromScreen(p))
}

// toScreen turns p, in w's space, into a screen point.
func toScreen(w *Window, p geom.Point) geom.Point {
	if sc, ok := w.dw.(driver.Screener); ok {
		return sc.ToScreen(p)
	}
	return p
}

// fromScreen turns a screen point into w's space.
func fromScreen(w *Window, p geom.Point) geom.Point {
	if sc, ok := w.dw.(driver.Screener); ok {
		return sc.FromScreen(p)
	}
	return p
}

// dragKind is what a drag message says.
type dragKind uint8

const (
	// dragOver: the drag is over the window, at a screen point.
	dragOver dragKind = iota
	// dragLeave: the drag has left the window.
	dragLeave
	// dragDrop: the drag was let go over the window.
	dragDrop
	// dragEnded: the drop was taken, or was not, back to the source.
	dragEnded
	// dragAnswer: what a drop over the window would do, back to the
	// source.
	dragAnswer
)

// dragMsg is one step of a drag, sent from one window to another. drop
// numbers a drop, and its end carries the number back, so the end
// reaches the drag it is for.
type dragMsg struct {
	kind  dragKind
	at    geom.Point
	data  any
	mods  input.Mods
	from  *Window
	taken bool
	drop  uint64
}

// pendingDrop is a drag let go whose end is still to be heard: the node
// it started from, and the picture it carried, until it closes.
type pendingDrop struct {
	from  *state
	ghost *Popup
}

// drag is the drag a window's pointer is carrying.
type drag struct {
	source *state
	data   any
	ghost  *Popup
	grab   geom.Point
	// over is the window the drag is over, or nil.
	over *Window
	// mods are the modifier keys held.
	mods input.Mods
}

// leaveReach is how far a drag of files must go from every window of
// the application before it leaves it. Crossing a gap between two of
// its windows, however slowly, stays a drag inside it. Once the
// platform's drag has it, the pointer may come back as close as it
// likes to drop.
const leaveReach = 64

// ghostWait is the longest the picture a drag carries waits, once let
// go, to hear whether the drop was taken, and dropWait the longest the
// node it came from waits: a window that closed with the drop still
// unread never answers, and the drop ends untaken.
const (
	ghostWait = 2 * time.Second
	dropWait  = 10 * time.Second
)

// StartDrag starts dragging data from n, which has the pointer pressed
// on it: call it from n's Handle, for a press or a move. ghost, when
// not nil, is shown under the pointer for the length of the drag, in a
// window of its own, held grab from its top left corner.
//
// The drag goes where the pointer does, over this window and the
// application's others. Nodes under it hear [input.DragOver], and the
// one that takes it hears [input.Drop] when the button is let go. n
// hears [input.DragEnd] at the end, saying whether a node took it.
// Escape gives the drag up.
//
// The ghost hears [input.DragMove] as the pointer moves it,
// [input.DragAnswer] when the node under the pointer says what a drop
// would do, and [input.DragEnd] before it leaves.
func (u *UI) StartDrag(n Node, data any, ghost Node, grab geom.Point) {
	s, ok := u.index[n]
	if !ok {
		panic("gunim: StartDrag from a node that is not in the tree")
	}
	d := &drag{source: s, data: data, grab: grab}
	if ghost != nil {
		// The root opens the picture, so it stays while the source leaves
		at := u.local(u.root, u.pointer).Sub(grab)
		d.ghost = u.OpenPopup(u.root.node, ghost, PopupOptions{
			Anchor:      geom.Rect{Min: at, Max: at},
			Passthrough: true,
			Over:        true,
		})
	}
	u.drag = d
	u.dragTo(u.pointer)
}

// AnswerDrag says what a drop here would do, from a node's Handle for
// [input.DragOver]. The picture the drag carries hears it as
// [input.DragAnswer], in whichever window the drag started. answer
// must be comparable.
func (u *UI) AnswerDrag(answer any) { u.dragAnswer = answer }

// toGhost hands e to the picture p shows.
func (u *UI) toGhost(p *Popup, e input.Event) {
	if p == nil {
		return
	}
	for _, k := range p.s.root.kids {
		u.deliver(k, e)
	}
}

// dragTo carries the drag to p, in the window's space.
func (u *UI) dragTo(p geom.Point) {
	d := u.drag
	at := toScreen(u.w, p)
	var over *Window
	if u.w.app != nil {
		over = u.w.app.windows.at(at, u.w)
	} else if holds(u.w, at) {
		over = u.w
	}
	if d.over != over {
		if d.over != nil {
			u.w.sendDrag(d.over, dragMsg{kind: dragLeave})
		}
		u.toGhost(d.ghost, input.DragAnswer{Time: time.Now()})
	}
	d.over = over
	if over == nil && u.leaving(at) && u.dragOut(d) {
		return
	}
	if d.ghost != nil {
		g := u.local(u.root, p).Sub(d.grab)
		d.ghost.Move(geom.Rect{Min: g, Max: g})
		u.toGhost(d.ghost, input.DragMove{At: p, Time: time.Now()})
	}
	if over != nil {
		u.w.sendDrag(over, dragMsg{kind: dragOver, at: at, data: d.data, mods: d.mods, from: u.w})
	}
	u.invalid = true
}

// modKeys are the modifier keys and what each holds.
var modKeys = map[input.Key]input.Mods{
	input.KeyLeftShift: input.ModShift, input.KeyRightShift: input.ModShift,
	input.KeyLeftControl: input.ModControl, input.KeyRightControl: input.ModControl,
	input.KeyLeftAlt: input.ModAlt, input.KeyRightAlt: input.ModAlt,
}

// dragKey takes a key pressed or let go while the pointer carries a
// drag: Escape gives the drag up, and a modifier key tells the node
// under the drag.
func (u *UI) dragKey(ev any) {
	var mods input.Mods
	var key input.Key
	down := false
	switch e := ev.(type) {
	case input.KeyPress:
		mods, key, down = e.Mods, e.Key, true
	case input.KeyRelease:
		mods, key = e.Mods, e.Key
	default:
		return
	}
	if down && key == input.KeyEscape {
		u.cancelDrag()
		return
	}
	// A modifier's own key may or may not count itself as held.
	if m, ok := modKeys[key]; ok {
		if down {
			mods |= m
		} else {
			mods &^= m
		}
	}
	if mods != u.drag.mods {
		u.drag.mods = mods
		u.dragTo(u.pointer)
	}
}

// cancelDrag gives up the drag the pointer carries: nothing takes it.
func (u *UI) cancelDrag() {
	d := u.drag
	u.drag = nil
	if d.over != nil {
		u.w.sendDrag(d.over, dragMsg{kind: dragLeave})
	}
	u.endDrop(u.holdDrop(d.source, d.ghost), false)
}

// leaving reports whether a drag at the screen point at, outside every
// window of the application, has gone far enough to leave it.
func (u *UI) leaving(at geom.Point) bool {
	if u.w.app == nil {
		return !holds(u.w, at)
	}
	return u.w.app.windows.farFrom(at, leaveReach)
}

// dragOut hands d to other programs, when it can go as files, and
// reports whether it did. The picture under the pointer goes at once:
// the platform's drag shows its own, and on Windows it holds the main
// thread, which moving a window needs.
func (u *UI) dragOut(d *drag) bool {
	fe, ok := d.data.(FileExporter)
	if !ok {
		return false
	}
	do, ok := u.w.dw.(driver.DragOuter)
	if !ok {
		return false
	}
	paths, err := fe.ExportFiles()
	if err != nil || len(paths) == 0 {
		return false
	}
	if d.ghost != nil {
		u.closePopupNow(d.ghost.s)
	}
	u.drag = nil
	id := u.holdDrop(d.source, nil)
	if err := do.DragOut(paths); err != nil {
		u.endDrop(id, false)
		return true
	}
	u.dragOuts = append(u.dragOuts, id)
	return true
}

// dragDrop lets the drag go at p, in the window's space. The picture
// under the pointer stays until the window under it says whether it
// took the drop.
func (u *UI) dragDrop(p geom.Point) {
	u.dragTo(p)
	d := u.drag
	if d == nil {
		// The drag went out to other programs on the way.
		return
	}
	u.drag = nil
	id := u.holdDrop(d.source, d.ghost)
	if d.over == nil {
		u.endDrop(id, false)
		return
	}
	u.After(dropWait, func(u *UI) { u.endDrop(id, false) })
	if d.ghost != nil {
		// The picture waits so long for the answer, and then goes; the
		// node still hears the end when it comes.
		u.After(ghostWait, func(u *UI) {
			if p, ok := u.drops[id]; ok && p.ghost != nil {
				u.dropGhost(p.ghost, false)
				p.ghost = nil
				u.drops[id] = p
			}
		})
	}
	u.w.sendDrag(d.over, dragMsg{kind: dragDrop, at: toScreen(u.w, p), data: d.data, mods: d.mods, from: u.w, drop: id})
}

// holdDrop keeps a drag let go until its end is heard, and returns its
// number.
func (u *UI) holdDrop(from *state, ghost *Popup) uint64 {
	if u.drops == nil {
		u.drops = map[uint64]pendingDrop{}
	}
	u.dropSeq++
	u.drops[u.dropSeq] = pendingDrop{from: from, ghost: ghost}
	return u.dropSeq
}

// endDrop tells the node drop id started from how it ended, and the
// picture it carried, which then leaves. An end heard twice, or for a
// drop unknown here, does nothing.
func (u *UI) endDrop(id uint64, taken bool) {
	p, ok := u.drops[id]
	if !ok {
		return
	}
	delete(u.drops, id)
	if p.from != nil && p.from.parent != nil {
		u.deliver(p.from, input.DragEnd{Taken: taken, Time: time.Now()})
	}
	u.dropGhost(p.ghost, taken)
	u.invalid = true
}

// dropGhost tells a picture a drag let go of how the drag ended, and
// closes it.
func (u *UI) dropGhost(g *Popup, taken bool) {
	if g == nil || !g.Open() {
		return
	}
	u.toGhost(g, input.DragEnd{Taken: taken, Time: time.Now()})
	g.Close()
}

// sendDrag hands m to window to, at once when it is this window.
func (w *Window) sendDrag(to *Window, m dragMsg) {
	if to == w {
		w.ui.dragMsg(m)
		return
	}
	select {
	case to.dragIn <- m:
	default:
		switch m.kind {
		case dragDrop:
			w.ui.endDrop(m.drop, false)
		case dragEnded:
			// A drop's end must arrive, or its node never hears it:
			// it waits for the window to take it, or to close.
			go func() {
				select {
				case to.dragIn <- m:
				case <-to.done:
				}
			}()
		default:
			// A window too busy to take a drag step misses it; the
			// next move sends another.
		}
	}
}

// dragHover offers a drag of data at p, in the window's space, to the
// node under it, and tells the node it left, if another took it. What
// the node answers goes back to the window the drag came from.
func (u *UI) dragHover(p geom.Point, data any) {
	now := time.Now()
	u.dragAnswer = nil
	took := u.dispatchAt(u.root, p, func(local geom.Point) input.Event {
		return input.DragOver{Pos: local, Data: data, Mods: u.dragOverMods, Time: now}
	})
	answer := u.dragAnswer
	u.dragAnswer = nil
	if took == nil {
		answer = nil
	}
	if took != u.dragAt {
		if u.dragAt != nil {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
		}
		u.dragAt = took
	}
	if answer != u.dragAnswered && u.dragOverFrom != nil {
		u.dragAnswered = answer
		u.w.sendDrag(u.dragOverFrom, dragMsg{kind: dragAnswer, data: answer, from: u.w})
	}
}

// dragMsg handles one step of a drag over this window.
func (u *UI) dragMsg(m dragMsg) {
	u.invalid = true
	now := time.Now()
	switch m.kind {
	case dragOver:
		p := fromScreen(u.w, m.at)
		u.dragOver, u.dragOverAt, u.dragOverData = true, p, m.data
		u.dragOverMods, u.dragOverFrom = m.mods, m.from
		u.dragHover(p, m.data)
	case dragLeave:
		u.dragOver, u.dragOverData, u.dragOverFrom, u.dragAnswered = false, nil, nil, nil
		if u.dragAt != nil {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
			u.dragAt = nil
		}
	case dragDrop:
		u.dragOver, u.dragOverData, u.dragOverFrom, u.dragAnswered = false, nil, nil, nil
		p := fromScreen(u.w, m.at)
		took := u.dispatchAt(u.root, p, func(local geom.Point) input.Event {
			return input.Drop{Pos: local, Data: m.data, Mods: m.mods, Time: now}
		})
		if u.dragAt != nil && u.dragAt != took {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
		}
		u.dragAt = nil
		reply := dragMsg{kind: dragEnded, taken: took != nil, drop: m.drop}
		if m.from != nil {
			u.w.sendDrag(m.from, reply)
		}
	case dragEnded:
		u.endDrop(m.drop, m.taken)
	case dragAnswer:
		if d := u.drag; d != nil && d.over == m.from {
			u.toGhost(d.ghost, input.DragAnswer{Answer: m.data, Time: now})
		}
	}
}
