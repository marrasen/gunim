package themeedit

import "github.com/marrasen/gunim/theme"

// The editor's tokens. Its colours are the widgets' own, so it takes the
// light theme from [widget.Light] with no more values.
var (
	// Width and Height are the editor's size where it is given no
	// bound, as in a dialog that grows to fit it.
	Width  = theme.Length("themeedit.width", 640)
	Height = theme.Length("themeedit.height", 520)
	// RowGap is the room between rows, and SectionGap between sections.
	RowGap     = theme.Length("themeedit.row.gap", 12)
	SectionGap = theme.Length("themeedit.section.gap", 24)
	// NamesGap is the room between a row's name and the words under it.
	NamesGap = theme.Length("themeedit.names.gap", 2)
	// TitleSize is the size of a section's title and of a group's in
	// All values.
	TitleSize = theme.Length("themeedit.title.size", 16)
	// DetailSize is the size of a field's sentence, and KeySize of a
	// token's key.
	DetailSize = theme.Length("themeedit.detail.size", 12)
	KeySize    = theme.Length("themeedit.key.size", 12)
	// NumberWidth is the width of a field for a length or a number, and
	// SideWidth of each of the four for insets.
	NumberWidth = theme.Length("themeedit.number.width", 96)
	SideWidth   = theme.Length("themeedit.side.width", 64)
	// SliderWidth is the width of the slider beside a number.
	SliderWidth = theme.Length("themeedit.slider.width", 140)
	// SpringWidth is the width of a spring's two sliders, and
	// PreviewWidth of the track its dot moves along.
	SpringWidth  = theme.Length("themeedit.spring.width", 300)
	PreviewWidth = theme.Length("themeedit.preview.width", 120)
	// PreviewDot is the size of the dot that shows a spring's motion.
	PreviewDot = theme.Length("themeedit.preview.dot", 12)
)
