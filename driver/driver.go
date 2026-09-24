// Package driver is the seam between gunim and the operating system.
//
// Everything above this line is portable Go. Everything below it knows
// about X11, Win32 and Cocoa, and gunim takes that layer from
// Ebitengine.
//
// Ebitengine 2.10 ships a complete reimplementation of GLFW in pure Go:
// X11 with GLX and EGL on Linux and the BSDs, Win32 with WGL on
// Windows, Cocoa with NSGL on macOS, all of it reached through purego.
// Its window constructor is the real GLFW one,
//
//	CreateWindow(width, height int, title string, monitor *Monitor, share *Window)
//
// so several windows, a chosen monitor and shared GL objects are all
// already possible down there. Ebitengine confines itself to a single
// window one layer up, in internal/ui, and that is the layer gunim
// replaces.
//
// So a gunim driver is a fork of ebiten/internal/glfw with its
// identifiers exported, plus the GL calls to replay a [paint] op list.
// That turns the hardest and least interesting part of the project —
// three platforms' worth of window and context creation, in pure Go —
// from a year of work into a merge.
//
// On Linux that port speaks X11, so a Wayland desktop runs gunim
// through XWayland. For [KindPopup] windows that is the lucky outcome:
// X11 lets a client place a window at an absolute screen position,
// while Wayland keeps a popup anchored to its parent through xdg_popup.
// The API here stays expressible on both by making popups
// anchor-relative.
//
// # Pacing
//
// A window is paced by its own buffer swaps. Pure-Go GL has no vsync
// event to wait on; what it has is SwapBuffers with a swap interval of
// one, which blocks until the display takes the frame. So each window
// gets a render goroutine of its own, locked to an OS thread with
// runtime.LockOSThread because a GL context belongs to one thread. That
// goroutine replays a frame's ops, swaps, and reports on
// [Window.Presented] once the swap returns.
//
// The engine keeps one frame in flight. It draws when it has something
// to show and nothing in flight, then waits for the report before it
// draws again. An idle window has nothing in flight, so the first frame
// after a keystroke is drawn at once. Two
// windows on two monitors swap on separate threads, so each one keeps
// its own monitor's rate.
package driver

import (
	"context"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A Driver owns the connection to the display server.
type Driver interface {
	// Run pumps platform events on the calling goroutine, which must be
	// the main goroutine: every one of the three platforms requires it.
	// Run calls ready once the pump is live and returns when ctx is
	// cancelled or the last window closes. Both of those are a normal
	// end and return nil; an error means the pump itself failed.
	Run(ctx context.Context, ready func()) error

	// NewWindow opens a window. It is safe to call from any goroutine;
	// the driver marshals the request onto the main goroutine.
	//
	// Every window a driver opens shares one group of GL objects, so a
	// glyph atlas or a shader can serve them all.
	NewWindow(o Options) (Window, error)

	// Monitors lists the attached displays.
	Monitors() []Monitor
}

// Options describes a window to open.
type Options struct {
	Title   string
	Size    geom.Size
	Monitor *Monitor
	Kind    Kind
	// Parent, for a popup or utility window, is the window it belongs
	// to. Anchor is where it wants to sit, in Parent's coordinate
	// space, which keeps the request expressible on Wayland as well as
	// on X11.
	Parent Window
	Anchor geom.Point
}

// Kind is what sort of window to open. Each one maps onto a real window
// type in the display server, which is what lets an overlay leave its
// parent's bounds.
type Kind uint8

const (
	// KindNormal is an ordinary top-level window.
	KindNormal Kind = iota
	// KindUtility is a tool window: a palette or an inspector. It floats
	// above its parent and keeps off the taskbar.
	KindUtility
	// KindPopup is a bare, transient overlay: a menu, a tooltip, a combo
	// box list. It extends past its parent's bounds, which is the one
	// thing Ebitengine has to give up by owning a single window.
	KindPopup
)

// A Window is one on-screen surface with its own GL context.
type Window interface {
	// Presented reports each frame handed to Present once it has
	// reached the screen: exactly one Frame per Present, in order.
	//
	// The engine waits for it before drawing the next frame, so the
	// rate the display takes frames at is the rate the window draws
	// at, whatever the application is doing. The engine keeps at most
	// one frame in flight, so a channel with room for one always has
	// room when the render thread reports.
	Presented() <-chan Frame

	// Input carries raw platform input for this window.
	Input() <-chan any

	// Present hands a frame to the render thread and returns at once;
	// the display takes it later. damage is the region that changed,
	// which a driver may use to present just that part of the surface.
	//
	// ops belong to the driver until the matching Frame arrives on
	// Presented; the engine records the next frame into the same
	// buffers only after that.
	Present(ops []paint.Op, damage geom.Rect) error

	// Size is the current size in logical pixels, and Scale is device
	// pixels per logical pixel on the monitor the window is on.
	Size() geom.Size
	Scale() float32

	// RefreshRate is the current monitor's rate in Hz. It follows the
	// window as it is dragged to another display.
	RefreshRate() float64

	Close() error
}

// Frame reports one presented frame.
type Frame struct {
	// Shown is when the frame reached the screen: when the swap
	// returned, or the display's own timestamp for it where the
	// platform has one. The engine predicts the next frame's
	// presentation time from it.
	Shown time.Time
}

// Monitor is an attached display.
type Monitor struct {
	Name        string
	Bounds      geom.Rect
	RefreshRate float64
	Scale       float32
	Primary     bool
}
