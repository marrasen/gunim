package main

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// scriptPause is how long each step of a script waits for the one before it to show.
const scriptPause = 500 * time.Millisecond

// runScript runs the steps of a script from -do one after another, each after a pause. A step is "day", "week" or
// "month" to pick the view, "next" or "back" to step it, "today", "new" to open the editor for a new event,
// "edit:title" to open it on the first event shown with that title, "draw:hh:mm-hh:mm" to draw out an event today,
// "hide:calendar" to toggle a calendar by its ID, "theme" to switch the theme, "shot:path" to write the window to
// a PNG file, "click:x,y", "dblclick:x,y" or "rclick:x,y" to click there, "show:words" to show the first event a search for
// the words finds, "invite" to have a colleague send an invitation, and "wait" to do nothing for a step.
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
	switch verb {
	case "day", "week", "month":
		a.handle(ViewChosen{View: map[string]View{"day": DayView, "week": WeekView, "month": MonthView}[verb]})
	case "next":
		a.handle(Stepped{By: 1})
	case "back":
		a.handle(Stepped{By: -1})
	case "today":
		a.handle(TodayAsked{})
	case "new":
		a.handle(NewAsked{})
	case "edit":
		for _, e := range a.state().Events {
			if e.Title == arg {
				a.handle(EditAsked{ID: e.ID})
				return
			}
		}
		log.Printf("script: no event %q shown", arg)
	case "draw":
		from, to, _ := strings.Cut(arg, "-")
		s, ok1 := calendar.ParseClock(from)
		e, ok2 := calendar.ParseClock(to)
		if !ok1 || !ok2 {
			log.Printf("script: %s: want times as hh:mm-hh:mm", step)
			return
		}
		day := calendar.Day(time.Now())
		a.handle(EventDrawn{Start: day.Add(s), End: day.Add(e)})
	case "hide":
		a.handle(CalendarToggled{ID: arg})
	case "theme":
		a.handle(ThemeToggled{})
	case "shot":
		if err := writeShot(a.ctx, a.c, arg); err != nil {
			log.Printf("script: %s: %v", step, err)
		}
	case "click", "dblclick", "rclick":
		xs, ys, _ := strings.Cut(arg, ",")
		x, err1 := strconv.ParseFloat(xs, 32)
		y, err2 := strconv.ParseFloat(ys, 32)
		if err1 != nil || err2 != nil {
			log.Printf("script: %s: want a place as x,y", step)
			return
		}
		at := geom.Pt(float32(x), float32(y))
		clicks := map[string]int{"click": 1, "dblclick": 2, "rclick": 1}[verb]
		button := input.ButtonPrimary
		if verb == "rclick" {
			button = input.ButtonSecondary
		}
		for c := 1; c <= clicks; c++ {
			if err := errors.Join(
				a.c.Input(a.ctx, input.PointerMove{Pos: at}),
				a.c.Input(a.ctx, input.PointerDown{Pos: at, Button: button, Clicks: c}),
				a.c.Input(a.ctx, input.PointerUp{Pos: at, Button: button})); err != nil {
				log.Printf("script: %s: %v", step, err)
			}
		}
	case "find":
		a.handle(SearchAsked{Query: arg})
	case "show":
		for _, f := range a.search(arg, time.Now()) {
			a.handle(EventShown{ID: f.ID})
			return
		}
		log.Printf("script: nothing found for %q", arg)
	case "invite":
		a.invite()
	case "wait":
	default:
		log.Printf("script: no step %q (%s)", step, strconv.Quote(step))
	}
}
