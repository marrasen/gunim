package main

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// scriptPause is how long each step of a script waits for the one before it to show.
const scriptPause = 500 * time.Millisecond

// runScript runs the steps of a script from -do one after another, each after a pause. A step is "reply" to reply
// to the second latest message, "type:text" to type in the message box, "send" to press Enter, "offline" and
// "online" to drop and restore the connection, "typing:name" to have name type, "hover:x,y" to move the pointer
// there, and "theme:light" or "theme:dark" to switch the theme.
func (a *app) runScript(steps []string) {
	if len(steps) == 0 {
		return
	}
	a.after(scriptPause, func() {
		a.scriptStep(steps[0])
		a.runScript(steps[1:])
	})
}

func (a *app) scriptStep(step string) {
	verb, arg, _ := strings.Cut(step, ":")
	var err error
	switch verb {
	case "reply":
		if n := len(a.current.msgs); n >= 2 {
			a.handle(ReplyAsked{ID: a.current.msgs[n-2].ID})
		}
	case "type":
		err = a.c.Input(a.ctx, input.TextInput{Text: strings.ReplaceAll(arg, `\n`, "\n")})
	case "send":
		err = a.c.Input(a.ctx, input.KeyPress{Key: input.KeyEnter})
	case "offline", "online":
		if (a.link == Online) == (verb == "offline") {
			a.handle(LinkToggled{})
		}
	case "typing":
		a.setTyping(arg)
	case "hover":
		xs, ys, _ := strings.Cut(arg, ",")
		x, _ := strconv.ParseFloat(xs, 32)
		y, _ := strconv.ParseFloat(ys, 32)
		err = a.c.Input(a.ctx, input.PointerMove{Pos: geom.Pt(float32(x), float32(y))})
	case "theme":
		a.light = arg == "light"
		err = a.c.SetTheme(arg)
	default:
		log.Printf("script: no step %q", step)
	}
	if err != nil {
		log.Printf("script: %s: %v", step, err)
	}
}
