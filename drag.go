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
)

// dragMsg is one step of a drag, sent from one window to another.
type dragMsg struct {
	kind  dragKind
	at    geom.Point
	data  any
	from  *Window
	taken bool
}

// drag is the drag a window's pointer is carrying.
type drag struct {
	source *state
	data   any
	ghost  *Popup
	grab   geom.Point
	// over is the window the drag is over, or nil.
	over *Window
}

// leaveReach is how far a drag of files must go from every window of
// the application before it leaves it. Crossing a gap between two of
// its windows, however slowly, stays a drag inside it. Once the
// platform's drag has it, the pointer may come back as close as it
// likes to drop.
const leaveReach = 64

// StartDrag starts dragging data from n, which has the pointer pressed
// on it: call it from n's Handle, for a press or a move. ghost, when
// not nil, is shown under the pointer for the length of the drag, in a
// window of its own, held grab from its top left corner.
//
// The drag goes where the pointer does, over this window and the
// application's others. Nodes under it hear [input.DragOver], and the
// one that takes it hears [input.Drop] when the button is let go. n
// hears [input.DragEnd] at the end, saying whether a node took it.
func (u *UI) StartDrag(n Node, data any, ghost Node, grab geom.Point) {
	s, ok := u.index[n]
	if !ok {
		panic("gunim: StartDrag from a node that is not in the tree")
	}
	d := &drag{source: s, data: data, grab: grab}
	if ghost != nil {
		at := u.local(s, u.pointer).Sub(grab)
		d.ghost = u.OpenPopup(n, ghost, PopupOptions{
			Anchor:      geom.Rect{Min: at, Max: at},
			Passthrough: true,
		})
	}
	u.drag = d
	u.dragTo(u.pointer)
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
	if d.over != nil && d.over != over {
		u.w.sendDrag(d.over, dragMsg{kind: dragLeave})
	}
	d.over = over
	if over == nil && u.leaving(at) && u.dragOut(d) {
		return
	}
	if over != nil {
		u.w.sendDrag(over, dragMsg{kind: dragOver, at: at, data: d.data})
	}
	if d.ghost != nil {
		g := u.local(d.source, p).Sub(d.grab)
		d.ghost.Move(geom.Rect{Min: g, Max: g})
	}
	u.invalid = true
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
	u.dragFrom = d.source
	if err := do.DragOut(paths); err != nil {
		u.dragEnded(false)
	}
	return true
}

// dragDrop lets the drag go at p, in the window's space.
func (u *UI) dragDrop(p geom.Point) {
	u.dragTo(p)
	d := u.drag
	u.drag = nil
	if d.ghost != nil {
		d.ghost.Close()
	}
	u.dragFrom = d.source
	if d.over == nil {
		u.dragEnded(false)
		return
	}
	u.w.sendDrag(d.over, dragMsg{kind: dragDrop, at: toScreen(u.w, p), data: d.data, from: u.w})
}

// dragEnded tells the node the drag started from how it ended.
func (u *UI) dragEnded(taken bool) {
	s := u.dragFrom
	u.dragFrom = nil
	if s != nil && s.parent != nil {
		u.deliver(s, input.DragEnd{Taken: taken, Time: time.Now()})
	}
	u.invalid = true
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
		// A window too busy to take a drag step misses it; the next
		// move sends another.
		if m.kind == dragDrop {
			w.ui.dragEnded(false)
		}
	}
}

// dragHover offers a drag of data at p, in the window's space, to the
// node under it, and tells the node it left, if another took it.
func (u *UI) dragHover(p geom.Point, data any) {
	now := time.Now()
	took := u.dispatchAt(u.root, p, func(local geom.Point) input.Event {
		return input.DragOver{Pos: local, Data: data, Time: now}
	})
	if took != u.dragAt {
		if u.dragAt != nil {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
		}
		u.dragAt = took
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
		u.dragHover(p, m.data)
	case dragLeave:
		u.dragOver, u.dragOverData = false, nil
		if u.dragAt != nil {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
			u.dragAt = nil
		}
	case dragDrop:
		u.dragOver, u.dragOverData = false, nil
		p := fromScreen(u.w, m.at)
		took := u.dispatchAt(u.root, p, func(local geom.Point) input.Event {
			return input.Drop{Pos: local, Data: m.data, Time: now}
		})
		if u.dragAt != nil && u.dragAt != took {
			u.deliver(u.dragAt, input.DragLeave{Time: now})
		}
		u.dragAt = nil
		reply := dragMsg{kind: dragEnded, taken: took != nil}
		if m.from != nil {
			u.w.sendDrag(m.from, reply)
		}
	case dragEnded:
		u.dragEnded(m.taken)
	}
}
