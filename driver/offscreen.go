package driver

import (
	"slices"
	"sync"
	"time"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// Offscreen returns a window backed by no display.
//
// It keeps the op list from the last frame instead of drawing it, which
// makes it the driver for tests, for golden-image comparisons once a
// rasteriser exists, and for running an interface headlessly on a build
// machine. Its Presented and Input channels stay quiet, so whoever
// holds it decides when a frame reaches the "screen".
func Offscreen(size geom.Size) *OffscreenWindow {
	return &OffscreenWindow{
		size:      size,
		scale:     1,
		rate:      60,
		presented: make(chan Frame),
		input:     make(chan any),
	}
}

// An OffscreenWindow records frames rather than showing them.
type OffscreenWindow struct {
	mu    sync.Mutex
	size  geom.Size
	scale float32
	rate  float64
	ops   []paint.Op
	// damage is the part of the window the last frame changed.
	damage geom.Rect
	// cursor is the pointer's shape last set.
	cursor input.Cursor
	clip   string
	// anchor is where a popup was last attached, and origin where the
	// window sits on its pretend screen.
	anchor geom.Rect
	origin geom.Point
	// out holds the files of each drag handed to other programs.
	out [][]string
	// access says whether something listens for the accessibility tree,
	// and tree is the last one published.
	access bool
	tree   *access.Tree
	// hidden says the window is hidden, kept to show again.
	hidden bool

	// title, full and attention are what the application last asked
	// of the window's frame.
	title     string
	full      bool
	attention int

	presented chan Frame
	input     chan any
}

// SetTitle implements [Titler].
func (w *OffscreenWindow) SetTitle(title string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.title = title
}

// Title returns the title last set.
func (w *OffscreenWindow) Title() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.title
}

// SetFullScreen implements [FullScreener].
func (w *OffscreenWindow) SetFullScreen(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.full = on
}

// FullScreen implements [FullScreener].
func (w *OffscreenWindow) FullScreen() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.full
}

// RequestAttention implements [Attender].
func (w *OffscreenWindow) RequestAttention() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attention++
}

// Attention returns how many times attention was asked for.
func (w *OffscreenWindow) Attention() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.attention
}

// Presented implements [Window]. It stays quiet until [OffscreenWindow.Tick].
func (w *OffscreenWindow) Presented() <-chan Frame { return w.presented }

// Input implements [Window]. Feed it with [OffscreenWindow.Post].
func (w *OffscreenWindow) Input() <-chan any { return w.input }

// Tick reports the frame in flight as shown, blocking until the window
// takes the report.
//
// A real driver reports each frame as its swap returns. This one waits
// for the test, so a test decides exactly when the display is ready for
// the next frame.
func (w *OffscreenWindow) Tick() { w.presented <- Frame{Shown: time.Now()} }

// Post delivers a platform event, blocking until the window reads it.
func (w *OffscreenWindow) Post(ev any) { w.input <- ev }

// Present implements [Window] by keeping the op list and the damage.
func (w *OffscreenWindow) Present(ops []paint.Op, damage geom.Rect) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	// The engine reuses its buffers once the frame is reported shown,
	// so keep a copy for Ops to return.
	w.ops = slices.Clone(ops)
	w.damage = damage
	return nil
}

// SetCursor implements [CursorSetter].
func (w *OffscreenWindow) SetCursor(c input.Cursor) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cursor = c
}

// Cursor returns the pointer's shape last set.
func (w *OffscreenWindow) Cursor() input.Cursor {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cursor
}

// Damage returns the part of the window the last frame changed, in
// logical pixels: empty when it changed nothing.
func (w *OffscreenWindow) Damage() geom.Rect {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.damage
}

// Ops returns the commands recorded by the last frame.
func (w *OffscreenWindow) Ops() []paint.Op {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ops
}

// Resize changes the window's size for the next frame.
func (w *OffscreenWindow) Resize(size geom.Size) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.size = size
}

// Place implements [Placer] by taking the size and keeping the anchor.
func (w *OffscreenWindow) Place(anchor geom.Rect, size geom.Size) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.anchor, w.size = anchor, size
	return nil
}

// ListenForAccess makes the window want its accessibility tree, as a
// screen reader would, and keep the last one published.
func (w *OffscreenWindow) ListenForAccess() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.access = true
}

// AccessWanted implements [AccessPublisher].
func (w *OffscreenWindow) AccessWanted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.access
}

// PublishAccess implements [AccessPublisher].
func (w *OffscreenWindow) PublishAccess(t *access.Tree) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tree = t
}

// AccessTree returns the last accessibility tree published.
func (w *OffscreenWindow) AccessTree() *access.Tree {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tree
}

// DragOut implements [DragOuter] by keeping the paths; a test ends the
// drag by sending [DragOutEnded] through Input.
func (w *OffscreenWindow) DragOut(paths []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.out = append(w.out, paths)
	return nil
}

// DraggedOut returns the files of each drag handed to other programs.
func (w *OffscreenWindow) DraggedOut() [][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out
}

// SetOrigin puts the window's top left corner at p on its pretend
// screen, for tests that drag between windows.
func (w *OffscreenWindow) SetOrigin(p geom.Point) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.origin = p
}

// ToScreen implements [Screener].
func (w *OffscreenWindow) ToScreen(p geom.Point) geom.Point {
	w.mu.Lock()
	defer w.mu.Unlock()
	return p.Add(w.origin)
}

// FromScreen implements [Screener].
func (w *OffscreenWindow) FromScreen(p geom.Point) geom.Point {
	w.mu.Lock()
	defer w.mu.Unlock()
	return p.Sub(w.origin)
}

// Anchor returns where the window was last attached, for a popup.
func (w *OffscreenWindow) Anchor() geom.Rect {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.anchor
}

// Size implements [Window].
func (w *OffscreenWindow) Size() geom.Size {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// Scale implements [Window].
func (w *OffscreenWindow) Scale() float32 { return w.scale }

// RefreshRate implements [Window].
func (w *OffscreenWindow) RefreshRate() float64 { return w.rate }

// Clipboard implements [Window] with a clipboard of the window's own.
func (w *OffscreenWindow) Clipboard() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.clip, nil
}

// SetClipboard implements [Window].
func (w *OffscreenWindow) SetClipboard(s string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clip = s
	return nil
}

// Close implements [Window].
func (w *OffscreenWindow) Close() error { return nil }

// Hide implements [Recycler].
func (w *OffscreenWindow) Hide() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hidden = true
	return nil
}

// Show implements [Recycler].
func (w *OffscreenWindow) Show() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hidden = false
	return nil
}

// Hidden reports whether the window is hidden, kept to show again.
func (w *OffscreenWindow) Hidden() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hidden
}
