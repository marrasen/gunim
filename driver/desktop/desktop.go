//go:build linux || windows || darwin

// Package desktop is gunim's driver for desktop operating systems:
// Linux and the BSDs through X11, Windows through Win32, macOS through
// Cocoa. It is built on a pure-Go port of GLFW taken from Ebitengine,
// so it needs no cgo.
//
// GLFW insists that windows are created and events pumped on the main
// thread, so [Driver.Run] runs there and everything else reaches it
// through a task queue. Each window renders on a thread of its own,
// which is what lets two windows on two monitors keep two refresh rates.
package desktop

import (
	"context"
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
)

// init pins the main goroutine to the main thread, which is where
// GLFW has to run.
func init() { runtime.LockOSThread() }

// errStopped is returned by calls that arrive after the event loop has
// ended.
var errStopped = errors.New("desktop: the event loop has stopped")

// Driver is the desktop implementation of [driver.Driver].
type Driver struct {
	// isES records whether the GL library loaded is OpenGL ES, which
	// decides the context to ask GLFW for and the shader dialect.
	isES bool

	mu      sync.Mutex
	tasks   []func()
	stopped bool

	// The fields below belong to the main thread.
	windows map[*glfw.Window]*Window
	// cursors holds the pointer shapes made so far.
	cursors map[input.Cursor]*glfw.Cursor
	// shareRoot is a hidden window whose context no thread ever makes
	// current. Every window's context shares with it, which puts them
	// all in one share group without sharing with a context that a render
	// thread holds: NVIDIA's driver refuses that.
	shareRoot *glfw.Window
	opened    bool
	quit      bool
	// stayOpen keeps the event loop running after the last window
	// closes, so tests can open one window after another.
	stayOpen bool

	// shared is what every window's renderer shares.
	shared shared
	// dxgi is set where windows present through DXGI; see
	// present_windows.go.
	dxgi bool
	// acc is the connection to assistive technology.
	acc driverAccess //nolint:unused // used on Linux
}

// Open initialises GLFW and loads the GL library. Call it on the main
// goroutine.
func Open() (*Driver, error) {
	if err := glfw.Init(); err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	probe, err := gl.NewDefaultContext()
	if err != nil {
		_ = glfw.Terminate()
		return nil, fmt.Errorf("desktop: %w", err)
	}
	raiseProcess()
	// The main thread pumps every window's events, and init has locked
	// it for life.
	raiseThread()
	d := &Driver{isES: probe.IsES(), windows: map[*glfw.Window]*Window{}}
	d.dxgi = d.presentsThroughDXGI()
	return d, nil
}

// Run implements [driver.Driver]. It pumps events on the calling
// goroutine, which must be the main one, until ctx ends or the last
// window has closed.
func (d *Driver) Run(ctx context.Context, ready func()) error {
	stop := context.AfterFunc(ctx, func() { d.post(func() { d.quit = true }) })
	defer stop()

	ready()
	var err error
	for !d.quit {
		if err = glfw.WaitEvents(); err != nil {
			err = fmt.Errorf("desktop: %w", err)
			break
		}
		d.runTasks()
		if d.opened && len(d.windows) == 0 && !d.stayOpen {
			break
		}
	}

	// Refuse new work, finish what was already queued, and take down
	// whatever windows are still open.
	d.mu.Lock()
	d.stopped = true
	d.mu.Unlock()
	d.runTasks()
	for _, w := range d.windows {
		w.shutdown()
	}
	if d.shareRoot != nil {
		_ = d.shareRoot.Destroy()
		d.shareRoot = nil
	}
	return errors.Join(err, glfw.Terminate())
}

// post queues f for the main thread and wakes the event loop.
//
// The wake-up happens under the lock that Run takes to stop, so Run
// cannot terminate GLFW while a wake-up is on its way.
func (d *Driver) post(f func()) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return false
	}
	d.tasks = append(d.tasks, f)
	_ = glfw.PostEmptyEvent()
	return true
}

// call runs f on the main thread and waits for its result. It must be
// called from any goroutine except the main one.
func (d *Driver) call(f func() error) error {
	done := make(chan error, 1)
	if !d.post(func() { done <- f() }) {
		return errStopped
	}
	return <-done
}

func (d *Driver) runTasks() {
	for {
		d.mu.Lock()
		tasks := d.tasks
		d.tasks = nil
		d.mu.Unlock()
		if len(tasks) == 0 {
			return
		}
		for _, f := range tasks {
			f()
		}
	}
}

// NewWindow implements [driver.Driver].
func (d *Driver) NewWindow(o driver.Options) (driver.Window, error) {
	var w *Window
	err := d.call(func() error {
		var err error
		w, err = d.openWindow(o)
		return err
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

// Monitors implements [driver.Driver].
func (d *Driver) Monitors() []driver.Monitor {
	var out []driver.Monitor
	_ = d.call(func() error {
		ms, err := glfw.GetMonitors()
		if err != nil {
			return err
		}
		primary, _ := glfw.GetPrimaryMonitor()
		for _, m := range ms {
			info, ok := monitorInfo(m)
			if !ok {
				continue
			}
			info.Primary = m == primary
			out = append(out, info)
		}
		return nil
	})
	return out
}

// monitorInfo describes m in screen coordinates. It runs on the main
// thread.
func monitorInfo(m *glfw.Monitor) (driver.Monitor, bool) {
	x, y, err := m.GetPos()
	if err != nil {
		return driver.Monitor{}, false
	}
	mode, err := m.GetVideoMode()
	if err != nil || mode == nil {
		return driver.Monitor{}, false
	}
	name, _ := m.GetName()
	scale, _, _ := m.GetContentScale()
	if scale <= 0 {
		scale = 1
	}
	return driver.Monitor{
		Name:        name,
		Bounds:      geom.Rc(float32(x), float32(y), float32(mode.Width), float32(mode.Height)),
		RefreshRate: float64(mode.RefreshRate),
		Scale:       scale,
	}, true
}

// contextWindow opens the hidden window that holds a DXGI window's
// context, sharing with share. It runs on the main thread.
func (d *Driver) contextWindow(share *glfw.Window) (*glfw.Window, error) {
	if err := glfw.DefaultWindowHints(); err != nil {
		return nil, err
	}
	if err := d.setContextHints(); err != nil {
		return nil, err
	}
	if err := glfw.WindowHint(glfw.Visible, glfw.False); err != nil {
		return nil, err
	}
	ctx, err := glfw.CreateWindow(1, 1, "", nil, share)
	if err != nil {
		return nil, fmt.Errorf("desktop: create context window: %w", err)
	}
	return ctx, nil
}

// setContextHints asks GLFW for a context the renderer can use: OpenGL
// 3.2 core, or OpenGL ES 3.0 where only ES was found. It runs on the
// main thread.
func (d *Driver) setContextHints() error {
	hints := [][2]int{{int(glfw.ContextVersionMajor), 3}}
	if d.isES {
		hints = append(hints,
			[2]int{int(glfw.ClientAPI), glfw.OpenGLESAPI},
			[2]int{int(glfw.ContextVersionMinor), 0},
			[2]int{int(glfw.ContextCreationAPI), glfw.EGLContextAPI})
	} else {
		hints = append(hints,
			[2]int{int(glfw.ClientAPI), glfw.OpenGLAPI},
			[2]int{int(glfw.ContextVersionMinor), 2},
			[2]int{int(glfw.OpenGLProfile), glfw.OpenGLCoreProfile},
			[2]int{int(glfw.OpenGLForwardCompat), glfw.True})
	}
	for _, h := range hints {
		if err := glfw.WindowHint(glfw.Hint(h[0]), h[1]); err != nil {
			return err
		}
	}
	return nil
}

// shareGroup returns the hidden window every window's context shares
// with, creating it on first use with the context hints already set. It
// runs on the main thread.
func (d *Driver) shareGroup() (*glfw.Window, error) {
	if d.shareRoot != nil {
		return d.shareRoot, nil
	}
	if err := glfw.WindowHint(glfw.Visible, glfw.False); err != nil {
		return nil, err
	}
	root, err := glfw.CreateWindow(1, 1, "", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("desktop: create share context: %w", err)
	}
	d.shareRoot = root
	return root, nil
}

// openWindow opens a window on the main thread and starts its render
// thread.
func (d *Driver) openWindow(o driver.Options) (*Window, error) {
	if err := glfw.DefaultWindowHints(); err != nil {
		return nil, err
	}
	if err := d.setContextHints(); err != nil {
		return nil, err
	}
	// Every window shares GL objects with the one share group.
	share, shareErr := d.shareGroup()
	if shareErr != nil {
		return nil, shareErr
	}
	share0 := share
	hints := [][2]int{
		{int(glfw.Visible), glfw.False},
		{int(glfw.DoubleBuffer), glfw.True},
		// Size the window in logical pixels on a scaled monitor.
		{int(glfw.ScaleToMonitor), glfw.True},
	}
	if d.dxgi {
		// The window shows what DXGI presents and holds no context of
		// its own; a hidden window holds it. With no redirection surface,
		// nothing the window would otherwise draw shows under the frame.
		hints = append(hints,
			[2]int{int(glfw.ClientAPI), glfw.NoAPI},
			[2]int{int(glfw.Win32NoRedirectionBitmap), glfw.True})
		share = nil
	}
	switch o.Kind {
	case driver.KindPopup:
		hints = append(hints,
			[2]int{int(glfw.Popup), glfw.True},
			// Cocoa has no popup hint yet; there, a popup is a borderless
			// window floating above the rest.
			[2]int{int(glfw.Floating), glfw.True},
			[2]int{int(glfw.Decorated), glfw.False},
			[2]int{int(glfw.FocusOnShow), glfw.False})
		if !d.dxgi {
			// DXGI's frames carry alpha; OpenGL's need a transparent
			// framebuffer to.
			hints = append(hints, [2]int{int(glfw.TransparentFramebuffer), glfw.True})
		}
	case driver.KindUtility:
		hints = append(hints, [2]int{int(glfw.Floating), glfw.True})
	case driver.KindNormal:
	}
	if o.Passthrough {
		hints = append(hints, [2]int{int(glfw.MousePassthrough), glfw.True})
	}
	for _, h := range hints {
		if err := glfw.WindowHint(glfw.Hint(h[0]), h[1]); err != nil {
			return nil, err
		}
	}

	size := o.Size
	if size.W <= 0 || size.H <= 0 {
		size = geom.Sz(800, 600)
	}
	gw, err := glfw.CreateWindow(int(size.W), int(size.H), o.Title, nil, share)
	if err != nil {
		return nil, fmt.Errorf("desktop: create window: %w", err)
	}

	w := newWindow(d, gw)
	if d.dxgi {
		ctx, err := d.contextWindow(share0)
		if err != nil {
			_ = gw.Destroy()
			return nil, err
		}
		w.ctx = ctx
	}
	if o.Kind == driver.KindPopup {
		t, err := gw.GetAttrib(glfw.TransparentFramebuffer)
		w.transparent = d.dxgi || (err == nil && t == glfw.True)
	}
	if err := w.position(o); err != nil {
		_ = gw.Destroy()
		return nil, err
	}
	if p, ok := o.Parent.(*Window); ok {
		w.textRendering = o.Text.Or(p.textRendering)
	} else {
		sys, err := systemText()
		if err != nil {
			_ = gw.Destroy()
			return nil, err
		}
		w.textRendering = o.Text.Or(sys)
	}
	if len(o.Icons) > 0 {
		// macOS has no window icons, and says so; a window without one
		// is no reason to fail.
		_ = gw.SetIcon(o.Icons)
	}
	w.install()
	if o.Chromeless && o.Kind == driver.KindNormal {
		w.setChromeless()
	}
	w.accessOpen()
	if x, y, err := gw.GetPos(); err == nil {
		fw, fh, _ := gw.GetFramebufferSize()
		w.debugf("made at %d,%d, framebuffer %dx%d, kind %d", x, y, fw, fh, o.Kind)
	}
	if !o.Hidden {
		if o.Kind == driver.KindNormal {
			// Kept off the screen until its first frame is there; see
			// cloak. A frame that never comes uncovers it all the same.
			cloak(w, true)
			w.covered = true
			time.AfterFunc(time.Second, w.uncover)
		}
		if err := gw.Show(); err != nil {
			_ = gw.Destroy()
			return nil, err
		}
		w.debugf("shown")
	}
	w.measure()
	d.windows[gw] = w
	d.opened = true
	go w.render()
	return w, nil
}
