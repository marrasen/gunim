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

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
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
	opened  bool
	quit    bool
	// stayOpen keeps the event loop running after the last window
	// closes, so tests can open one window after another.
	stayOpen bool
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
	// The main thread pumps every window's events, and init has locked
	// it for life.
	raiseThread()
	return &Driver{isES: probe.IsES(), windows: map[*glfw.Window]*Window{}}, nil
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
	return errors.Join(err, glfw.Terminate())
}

// post queues f for the main thread and wakes the event loop.
func (d *Driver) post(f func()) bool {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return false
	}
	d.tasks = append(d.tasks, f)
	d.mu.Unlock()
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

// openWindow opens a window on the main thread and starts its render
// thread.
func (d *Driver) openWindow(o driver.Options) (*Window, error) {
	if err := glfw.DefaultWindowHints(); err != nil {
		return nil, err
	}
	if err := d.setContextHints(); err != nil {
		return nil, err
	}
	hints := [][2]int{
		{int(glfw.Visible), glfw.False},
		{int(glfw.DoubleBuffer), glfw.True},
		// Size the window in logical pixels on a scaled monitor.
		{int(glfw.ScaleToMonitor), glfw.True},
	}
	switch o.Kind {
	case driver.KindPopup:
		hints = append(hints,
			[2]int{int(glfw.Decorated), glfw.False},
			[2]int{int(glfw.Floating), glfw.True},
			[2]int{int(glfw.FocusOnShow), glfw.False})
	case driver.KindUtility:
		hints = append(hints, [2]int{int(glfw.Floating), glfw.True})
	case driver.KindNormal:
	}
	for _, h := range hints {
		if err := glfw.WindowHint(glfw.Hint(h[0]), h[1]); err != nil {
			return nil, err
		}
	}

	var share *glfw.Window
	if s, ok := o.Share.(*Window); ok {
		share = s.gw
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
	if err := w.place(o); err != nil {
		_ = gw.Destroy()
		return nil, err
	}
	w.install()
	if err := gw.Show(); err != nil {
		_ = gw.Destroy()
		return nil, err
	}
	w.measure()
	d.windows[gw] = w
	d.opened = true
	go w.render()
	return w, nil
}
