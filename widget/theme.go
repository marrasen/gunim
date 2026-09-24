package widget

import (
	"image/color"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/theme"
)

// Tokens shared by every widget. Their defaults make the dark theme.
var (
	// Ink is the colour of text.
	Ink = theme.Color("ink", color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff})
	// Accent marks focus and selection.
	Accent = theme.Color("accent", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff})
	// TextSize is the body text size.
	TextSize = theme.Length("text.size", 14)

	// Quick is the motion for direct feedback: hover, press, focus.
	Quick = theme.Spring("motion.quick", anim.Snappy)
	// Settle is the motion for things coming to rest or going away.
	Settle = theme.Spring("motion.settle", anim.Gentle)
	// Bounce is the motion for things arriving, with a visible
	// overshoot.
	Bounce = theme.Spring("motion.bounce", anim.Bouncy)
)

// Button tokens.
var (
	ButtonFill    = theme.Color("button.fill", color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff})
	ButtonHover   = theme.Color("button.hover", color.NRGBA{R: 0x3d, G: 0x45, B: 0x58, A: 0xff})
	ButtonRadius  = theme.Length("button.radius", 8)
	ButtonPadding = theme.Length("button.padding", 16)
	ButtonHeight  = theme.Length("button.height", 36)
	// ButtonSquash is how far a press shrinks the button, as a fraction
	// of its size.
	ButtonSquash = theme.Length("button.squash", 0.035)
)

// Dialog tokens.
var (
	DialogFill      = theme.Color("dialog.fill", color.NRGBA{R: 0x1d, G: 0x20, B: 0x28, A: 0xff})
	DialogBorder    = theme.Color("dialog.border", color.NRGBA{R: 0x3a, G: 0x40, B: 0x50, A: 0xff})
	DialogShadow    = theme.Color("dialog.shadow", color.NRGBA{A: 0x80})
	Scrim           = theme.Color("dialog.scrim", color.NRGBA{A: 0x99})
	DialogRadius    = theme.Length("dialog.radius", 14)
	DialogPadding   = theme.Length("dialog.padding", 20)
	DialogTitleSize = theme.Length("dialog.title.size", 17)
	DialogWidth     = theme.Length("dialog.width", 420)
	DialogHeight    = theme.Length("dialog.height", 200)
	DialogGap       = theme.Length("dialog.gap", 10)
	// DialogBackdrop is how far the scrim blurs what is behind it.
	DialogBackdrop = theme.Length("dialog.backdrop", 14)
)

// List tokens.
var (
	// ListSpacing is the gap between rows.
	ListSpacing = theme.Length("list.spacing", 6)
	// RowRadius rounds a row's clip.
	RowRadius = theme.Length("list.row.radius", 4)
)

// Light is a light theme. It changes more than colour: padding and
// radii grow and motion softens, so switching to it shows every kind of
// token animating.
func Light() theme.Theme {
	return theme.Make("light",
		theme.Set(Ink, color.NRGBA{R: 0x1c, G: 0x20, B: 0x28, A: 0xff}),
		theme.Set(Accent, color.NRGBA{R: 0x2f, G: 0x6f, B: 0xe0, A: 0xff}),
		theme.Set(TextSize, 15),
		theme.Set(Quick, anim.Spring{Response: 0.35, Damping: 0.9}),
		theme.Set(Bounce, anim.Spring{Response: 0.5, Damping: 0.7}),
		theme.Set(ButtonFill, color.NRGBA{R: 0xe4, G: 0xe8, B: 0xf0, A: 0xff}),
		theme.Set(ButtonHover, color.NRGBA{R: 0xd2, G: 0xda, B: 0xe8, A: 0xff}),
		theme.Set(ButtonRadius, 18),
		theme.Set(ButtonPadding, 22),
		theme.Set(ButtonHeight, 40),
		theme.Set(DialogFill, color.NRGBA{R: 0xfa, G: 0xfb, B: 0xfd, A: 0xff}),
		theme.Set(DialogBorder, color.NRGBA{R: 0xd5, G: 0xdb, B: 0xe5, A: 0xff}),
		theme.Set(DialogShadow, color.NRGBA{A: 0x40}),
		theme.Set(Scrim, color.NRGBA{R: 0xf0, G: 0xf2, B: 0xf6, A: 0x80}),
		theme.Set(DialogRadius, 22),
		theme.Set(DialogPadding, 28),
		theme.Set(DialogWidth, 460),
		theme.Set(DialogHeight, 220),
		theme.Set(ListSpacing, 10),
		theme.Set(RowRadius, 10),
	)
}

// Dark is the default theme, with every token at its default.
func Dark() theme.Theme { return theme.Make("dark") }
