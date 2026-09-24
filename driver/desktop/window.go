//go:build linux || windows || darwin

package desktop

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/internal/glfw"
	"github.com/marrasen/gunim/paint"
)

// errClosed is returned by Present once the window has closed.
var errClosed = errors.New("desktop: window is closed")

const (
	// doubleClick is how close in time and space two presses must be to
	// count as one sequence.
	doubleClick     = 500 * time.Millisecond
	doubleClickSlop = 4
	// scrollLine is how far one notch of a wheel scrolls, in logical
	// pixels.
	scrollLine = 40
	// resizeWait is the longest a resize waits for a frame at its new
	// size.
	resizeWait = 100 * time.Millisecond
)

// holdResize is true where the operating system shows a resized window
// as soon as its resize callback returns: Windows runs a modal loop
// during a drag-resize and calls back from inside it. Holding the
// callback until a frame at the new size has been drawn keeps frames
// drawn for the old size off the screen.
const holdResize = runtime.GOOS == "windows"

// Window is the desktop implementation of [driver.Window].
type Window struct {
	d  *Driver
	gw *glfw.Window

	in        *inbox
	presented chan driver.Frame
	frames    chan []paint.Op
	quit      chan struct{}
	done      chan struct{}
	// closeOnce runs Close once; quitOnce closes quit once, whichever
	// of Close and shutdown gets there first. They are separate so that
	// shutdown, run on the main thread while Close waits for it, waits on
	// quitOnce alone.
	closeOnce sync.Once
	quitOnce  sync.Once

	// mu guards what the main thread measures and the render thread
	// and the engine read.
	mu    sync.Mutex
	fbW   int
	fbH   int
	scale float32
	rate  float64
	// perCoord is framebuffer pixels per GLFW screen coordinate. It is 1
	// on X11 and Windows, and 2 on a Retina display.
	perCoord float32
	err      error
	// drawnW and drawnH are the framebuffer size of the last frame the
	// render thread swapped, and drew is nudged after each swap.
	drawnW, drawnH int
	drew           chan struct{}

	// The fields below belong to the main thread.
	closed     bool
	cursor     geom.Point
	mods       input.Mods
	lastPress  time.Time
	lastButton input.Button
	lastPos    geom.Point
	clicks     int
}

func newWindow(d *Driver, gw *glfw.Window) *Window {
	w := &Window{
		d:         d,
		gw:        gw,
		presented: make(chan driver.Frame, 1),
		frames:    make(chan []paint.Op, 1),
		quit:      make(chan struct{}),
		drew:      make(chan struct{}, 1),
		done:      make(chan struct{}),
		scale:     1,
		rate:      60,
		perCoord:  1,
	}
	w.in = newInbox(w.quit)
	return w
}

// Presented implements [driver.Window].
func (w *Window) Presented() <-chan driver.Frame { return w.presented }

// Input implements [driver.Window].
func (w *Window) Input() <-chan any { return w.in.out }

// Present implements [driver.Window]. It hands ops to the render thread
// and returns at once.
func (w *Window) Present(ops []paint.Op, _ geom.Rect) error {
	w.mu.Lock()
	err := w.err
	w.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case <-w.quit:
		return errClosed
	case w.frames <- ops:
		return nil
	default:
		return errors.New("desktop: Present called with a frame already in flight")
	}
}

// Size implements [driver.Window].
func (w *Window) Size() geom.Size {
	w.mu.Lock()
	defer w.mu.Unlock()
	return geom.Sz(float32(w.fbW)/w.scale, float32(w.fbH)/w.scale)
}

// Scale implements [driver.Window].
func (w *Window) Scale() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.scale
}

// RefreshRate implements [driver.Window].
func (w *Window) RefreshRate() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rate
}

// Close implements [driver.Window]. It stops the render thread, then
// destroys the window on the main thread.
func (w *Window) Close() error {
	var err error
	w.closeOnce.Do(func() {
		w.stopRender()
		<-w.done
		err = w.d.call(func() error {
			w.shutdown()
			return nil
		})
		if errors.Is(err, errStopped) {
			// Run has ended and destroyed every window already.
			err = nil
		}
	})
	return err
}

// shutdown destroys the window. It runs on the main thread, and is
// safe to call twice.
func (w *Window) shutdown() {
	if w.closed {
		return
	}
	w.closed = true
	w.in.close()
	w.stopRender()
	<-w.done
	delete(w.d.windows, w.gw)
	_ = w.gw.Destroy()
}

// awaitFrameAtSize blocks the main thread until the render thread has
// swapped a frame at the current framebuffer size, or resizeWait has
// passed. It runs on the main thread, inside a resize.
func (w *Window) awaitFrameAtSize() {
	timeout := time.NewTimer(resizeWait)
	defer timeout.Stop()
	for {
		w.mu.Lock()
		done := w.drawnW == w.fbW && w.drawnH == w.fbH
		w.mu.Unlock()
		if done {
			return
		}
		select {
		case <-w.drew:
		case <-timeout.C:
			return
		case <-w.quit:
			return
		}
	}
}

// RaiseThread implements [driver.Raiser].
func (*Window) RaiseThread() { raiseThread() }

// stopRender tells the render thread and the input feed to stop.
func (w *Window) stopRender() { w.quitOnce.Do(func() { close(w.quit) }) }

// place moves the window where the options ask: beside its parent at
// Anchor, or centred on a chosen monitor. It runs on the main thread.
func (w *Window) place(o driver.Options) error {
	if p, ok := o.Parent.(*Window); ok {
		px, py, err := p.gw.GetPos()
		if err != nil {
			return err
		}
		p.mu.Lock()
		f := p.scale / p.perCoord
		p.mu.Unlock()
		return w.gw.SetPos(px+int(o.Anchor.X*f), py+int(o.Anchor.Y*f))
	}
	if o.Monitor == nil {
		return nil
	}
	ww, wh, err := w.gw.GetSize()
	if err != nil {
		return err
	}
	// On Windows a window that scales to its monitor grows by the ratio
	// of the two monitors' scales once it arrives, so centre it at that
	// size.
	if runtime.GOOS == "windows" && o.Monitor.Scale > 0 {
		if from, _, err := w.gw.GetContentScale(); err == nil && from > 0 {
			ww = int(float32(ww) * o.Monitor.Scale / from)
			wh = int(float32(wh) * o.Monitor.Scale / from)
		}
	}
	b := o.Monitor.Bounds
	x := int(b.Min.X) + (int(b.Size().W)-ww)/2
	y := int(b.Min.Y) + (int(b.Size().H)-wh)/2
	return w.gw.SetPos(x, y)
}

// measure reads the window's framebuffer, scale and monitor. It runs
// on the main thread.
func (w *Window) measure() {
	fbW, fbH, err := w.gw.GetFramebufferSize()
	if err != nil {
		return
	}
	ww, _, err := w.gw.GetSize()
	if err != nil {
		return
	}
	scale, _, err := w.gw.GetContentScale()
	if err != nil || scale <= 0 {
		scale = 1
	}
	perCoord := float32(1)
	if ww > 0 {
		perCoord = float32(fbW) / float32(ww)
	}
	rate := w.monitorRate()

	w.mu.Lock()
	w.fbW, w.fbH, w.scale, w.perCoord = fbW, fbH, scale, perCoord
	if rate > 0 {
		w.rate = rate
	}
	w.mu.Unlock()
}

// monitorRate returns the refresh rate of the monitor holding the
// window's centre. It runs on the main thread.
func (w *Window) monitorRate() float64 {
	x, y, err := w.gw.GetPos()
	if err != nil {
		return 0
	}
	ww, wh, err := w.gw.GetSize()
	if err != nil {
		return 0
	}
	centre := geom.Pt(float32(x+ww/2), float32(y+wh/2))
	ms, err := glfw.GetMonitors()
	if err != nil {
		return 0
	}
	for _, m := range ms {
		if info, ok := monitorInfo(m); ok && info.Bounds.Contains(centre) {
			return info.RefreshRate
		}
	}
	if m, err := glfw.GetPrimaryMonitor(); err == nil && m != nil {
		if info, ok := monitorInfo(m); ok {
			return info.RefreshRate
		}
	}
	return 0
}

// logical converts a GLFW screen coordinate to logical pixels.
func (w *Window) logical(x, y float64) geom.Point {
	w.mu.Lock()
	f := w.perCoord / w.scale
	w.mu.Unlock()
	return geom.Pt(float32(x)*f, float32(y)*f)
}

// install connects GLFW's callbacks. They all run on the main thread.
func (w *Window) install() {
	gw := w.gw
	remeasure := func() {
		w.measure()
		w.in.push(driver.Redraw{})
	}
	_, _ = gw.SetFramebufferSizeCallback(func(*glfw.Window, int, int) {
		remeasure()
		if holdResize {
			w.awaitFrameAtSize()
		}
	})
	_, _ = gw.SetContentScaleCallback(func(*glfw.Window, float32, float32) { remeasure() })
	_, _ = gw.SetPosCallback(func(*glfw.Window, int, int) { remeasure() })
	_, _ = gw.SetRefreshCallback(func(*glfw.Window) { w.in.push(driver.Redraw{}) })
	_, _ = gw.SetCloseCallback(func(*glfw.Window) {
		// Closing the input tells the engine the window has gone; it
		// then calls Close, which destroys it.
		w.in.close()
	})

	_, _ = gw.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) {
		w.cursor = w.logical(x, y)
		w.in.push(input.PointerMove{Pos: w.cursor, Mods: w.mods, Time: time.Now()})
	})
	_, _ = gw.SetCursorEnterCallback(func(_ *glfw.Window, entered bool) {
		if !entered {
			w.in.push(input.PointerLeave{Time: time.Now()})
		}
	})
	_, _ = gw.SetMouseButtonCallback(func(_ *glfw.Window, b glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		button, ok := buttonOf(b)
		if !ok {
			return
		}
		w.mods = modsOf(mods)
		now := time.Now()
		if action == glfw.Release {
			w.in.push(input.PointerUp{Pos: w.cursor, Button: button, Mods: w.mods, Time: now})
			return
		}
		d := w.cursor.Sub(w.lastPos)
		if button == w.lastButton && now.Sub(w.lastPress) < doubleClick &&
			abs(d.X) <= doubleClickSlop && abs(d.Y) <= doubleClickSlop {
			w.clicks++
		} else {
			w.clicks = 1
		}
		w.lastPress, w.lastButton, w.lastPos = now, button, w.cursor
		w.in.push(input.PointerDown{Pos: w.cursor, Button: button, Mods: w.mods, Clicks: w.clicks, Time: now})
	})
	_, _ = gw.SetScrollCallback(func(_ *glfw.Window, x, y float64) {
		w.in.push(input.Scroll{
			Pos:   w.cursor,
			Delta: geom.Pt(float32(x)*scrollLine, float32(y)*scrollLine),
			Mods:  w.mods,
			Time:  time.Now(),
		})
	})
	_, _ = gw.SetKeyCallback(func(_ *glfw.Window, k glfw.Key, _ int, action glfw.Action, mods glfw.ModifierKey) {
		w.mods = modsOf(mods)
		now := time.Now()
		switch action {
		case glfw.Press, glfw.Repeat:
			w.in.push(input.KeyPress{Key: keyOf(k), Mods: w.mods, Repeat: action == glfw.Repeat, Time: now})
		case glfw.Release:
			w.in.push(input.KeyRelease{Key: keyOf(k), Mods: w.mods, Time: now})
		}
	})
	_, _ = gw.SetCharCallback(func(_ *glfw.Window, r rune) {
		w.in.push(input.TextInput{Text: string(r), Time: time.Now()})
	})
}

// render is the window's render thread. It owns the GL context: it
// replays each frame the engine presents, swaps, and reports the frame
// shown once the swap returns.
//
// The goroutine keeps its thread until it exits, so the raised thread
// and the GL context end together with it.
func (w *Window) render() {
	runtime.LockOSThread()
	raiseThread()
	defer close(w.done)

	r, err := w.startGL()
	if err != nil {
		w.fail(err)
	}
	if r != nil {
		defer func() {
			r.release()
			_ = (*glfw.Window)(nil).MakeContextCurrent()
		}()
	}

	vb := newVBlank(w.gw)
	defer vb.close()

	var last time.Time
	for {
		var ops []paint.Op
		select {
		case <-w.quit:
			return
		case ops = <-w.frames:
		}
		if r != nil {
			w.mu.Lock()
			fbW, fbH, scale, rate := w.fbW, w.fbH, w.scale, w.rate
			w.mu.Unlock()
			r.draw(ops, fbW, fbH, scale)
			synced := vb.wait()
			if err := w.gw.SwapBuffers(); err != nil {
				w.fail(fmt.Errorf("desktop: swap buffers: %w", err))
			}
			w.mu.Lock()
			w.drawnW, w.drawnH = fbW, fbH
			w.mu.Unlock()
			select {
			case w.drew <- struct{}{}:
			default:
			}
			if synced {
				// The frame went out on the monitor's vertical blank,
				// so that is when it reached the screen.
				last = time.Now()
			} else {
				last = pace(last, rate)
			}
		}
		select {
		case w.presented <- driver.Frame{Shown: last}:
		case <-w.quit:
			return
		}
	}
}

// pace returns the time a frame reached the screen, given when the
// last one did. It serves where the swap is the only signal of the
// display's timing: Linux and macOS, and Windows when the vertical
// blank wait is unavailable.
//
// A swap interval of one should make SwapBuffers wait for the display.
// Some setups skip the wait: a virtual machine, a remote desktop, a
// driver with vsync forced off. A swap that returns well inside one
// refresh has skipped it, so pace sleeps out the rest of the refresh
// itself, which holds such a window to the display's rate.
//
// How long the swap took proves nothing either way. With software GL a
// swap spends milliseconds copying pixels with no vsync behind it.
func pace(last time.Time, rate float64) time.Time {
	now := time.Now()
	if last.IsZero() || rate <= 0 {
		return now
	}
	interval := time.Duration(float64(time.Second) / rate)
	if next := last.Add(interval); now.Sub(last) < interval*3/4 {
		time.Sleep(time.Until(next))
		return next
	}
	return now
}

// startGL makes the context current on this thread, turns on vsync and
// builds the renderer.
func (w *Window) startGL() (*renderer, error) {
	if err := w.gw.MakeContextCurrent(); err != nil {
		return nil, fmt.Errorf("desktop: make context current: %w", err)
	}
	// An error here leaves the swap unpaced, which pace makes up for.
	_ = w.gw.SwapInterval(swapInterval)
	ctx, err := gl.NewDefaultContext()
	if err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	if err := ctx.LoadFunctions(); err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	return newRenderer(ctx, w.d.isES)
}

// fail records an error for the next Present to return.
func (w *Window) fail(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// inbox carries input from the main thread to the engine.
//
// It queues without bound, and a goroutine of its own feeds the
// channel. So the main thread, which pumps events for every window,
// always hands input over at once, and a window slow to read its input
// leaves the others running.
type inbox struct {
	out  chan any
	quit <-chan struct{}
	wake chan struct{}

	mu     sync.Mutex
	items  []any
	closed bool
}

func newInbox(quit <-chan struct{}) *inbox {
	q := &inbox{out: make(chan any), quit: quit, wake: make(chan struct{}, 1)}
	go q.feed()
	return q
}

func (q *inbox) push(ev any) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, ev)
	q.mu.Unlock()
	q.nudge()
}

// close ends the stream once what is queued has been delivered.
func (q *inbox) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.nudge()
}

func (q *inbox) nudge() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *inbox) feed() {
	for {
		q.mu.Lock()
		if len(q.items) == 0 {
			closed := q.closed
			q.mu.Unlock()
			if closed {
				close(q.out)
				return
			}
			select {
			case <-q.wake:
			case <-q.quit:
				return
			}
			continue
		}
		ev := q.items[0]
		q.items = q.items[1:]
		q.mu.Unlock()
		select {
		case q.out <- ev:
		case <-q.quit:
			return
		}
	}
}
