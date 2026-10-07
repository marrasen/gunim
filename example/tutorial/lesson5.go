package main

import (
	_ "embed"
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Lesson 5: themes.
//
// A theme sets how widgets look and move: colours, paddings, radii,
// fonts, text sizes and springs. A widget declares each value as a
// token with a default and reads it every frame. Switching themes
// retargets every token at once, so colours, sizes and motion move
// together.

//go:embed lesson5.go
var lesson5Source string

// The vocabulary the two halves share.
type (
	// Look is what the view shows: which theme is on.
	Look struct{ Light bool }
	// ThemeChosen travels when the switch flips.
	ThemeChosen struct{ Light bool }
)

func init() {
	gunim.RegisterType[Look]("tutorial.look")
	gunim.RegisterType[ThemeChosen]("tutorial.theme")
}

var lesson5 = lesson{
	Title:  lessonTitles[4],
	File:   lessonFile(4),
	Source: lesson5Source,
	View:   "lesson5",
	Register: func(w *gunim.Window) {
		gunim.RegisterView(w, "lesson5", buildLesson5, (*lookPage).show)
	},
	State: func(a *app) any { return Look{Light: a.light} },
	Handle: func(a *app, c gunim.Client, in gunim.Intent) bool {
		chosen, ok := in.(ThemeChosen)
		if !ok {
			return false
		}
		a.light = chosen.Light
		// The themes were registered with the window in main.go, by
		// name, and SetTheme switches to one.
		name := "dark"
		if a.light {
			name = "light"
		}
		_ = c.SetTheme(name)
		return true
	},
}

const lesson5Intro = `A theme sets how widgets look and move: colours, but also paddings, radii, fonts, text sizes and the springs animations run on. A widget declares each value as a **token** with a default, and reads it every frame with ` + "`Get(f.Theme)`" + `. A theme gives tokens values, and the window keeps one animated value per token.

Switching themes retargets them all, so going from dark to light moves colours, sizes and motion together, and a button half way into its hover colour carries on from where it is. Colours blend through Oklab, so they keep their brightness on the way.

The card on the right wears a theme of its own, made with ` + "`widget.NewThemed`" + `: it sets its fill and ink, and takes everything else from the theme around it. The big text reads a token declared in this file, which the light theme sets larger.

Flip the switch and watch every colour and size move at once.`

// bigText is a token of this lesson's own: the size of its big label.
// Light() sets it larger below.
var bigText = theme.Length("tutorial.big", 28)

// lookPage is the lesson's view, with a handle on the switch.
type lookPage struct {
	*page
	light *widget.Switch
}

func buildLesson5(Look) *lookPage {
	light := widget.NewSwitch("Light theme")
	light.OnChange = func(on bool, u *gunim.UI) gunim.Intent { return ThemeChosen{Light: on} }
	big := widget.NewLabel("Big text")
	big.Size = bigText
	primary := widget.NewButton("Primary")
	primary.Kind = widget.ButtonPrimary
	plain := widget.NewButton("Plain")
	buttons := widget.Row(primary, plain, big)
	buttons.Cross = widget.CrossCenter
	callout := widget.NewThemed(
		widget.NewCard(widget.NewLabel("This card sets its own fill and ink. Its padding and corners come from the theme around it, and move with it.")),
		theme.Make("callout",
			theme.Set(widget.CardFill, color.NRGBA{R: 0x2f, G: 0x6f, B: 0xe0, A: 0xff}),
			theme.Set(widget.Ink, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})))
	left := widget.Column(light, buttons)
	row := widget.Row(left, callout).Grow(left, 1).Grow(callout, 1)
	return &lookPage{
		page:  newPage(4, lesson5Intro, row, lesson5Source),
		light: light,
	}
}

// show puts the switch where the state says. SetChecked is for exactly
// this: it moves the switch and leaves the intent to the user's flips.
func (p *lookPage) show(s Look, u *gunim.UI) { p.light.SetChecked(s.Light, u) }
