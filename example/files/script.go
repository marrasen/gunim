package main

import (
	"path/filepath"
	"strings"
	"time"
)

// scriptPause is how long each step of a script waits for the one
// before it to show.
const scriptPause = 700 * time.Millisecond

// runScript runs the next step of the script from -do, and the rest after
// it, each after a pause. A step is a command's name, "into:name" to go
// into a folder, "select:name" to select an item, or "wait" to do nothing.
func (a *app) runScript() {
	time.AfterFunc(scriptPause, func() {
		a.post(func() {
			if len(a.script) == 0 {
				return
			}
			step := a.script[0]
			a.script = a.script[1:]
			a.scriptStep(step)
			a.runScript()
		})
	})
}

func (a *app) scriptStep(step string) {
	verb, arg, _ := strings.Cut(step, ":")
	switch verb {
	case "into":
		a.navigate(filepath.Join(a.nav.path, arg), 1, true)
	case "select":
		clear(a.nav.sel)
		a.nav.sel[arg] = true
		a.nav.cursor = arg
		a.publishSelection()
		a.publishStatus()
		a.showPreview()
	case "wait":
	default:
		a.handle(a.handlers, Command{Name: step})
	}
}
