package driver

import (
	"errors"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A CursorSetter is a [Window] that can change the pointer's shape
// while the pointer is over it.
type CursorSetter interface {
	SetCursor(c input.Cursor)
}

// ErrNoDriver is returned when no platform driver is built in for the
// operating system gunim is running on.
var ErrNoDriver = errors.New("driver: no platform driver for this operating system")

// A Raiser is a [Window] that can raise the priority of the calling
// thread. The engine calls RaiseThread from its UI goroutine, which it
// keeps on one thread for life, so the work of making frames stays on
// time when other programs load the CPU.
type Raiser interface {
	RaiseThread()
}

// A TextInputter is a [Window] that can be told when the application is
// taking text, so the platform's input method composes into it: it
// holds the key presses it needs while the user composes, and reports
// the composition as [github.com/marrasen/gunim/input.Composing] and the
// result as [github.com/marrasen/gunim/input.TextInput]. Setting it false ends any composition.
type TextInputter interface {
	SetTextInput(active bool)
}

// A CaretPlacer is a [Window] that can tell the platform's input method
// where the text caret is, in logical pixels of window space, so its
// candidate window opens beside the text being composed.
type CaretPlacer interface {
	SetTextCaret(r geom.Rect)
}

// Redraw is sent on [Window.Input] when the window needs drawing again
// with no input behind it: after a resize, a move to another monitor,
// or the display server asking for the contents back.
type Redraw struct{}

// A Placer is a popup [Window] that can move and resize: to fit new
// content, or to follow its anchor. anchor is in the parent's logical
// space, as in [Options], and size is in logical pixels.
type Placer interface {
	Place(anchor geom.Rect, size geom.Size) error
}

// A Recycler is a popup [Window] that can be hidden and shown again,
// so a popup that opens often, such as a menu or a palette, need not
// make a window and its surface each time: on Windows that is the
// larger part of opening one. Hide takes the window off the screen,
// keeping it; Show puts it back where Place last put it.
type Recycler interface {
	Hide() error
	Show() error
}

// WindowFocus is sent on [Window.Input] when the window gains or loses
// the keyboard.
type WindowFocus struct{ Focused bool }

// A Positioner is a [Window] that says where its content sits on the
// screen: the top-left corner of what it draws, in device pixels, so a
// screenshot can put a popup where it shows over its parent.
type Positioner interface {
	ContentOrigin() (image.Point, error)
}

// Shooter is a [Window] that can hand over what it draws: fn receives
// the next frame's pixels, the right way up, on a goroutine of the
// driver's. A driver that draws nothing, such as the offscreen one,
// does not implement it.
type Shooter interface {
	Shoot(fn func(*image.RGBA))
}

// CloseAsked is sent on [Window.Input] when the user asks to close the
// window: its close button, or the system's keys for closing one. The
// window stays open, for the engine to close or keep.
type CloseAsked struct{}

// A Backgrounder is a [Window] the engine tells the colour under its
// frame: what shows where the frame paints nothing and its first op
// leaves the window uncovered. Its alpha is how opaque that is, as for
// a window fading in, and zero leaves the window clear there.
type Backgrounder interface {
	SetBackground(c color.NRGBA)
}

// A Fader is a [Window] the engine tells how opaque it is drawing the
// window, below 1 as the window fades in as it opens or out as it
// leaves, and how large, about its middle, as it grows and shrinks
// meanwhile. What the driver draws round the window, such as a shadow,
// fades and shrinks with it; what the system draws, which cannot, the
// driver can hide.
type Fader interface {
	SetFade(opacity, scale float32)
}

// An Outliner is a [Window] that says what shape it shows: the radius its
// corners are cut to and the edge it leaves for a border, both in logical
// pixels and 0 for none, and whether it can show what is behind it. The
// engine draws a fading window's background inside that shape, so the
// window fades and shrinks whole.
type Outliner interface {
	Outline() (radius, edge float32, blends bool)
}

// A Transparent is a [Window] that can say whether it shows what is
// behind it wherever nothing is painted. A popup is transparent where
// the display server can blend windows.
type Transparent interface {
	Transparent() bool
}

// A Screener is a [Window] that knows where it is on the screen. It
// turns a point in its own logical space into screen coordinates, and
// back, so a drag can go from one window to another.
type Screener interface {
	ToScreen(p geom.Point) geom.Point
	FromScreen(p geom.Point) geom.Point
}

// A DragOuter is a [Window] that can hand a drag the pointer is
// carrying, with a button down in it, to other programs, as files.
// DragOut returns at once, and the window reports the end on
// [Window.Input] as [DragOutEnded].
type DragOuter interface {
	DragOut(paths []string) error
}

// DragOutEnded is sent on [Window.Input] when a drag handed to other
// programs ends. Taken says whether a program took the drop.
type DragOutEnded struct {
	Taken bool
	// Back says the pointer came back over a window of the application with the button still down, so the window
	// that started the drag carries it on, and At is where the pointer is, in the window's logical space.
	Back bool
	At   geom.Point
}

// An AccessPublisher is a [Window] that tells assistive technology,
// such as a screen reader, what the window holds. AccessWanted reports
// whether anything is listening, so the engine gathers the tree only
// then. PublishAccess takes the tree as the last frame drew it; it
// belongs to the publisher from then on. A screen reader's requests
// come back on [Window.Input] as [access.Request].
type AccessPublisher interface {
	AccessWanted() bool
	PublishAccess(t *access.Tree)
}

// A Titler is a [Window] whose title the application can change, as
// the platform shows it on the title bar and in the task switcher.
type Titler interface {
	SetTitle(title string)
}

// A FullScreener is a [Window] that can fill its monitor, without the
// platform's frame, and go back to the size and place it had.
type FullScreener interface {
	SetFullScreen(on bool)
	FullScreen() bool
}

// An Attender is a [Window] that can ask for the user's attention
// without taking the keyboard, as a terminal does for a bell in a
// window the user is not looking at: the platform flashes it in the
// task bar, or bounces it in the dock.
type Attender interface {
	RequestAttention()
}

// A Framer is a [Window] opened with [Options.Chromeless]: its
// application draws the title bar, and the system goes on moving,
// sizing and maximizing it as it would by its own.
type Framer interface {
	// Chromeless reports whether the system's title bar is gone. It is
	// false where a system keeps its own, as macOS does for now, and
	// the application then draws none.
	Chromeless() bool
	// SetTitleBar says which parts of the window are title bar with
	// nothing on them, where a press moves the window and a double
	// click maximizes it, and where the maximize button is, in logical
	// pixels in the window's space. The system asks about a point at
	// once, on its own thread, and is answered from these.
	SetTitleBar(caption []geom.Rect, maximize geom.Rect)
	// NativeFrame reports whether the system moves and sizes the window
	// from the title bar and the edges itself, as Windows does. Where it
	// does not, the engine starts moves and resizes with StartMove and
	// StartResize, and maximizes on a double click.
	NativeFrame() bool
	StartMove() error
	StartResize(e Edge) error
	Minimize() error
	SetMaximized(on bool) error
	Maximized() bool
}

// A Pinner is a [Window] that can be kept above other windows.
type Pinner interface {
	SetPinned(on bool) error
}

// MoveStarted is sent on [Window.Input] when the user starts moving or sizing the window by its frame or its title
// bar, where the system takes the press that starts it for itself. It closes what a press outside would.
type MoveStarted struct{}

// A PopupRoomer is a [Window] that says how much room the screen leaves round a rectangle in its own logical space,
// for a popup attached there: to the edges of the work area of the monitor under it.
type PopupRoomer interface {
	PopupRoom(anchor geom.Rect) Room
}

// Room is the room, in logical pixels, from a popup's anchor to the edges of the screen's work area: below its bottom
// and above its top, and right and left of its left edge, where a popup starts.
type Room struct {
	Below, Above, Left, Right float32
}

// NoRoomLimit is the room where nothing says where the screen ends.
var NoRoomLimit = Room{
	Below: float32(math.Inf(1)), Above: float32(math.Inf(1)), Left: float32(math.Inf(1)), Right: float32(math.Inf(1)),
}

// Border is the thin line round the edge of a window opened with [Options.Chromeless], where the platform draws one,
// as Windows 11 does. Its zero value is the system's own.
type Border struct {
	// Color is the line's colour while the window is active. Its zero value is the system's.
	Color color.NRGBA
	// Inactive is its colour while another window is active. Its zero value is Color.
	Inactive color.NRGBA
	// Width is the line's width in logical pixels, taken from the window's edge. Its zero value is one. Where the
	// system draws the line, it is always one pixel wide.
	Width float32
	// None draws no line, and the window's content reaches its edge.
	None bool
	// Transition is how long a change of colour or width takes, eased, whether the application changes the border or
	// the window is activated. Its zero value is [BorderTransition], and a negative one changes at once.
	Transition time.Duration
}

// BorderTransition is how long a [Border] takes to change when its Transition is zero.
const BorderTransition = 150 * time.Millisecond

// A Borderer is a [Window] whose [Border] the application can set.
type Borderer interface {
	SetBorder(b Border)
}

// Edge is an edge or corner of a window, to size it by.
type Edge uint8

// The edges and corners, clockwise from the top left.
const (
	EdgeTopLeft Edge = iota
	EdgeTop
	EdgeTopRight
	EdgeRight
	EdgeBottomRight
	EdgeBottom
	EdgeBottomLeft
	EdgeLeft
)

// WindowMaximized is sent on [Window.Input] when the window is
// maximized or restored.
type WindowMaximized struct{ Maximized bool }
