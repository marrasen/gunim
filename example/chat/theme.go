package main

import (
	"image/color"

	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The chat's own tokens. Their defaults belong to the dark theme.
var (
	RailFill    = theme.Color("chat.rail", color.NRGBA{R: 0x14, G: 0x16, B: 0x1c, A: 0xff})
	SidebarFill = theme.Color("chat.sidebar", color.NRGBA{R: 0x1a, G: 0x1d, B: 0x24, A: 0xff})
	SidebarHot  = theme.Color("chat.sidebar.hot", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x10})
	SidebarOn   = theme.Color("chat.sidebar.on", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x30})
	PaneFill    = theme.Color("chat.pane", color.NRGBA{R: 0x1d, G: 0x20, B: 0x28, A: 0xff})
	RowHot      = theme.Color("chat.row.hot", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x08})
	QuoteFill   = theme.Color("chat.quote", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0a})
	Faint       = theme.Foreground("chat.faint", color.NRGBA{R: 0x8a, G: 0x93, B: 0xa6, A: 0xff})
	ErrorInk    = theme.Foreground("chat.error", color.NRGBA{R: 0xff, G: 0x8a, B: 0x80, A: 0xff})
	WarnFill    = theme.Color("chat.warn", color.NRGBA{R: 0x5a, G: 0x45, B: 0x1c, A: 0xff})
	BadgeFill   = theme.Color("chat.badge", color.NRGBA{R: 0xe0, G: 0x4f, B: 0x4f, A: 0xff})
	SmallText   = theme.Length("chat.small", 12)
)

// avatarTints colour the avatars, one per author, picked by the author's name.
var avatarTints = []color.NRGBA{
	{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff},
	{R: 0xe8, G: 0x8b, B: 0x4a, A: 0xff},
	{R: 0x4f, G: 0xb6, B: 0x8c, A: 0xff},
	{R: 0xc4, G: 0x7b, B: 0xe8, A: 0xff},
	{R: 0xe0, G: 0x5f, B: 0x7d, A: 0xff},
	{R: 0x3f, G: 0xb0, B: 0xc8, A: 0xff},
}

func darkTheme() theme.Theme { return widget.Dark() }

func lightTheme() theme.Theme {
	return widget.Light().With(
		theme.Set(RailFill, color.NRGBA{R: 0xdf, G: 0xe3, B: 0xea, A: 0xff}),
		theme.Set(SidebarFill, color.NRGBA{R: 0xe9, G: 0xec, B: 0xf2, A: 0xff}),
		theme.Set(SidebarHot, color.NRGBA{A: 0x0c}),
		theme.Set(SidebarOn, color.NRGBA{R: 0x2f, G: 0x6f, B: 0xe0, A: 0x26}),
		theme.Set(PaneFill, color.NRGBA{R: 0xfa, G: 0xfb, B: 0xfd, A: 0xff}),
		theme.Set(RowHot, color.NRGBA{A: 0x08}),
		theme.Set(QuoteFill, color.NRGBA{A: 0x0a}),
		theme.Set(Faint, color.NRGBA{R: 0x5f, G: 0x67, B: 0x75, A: 0xff}),
		theme.Set(ErrorInk, color.NRGBA{R: 0xb4, G: 0x2a, B: 0x22, A: 0xff}),
		theme.Set(WarnFill, color.NRGBA{R: 0xfb, G: 0xe8, B: 0xc0, A: 0xff}),
		theme.Set(widget.TextSize, 14),
		theme.Set(widget.ButtonHeight, 34),
		theme.Set(widget.ButtonPadding, 16),
		theme.Set(widget.ButtonRadius, 8),
		theme.Set(widget.FieldRadius, 8),
	)
}
