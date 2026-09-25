//go:build linux || windows || darwin

package desktop

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
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
	frames    chan frame
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
	// origin is the client area's top left corner in screen
	// coordinates.
	origin geom.Point
	err    error
	// caret is the text caret the engine last reported, in window space
	// and logical pixels. Platform code places an input method's
	// composition and candidate windows from it.
	caret geom.Rect
	// drawnW and drawnH are the framebuffer size of the last frame the
	// render thread swapped, and drew is nudged after each swap.
	drawnW, drawnH int
	drew           chan struct{}
	// readback, when set by a test, receives each frame's pixels as
	// RGBA rows from the bottom up, read before the swap.
	readback func(pix []byte, w, h int)

	// textInput is whether the application is taking text, which the
	// input method asks from the main thread.
	textInput atomic.Bool

	// ctx, where windows present through DXGI, is the hidden window
	// that holds this window's context, and pres, which belongs to the
	// render thread, presents its frames.
	ctx  *glfw.Window
	pres *presenter

	// acc is the window's place with assistive technology, and focused
	// whether it has the keyboard.
	acc     windowAccess
	focused atomic.Bool

	// parent is the window a popup belongs to, and popup is set for a
	// popup. Both are set once, before the window shows.
	parent *Window
	popup  bool
	// transparent is whether the window blends with what is behind it,
	// read once as it opens.
	transparent bool

	// The fields below belong to the main thread.
	//
	// anchor is where a popup was last attached, and popups are the
	// open popups that belong to this window, which follow it as it
	// moves.
	anchor     geom.Rect
	popups     []*Window
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
		frames:    make(chan frame, 1),
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
func (w *Window) Present(ops []paint.Op, damage geom.Rect) error {
	w.mu.Lock()
	err := w.err
	w.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case <-w.quit:
		return errClosed
	case w.frames <- frame{ops, damage}:
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

// SetTextCaret implements [driver.CaretPlacer]. It keeps the caret for
// the platform's input method.
func (w *Window) SetTextCaret(r geom.Rect) {
	w.mu.Lock()
	w.caret = r
	w.mu.Unlock()
	caretMoved(w)
}

// SetTextInput implements [driver.TextInputter].
func (w *Window) SetTextInput(active bool) {
	if w.textInput.Swap(active) == active {
		return
	}
	w.d.post(func() { textInputChanged(w, active) })
}

// Clipboard implements [driver.Window]. It reads the clipboard on the
// main thread, where GLFW has to, and waits for the answer.
func (w *Window) Clipboard() (string, error) {
	var s string
	err := w.d.call(func() error {
		var err error
		s, err = glfw.GetClipboardString()
		return err
	})
	return s, err
}

// SetClipboard implements [driver.Window].
func (w *Window) SetClipboard(s string) error {
	return w.d.call(func() error { return w.gw.SetClipboardString(s) })
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
	w.accessClose()
	if w.parent != nil {
		w.parent.popups = slices.DeleteFunc(w.parent.popups, func(c *Window) bool { return c == w })
	}
	w.in.close()
	w.stopRender()
	<-w.done
	delete(w.d.windows, w.gw)
	_ = w.gw.Destroy()
	if w.ctx != nil {
		_ = w.ctx.Destroy()
	}
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

// position moves the window where the options ask: attached to its
// anchor, or centred on a chosen monitor. It runs on the main thread.
func (w *Window) position(o driver.Options) error {
	if p, ok := o.Parent.(*Window); ok {
		if o.Kind == driver.KindPopup {
			w.parent, w.popup = p, true
			return w.attach(o.Anchor)
		}
		px, py, err := p.gw.GetPos()
		if err != nil {
			return err
		}
		f := p.coordsPerLogical()
		return w.gw.SetPos(px+int(o.Anchor.Min.X*f), py+int(o.Anchor.Min.Y*f))
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

// coordsPerLogical is GLFW screen coordinates per logical pixel.
func (w *Window) coordsPerLogical() float32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.scale / w.perCoord
}

// attach puts a popup beside anchor, a rectangle in its parent's
// logical space, as [driver.Options] describes. It runs on the main
// thread.
func (w *Window) attach(anchor geom.Rect) error {
	p := w.parent
	w.anchor = anchor
	if !slices.Contains(p.popups, w) {
		p.popups = append(p.popups, w)
	}
	px, py, err := p.gw.GetPos()
	if err != nil {
		return err
	}
	f := p.coordsPerLogical()
	a := geom.Rect{
		Min: geom.Pt(float32(px)+anchor.Min.X*f, float32(py)+anchor.Min.Y*f),
		Max: geom.Pt(float32(px)+anchor.Max.X*f, float32(py)+anchor.Max.Y*f),
	}
	ww, wh, err := w.gw.GetSize()
	if err != nil {
		return err
	}
	x, y := popupAt(a, float32(ww), float32(wh), workArea(a.Min))
	return w.gw.SetPos(int(x), int(y))
}

// popupAt returns where a popup of size w×h goes for anchor a, all in
// screen coordinates: below a, or above it when the room below runs
// out and there is more above, and slid sideways to stay inside area.
func popupAt(a geom.Rect, w, h float32, area geom.Rect) (x, y float32) {
	x, y = a.Min.X, a.Max.Y
	if area.Empty() {
		return x, y
	}
	if y+h > area.Max.Y && a.Min.Y-area.Min.Y > area.Max.Y-a.Max.Y {
		y = a.Min.Y - h
	}
	x = max(area.Min.X, min(x, area.Max.X-w))
	y = max(area.Min.Y, min(y, area.Max.Y-h))
	return x, y
}

// workArea returns the part of the monitor holding p that windows may
// use, which leaves out the taskbar and the like, in screen
// coordinates. It runs on the main thread.
func workArea(p geom.Point) geom.Rect {
	ms, err := glfw.GetMonitors()
	if err != nil {
		return geom.Rect{}
	}
	var first geom.Rect
	for _, m := range ms {
		x, y, mw, mh, err := m.GetWorkarea()
		if err != nil || mw <= 0 || mh <= 0 {
			continue
		}
		r := geom.Rc(float32(x), float32(y), float32(mw), float32(mh))
		if r.Contains(p) {
			return r
		}
		if first.Empty() {
			first = r
		}
	}
	return first
}

// dragOutWait is how long a drag handed to another program may wait
// for it to finish taking the drop before it is given up.
const dragOutWait = 10 * time.Second

// DragOut implements [driver.DragOuter]. The platform's drag and drop
// runs on the main thread; on Windows it holds the main thread until
// the drop, so this returns at once and reports the end on Input.
func (w *Window) DragOut(paths []string) error {
	ok := w.d.post(func() {
		ended := false
		end := func(taken bool) {
			ended = true
			w.in.push(driver.DragOutEnded{Taken: taken})
		}
		if err := w.gw.StartDragOut(paths, end); err != nil {
			end(false)
			return
		}
		time.AfterFunc(dragOutWait, func() {
			w.d.post(func() {
				if !ended {
					w.gw.CancelDragOut()
				}
			})
		})
	})
	if !ok {
		return errStopped
	}
	return nil
}

// ToScreen implements [driver.Screener].
func (w *Window) ToScreen(p geom.Point) geom.Point {
	w.mu.Lock()
	defer w.mu.Unlock()
	f := w.scale / w.perCoord
	return geom.Pt(w.origin.X+p.X*f, w.origin.Y+p.Y*f)
}

// FromScreen implements [driver.Screener].
func (w *Window) FromScreen(p geom.Point) geom.Point {
	w.mu.Lock()
	defer w.mu.Unlock()
	f := w.scale / w.perCoord
	return geom.Pt((p.X-w.origin.X)/f, (p.Y-w.origin.Y)/f)
}

// hasFocus reports whether the window has the keyboard.
func (w *Window) hasFocus() bool { return w.focused.Load() }

// Transparent implements [driver.Transparent].
func (w *Window) Transparent() bool { return w.transparent }

// Place implements [driver.Placer].
func (w *Window) Place(anchor geom.Rect, size geom.Size) error {
	return w.d.call(func() error {
		if w.closed || w.parent == nil {
			return nil
		}
		f := w.coordsPerLogical()
		ww, wh := max(1, int(size.W*f+0.5)), max(1, int(size.H*f+0.5))
		if cw, ch, err := w.gw.GetSize(); err == nil && (cw != ww || ch != wh) {
			if err := w.gw.SetSize(ww, wh); err != nil {
				return err
			}
		}
		return w.attach(anchor)
	})
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
	x, y, err := w.gw.GetPos()
	if err != nil {
		x, y = 0, 0
	}

	w.mu.Lock()
	w.fbW, w.fbH, w.scale, w.perCoord = fbW, fbH, scale, perCoord
	w.origin = geom.Pt(float32(x), float32(y))
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
	_, _ = gw.SetPosCallback(func(*glfw.Window, int, int) {
		remeasure()
		for _, c := range w.popups {
			_ = c.attach(c.anchor)
		}
	})
	_, _ = gw.SetRefreshCallback(func(*glfw.Window) { w.in.push(driver.Redraw{}) })
	_, _ = gw.SetFocusCallback(func(_ *glfw.Window, focused bool) {
		w.focused.Store(focused)
		w.accessFocus(focused)
		w.in.push(driver.WindowFocus{Focused: focused})
	})
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
	_, _ = gw.SetDropCallback(func(_ *glfw.Window, names []string) {
		// GLFW moves the cursor to where the files were let go first.
		w.in.push(input.Drop{Pos: w.cursor, Paths: names, Time: time.Now()})
	})
	_, _ = gw.SetScrollCallback(func(_ *glfw.Window, x, y float64) {
		w.in.push(input.Scroll{
			Pos:   w.cursor,
			Delta: geom.Pt(float32(x)*scrollLine, float32(y)*scrollLine),
			Mods:  w.mods,
			Time:  time.Now(),
		})
	})
	_, _ = gw.SetKeyCallback(func(gw *glfw.Window, k glfw.Key, _ int, action glfw.Action, mods glfw.ModifierKey) {
		w.mods = modsOf(mods)
		now := time.Now()
		switch action {
		case glfw.Press, glfw.Repeat:
			w.in.push(input.KeyPress{Key: keyOf(k), Mods: w.mods, Repeat: action == glfw.Repeat, Typed: gw.KeyTyped(), Time: now})
		case glfw.Release:
			w.in.push(input.KeyRelease{Key: keyOf(k), Mods: w.mods, Time: now})
		}
	})
	installText(w)
}

// frame is one frame for the render thread: its ops, and the part of
// the window that changed since the frame before.
type frame struct {
	ops    []paint.Op
	damage geom.Rect
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
			if w.pres != nil {
				w.pres.close()
			}
			r.release()
			_ = (*glfw.Window)(nil).MakeContextCurrent()
		}()
	}

	vb := newVBlank(w.gw)
	defer vb.close()

	var last time.Time
	for {
		var f frame
		select {
		case <-w.quit:
			return
		case f = <-w.frames:
		}
		if r != nil {
			w.mu.Lock()
			fbW, fbH, scale, rate := w.fbW, w.fbH, w.scale, w.rate
			readback := w.readback
			w.mu.Unlock()
			if w.pres != nil {
				fbo, err := w.pres.begin(fbW, fbH)
				if err != nil {
					w.fail(err)
				}
				r.windowFBO = fbo
			}
			r.draw(f.ops, f.damage, fbW, fbH, scale)
			if readback != nil {
				if w.pres != nil {
					// The window's texture is upside down for Direct3D;
					// the canvas holds the frame the right way up.
					r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.layers[0].fbo)
				}
				pix := make([]byte, fbW*fbH*4)
				r.gl.ReadPixels(pix, 0, 0, int32(fbW), int32(fbH), gl.RGBA, gl.UNSIGNED_BYTE)
				readback(pix, fbW, fbH)
			}
			synced := vb.wait()
			if w.pres != nil {
				if err := w.pres.present(); err != nil {
					w.fail(err)
				}
			} else if err := w.gw.SwapBuffers(); err != nil {
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
	holder := w.gw
	if w.ctx != nil {
		holder = w.ctx
	}
	if err := holder.MakeContextCurrent(); err != nil {
		return nil, fmt.Errorf("desktop: make context current: %w", err)
	}
	// An error here leaves the swap unpaced, which pace makes up for.
	_ = holder.SwapInterval(swapInterval)
	ctx, err := gl.NewDefaultContext()
	if err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	if err = ctx.LoadFunctions(); err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	r, err := newRenderer(ctx, w.d.isES, &w.d.shared)
	if err != nil {
		return nil, err
	}
	if w.ctx != nil {
		if w.pres, err = w.startPresenter(ctx); err != nil {
			r.release()
			return nil, err
		}
		r.flipWindow = true
	}
	return r, nil
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
