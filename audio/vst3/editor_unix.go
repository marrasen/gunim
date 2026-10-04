//go:build !windows

package vst3

import (
	"encoding/binary"
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// On Linux an editor sits in a window of X11's, and runs on the host's
// loop: the plugin hands the host the files it waits on and its timers.

// platformType is the kind of window an editor attaches to here.
const platformType = "X11EmbedWindowID"

// x is libX11, loaded once an editor opens.
var x struct {
	once sync.Once
	err  error
	dpy  uintptr

	OpenDisplay        func(name uintptr) uintptr
	DefaultRootWindow  func(dpy uintptr) uintptr
	CreateSimpleWindow func(dpy, parent uintptr, x, y int32, w, h, border uint32, borderPixel, bg uintptr) uintptr
	StoreName          func(dpy, win uintptr, name string) int32
	InternAtom         func(dpy uintptr, name string, onlyIfExists int32) uintptr
	SetWMProtocols     func(dpy, win uintptr, atoms *uintptr, n int32) int32
	SelectInput        func(dpy, win uintptr, mask int64) int32
	MapRaised          func(dpy, win uintptr) int32
	ResizeWindow       func(dpy, win uintptr, w, h uint32) int32
	DestroyWindow      func(dpy, win uintptr) int32
	Pending            func(dpy uintptr) int32
	NextEvent          func(dpy uintptr, ev *[24]int64) int32
	Flush              func(dpy uintptr) int32

	deleteWindow uintptr
}

func loadX() error {
	x.once.Do(func() {
		lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			x.err = err
			return
		}
		for name, fn := range map[string]any{
			"XOpenDisplay": &x.OpenDisplay, "XDefaultRootWindow": &x.DefaultRootWindow,
			"XCreateSimpleWindow": &x.CreateSimpleWindow, "XStoreName": &x.StoreName,
			"XInternAtom": &x.InternAtom, "XSetWMProtocols": &x.SetWMProtocols,
			"XSelectInput": &x.SelectInput, "XMapRaised": &x.MapRaised, "XResizeWindow": &x.ResizeWindow,
			"XDestroyWindow": &x.DestroyWindow, "XPending": &x.Pending, "XNextEvent": &x.NextEvent,
			"XFlush": &x.Flush,
		} {
			purego.RegisterLibFunc(fn, lib, name)
		}
		if x.dpy = x.OpenDisplay(0); x.dpy == 0 {
			x.err = errors.New("vst3: no X display")
			return
		}
		x.deleteWindow = x.InternAtom(x.dpy, "WM_DELETE_WINDOW", 0)
		tick(&x, pumpX)
	})
	return x.err
}

// windows are the editors' windows, by X's ID.
var windowsByID = map[uintptr]*window{}

// pumpX takes the events of the editors' windows.
func pumpX() {
	for x.Pending(x.dpy) > 0 {
		var ev [24]int64
		x.NextEvent(x.dpy, &ev)
		b := unsafe.Slice((*byte)(unsafe.Pointer(&ev[0])), 192)
		typ := int32(binary.LittleEndian.Uint32(b))
		id := uintptr(binary.LittleEndian.Uint64(b[32:]))
		switch typ {
		case 33: // ClientMessage
			if w := windowsByID[id]; w != nil && uintptr(binary.LittleEndian.Uint64(b[56:])) == x.deleteWindow {
				w.e.close()
			}
		case 22: // ConfigureNotify
			if w := windowsByID[id]; w != nil {
				cw := int(int32(binary.LittleEndian.Uint32(b[56:])))
				ch := int(int32(binary.LittleEndian.Uint32(b[60:])))
				if cw != w.w || ch != w.h {
					w.w, w.h = cw, ch
					if !w.sizing {
						w.e.resized(cw, ch)
					}
				}
			}
		}
	}
}

// window is an editor's window.
type window struct {
	id     uintptr
	e      *editor
	w, h   int
	sizing bool
}

// newWindow makes a window of w×h, titled title, for e.
func newWindow(title string, w, h int, e *editor) (*window, error) {
	if err := loadX(); err != nil {
		return nil, err
	}
	id := x.CreateSimpleWindow(x.dpy, x.DefaultRootWindow(x.dpy), 0, 0, uint32(w), uint32(h), 0, 0, 0)
	if id == 0 {
		return nil, errors.New("vst3: X made no window")
	}
	x.StoreName(x.dpy, id, title)
	atom := x.deleteWindow
	x.SetWMProtocols(x.dpy, id, &atom, 1)
	x.SelectInput(x.dpy, id, 1<<17) // StructureNotifyMask
	win := &window{id: id, e: e, w: w, h: h}
	windowsByID[id] = win
	x.Flush(x.dpy)
	return win, nil
}

func (w *window) handle() uintptr { return w.id }

func (w *window) show() {
	x.MapRaised(x.dpy, w.id)
	x.Flush(x.dpy)
}

func (w *window) raise() { w.show() }

func (w *window) resize(cw, ch int) {
	w.w, w.h = cw, ch
	x.ResizeWindow(x.dpy, w.id, uint32(cw), uint32(ch))
	x.Flush(x.dpy)
}

func (w *window) destroy() {
	delete(windowsByID, w.id)
	x.DestroyWindow(x.dpy, w.id)
	x.Flush(x.dpy)
}

// The host's run loop, which a Linux editor asks its frame for.

type runLoopObject struct{ object }

type fdHandler struct {
	handler uintptr
	fd      int32
}

type timer struct {
	handler uintptr
	every   time.Duration
	next    time.Time
}

// The run loop's handlers and timers, kept on the plugins' thread.
var (
	fdHandlers []fdHandler
	timers     []*timer
)

var runLoopKind = &kind{iids: []uid{iidRunLoop}, make: func() []uintptr {
	return []uintptr{
		newCallback(func(this, h, fd uintptr) uintptr {
			fdHandlers = append(fdHandlers, fdHandler{h, i32(fd)})
			return resultOf(resultOK)
		}),
		newCallback(func(this, h uintptr) uintptr {
			kept := fdHandlers[:0]
			for _, f := range fdHandlers {
				if f.handler != h {
					kept = append(kept, f)
				}
			}
			fdHandlers = kept
			return resultOf(resultOK)
		}),
		newCallback(func(this, h, ms uintptr) uintptr {
			every := time.Duration(ms) * time.Millisecond
			timers = append(timers, &timer{h, every, time.Now().Add(every)})
			return resultOf(resultOK)
		}),
		newCallback(func(this, h uintptr) uintptr {
			kept := timers[:0]
			for _, t := range timers {
				if t.handler != h {
					kept = append(kept, t)
				}
			}
			timers = kept
			return resultOf(resultOK)
		}),
	}
}}

var runLoop = sync.OnceValue(func() uintptr {
	p := keep(new(runLoopObject), runLoopKind, false)
	tick(&fdHandlers, runPlugins)
	return p
})

// runPlugins runs the handlers of the files ready, and the timers due.
func runPlugins() {
	if len(fdHandlers) > 0 {
		fds := make([]unix.PollFd, len(fdHandlers))
		hs := append([]fdHandler(nil), fdHandlers...)
		for i, f := range hs {
			fds[i] = unix.PollFd{Fd: f.fd, Events: unix.POLLIN}
		}
		if n, _ := unix.Poll(fds, 0); n > 0 {
			for i, f := range fds {
				if f.Revents != 0 {
					call(hs[i].handler, 3, uintptr(hs[i].fd)) // onFDIsSet
				}
			}
		}
	}
	now := time.Now()
	for _, t := range append([]*timer(nil), timers...) {
		if !now.Before(t.next) {
			t.next = now.Add(t.every)
			call(t.handler, 3) // onTimer
		}
	}
}
