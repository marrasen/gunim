package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The example's own tokens, next to the widget library's. Their
// defaults belong to the dark theme.
var (
	Background   = theme.Color("jobs.background", color.NRGBA{R: 0x16, G: 0x18, B: 0x1e, A: 0xff})
	RowIdle      = theme.Color("jobs.row.idle", color.NRGBA{R: 0x22, G: 0x26, B: 0x30, A: 0xff})
	RowRunning   = theme.Color("jobs.row.running", color.NRGBA{R: 0x1e, G: 0x33, B: 0x4a, A: 0xff})
	RowDone      = theme.Color("jobs.row.done", color.NRGBA{R: 0x1e, G: 0x3a, B: 0x2c, A: 0xff})
	RowHeight    = theme.Length("jobs.row.height", 44)
	RowRadius    = theme.Length("jobs.row.radius", 8)
	RowTitleSize = theme.Length("jobs.row.title.size", 15)
)

// darkTheme and lightTheme extend the widget library's themes with the
// example's tokens. The light one also makes rows taller and rounder,
// so a switch shows sizes and shapes moving along with the colours.
func darkTheme() theme.Theme { return widget.Dark() }

func lightTheme() theme.Theme {
	return widget.Light().With(
		theme.Set(Background, color.NRGBA{R: 0xf3, G: 0xf5, B: 0xf9, A: 0xff}),
		theme.Set(RowIdle, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}),
		theme.Set(RowRunning, color.NRGBA{R: 0xe3, G: 0xee, B: 0xff, A: 0xff}),
		theme.Set(RowDone, color.NRGBA{R: 0xe2, G: 0xf5, B: 0xe8, A: 0xff}),
		theme.Set(RowHeight, 54),
		theme.Set(RowRadius, 14),
		theme.Set(RowTitleSize, 16),
	)
}

// ThemeToggled travels when the user presses T.
type ThemeToggled struct{}

func init() { gunim.RegisterType[ThemeToggled]("theme.toggle") }

// shell is the window's root: it paints the themed background, stacks
// the mounted views over it, and turns T into a ThemeToggled intent.
// Keys reach it from whatever has focus, or directly when nothing does.
type shell struct {
	gunim.Box
}

// Handle implements [gunim.Handler].
func (s *shell) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok && k.Key == input.KeyT && !k.Repeat {
		u.Send(s, ThemeToggled{})
		return true
	}
	return false
}

// Paint implements [gunim.Node].
func (s *shell) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(Background.Get(f.Theme)))
	s.Box.Paint(p, f, box, kids)
}
