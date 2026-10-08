package input

// Cursor is the pointer's shape. A node names one for the part of the
// window it covers; see [github.com/marrasen/gunim.CursorShaper].
type Cursor uint8

// The pointer's shapes, as the platform draws them.
const (
	// CursorArrow is the usual pointer.
	CursorArrow Cursor = iota
	// CursorText is the I-beam, over text that can be selected or typed
	// into.
	CursorText
	// CursorHand is the pointing hand, over a link.
	CursorHand
	// CursorCrosshair is for picking a point.
	CursorCrosshair
	// CursorResizeH is a left and right arrow, over something dragged
	// sideways, such as a divider between two panes.
	CursorResizeH
	// CursorResizeV is an up and down arrow, over something dragged up
	// and down.
	CursorResizeV
	// CursorMove is four arrows, over something dragged anywhere.
	CursorMove
	// CursorNotAllowed says what the pointer is over refuses it.
	CursorNotAllowed
	// CursorResizeNWSE and CursorResizeNESW are the diagonal arrows,
	// over a window's corners.
	CursorResizeNWSE
	CursorResizeNESW
	// CursorNone hides the pointer, for a node that draws its own in
	// its place, such as a magnifier under a colour picker.
	CursorNone
	// CursorInherit leaves the shape to the nodes around the one that
	// names it.
	CursorInherit
)
