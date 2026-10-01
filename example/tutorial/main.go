// Command tutorial teaches how to build a gunim application, one lesson
// at a time, in a window. Each lesson shows a piece of interface to
// try, says what makes it work, and shows its own source file below,
// so what is on screen and what is in the file are the same code.
//
//	CGO_ENABLED=0 go run ./example/tutorial
//	CGO_ENABLED=0 go run ./example/tutorial -lesson 3
//
// The lessons, in order:
//
//  1. A label in a card: the node tree, and layout with rows and columns.
//  2. A button: the two halves, an intent, and state published back.
//  3. A list: keyed rows that grow in, collapse out, and slide to their places.
//  4. A node of your own: Layout, Paint, Handle and Step.
//  5. Themes: tokens, and a switch that animates every one of them.
//  6. A dialog: mounting a view, and an exit that finishes itself.
//
// Each lesson's file is in an editor on its page. Change it and press
// Run: the tutorial builds again with the file as edited, and opens on
// that lesson in a window of its own. A build error marks its line.
// Run needs the Go toolchain and starts from the gunim repository.
//
// Read the lessons in the window, or read the files: main.go is the
// application half and the frame around the lessons, shell.go the
// lesson list, page.go the page each lesson sits on, runner.go the
// build and run, and lesson1.go to lesson6.go one lesson each.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	start := flag.Int("lesson", 1, "the lesson to open on, 1 to 6")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 1500*time.Millisecond, "how long -shot waits")
	flag.Parse()
	if err := run(*runFor, *start-1, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration, start int, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		// A window: a title, a size, and a root node. A Surface fills
		// the window with the theme's background and stacks the views
		// the application mounts over it.
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim tutorial",
			Size:  geom.Sz(1180, 800),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		registerViews(w)
		c := w.Client()
		if shot != "" {
			go func() {
				select {
				case <-time.After(after):
				case <-ctx.Done():
					return
				}
				if err := writeShot(ctx, c, shot); err != nil {
					log.Print(err)
				}
				c.Close()
			}()
		}
		return serve(ctx, c, start)
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// registerViews is the window half: the themes the application can
// switch to, the shell, and each lesson's view.
func registerViews(w *gunim.Window) {
	w.RegisterTheme(widget.Dark())
	// The light theme also sets a token of this program's own, declared
	// in lesson5.go, so the switch moves it with the rest.
	w.RegisterTheme(widget.Light().With(theme.Set(bigText, 36)))
	registerShell(w)
	for _, l := range lessons {
		l.Register(w)
		registerPagePatches(w, l.View)
	}
}

// A lesson is one page of the tutorial: its view, the file it shows,
// and the part of the application half it needs.
type lesson struct {
	// Title names the lesson in the list.
	Title string
	// File is the lesson's file, and Source what it holds.
	File, Source string
	// View is the name Register gives the lesson's view.
	View string
	// Register registers the lesson's view with a window.
	Register func(w *gunim.Window)
	// State is the state the lesson's view shows, from the
	// application's.
	State func(a *app) any
	// Handle carries out one intent for the lesson, and reports
	// whether the intent was one of its own. It may be nil.
	Handle func(a *app, c gunim.Client, in gunim.Intent) bool
}

// lessons are the tutorial's lessons, in order.
var lessons = []lesson{lesson1, lesson2, lesson3, lesson4, lesson5, lesson6}

// app is the application half's state. It is reached from serve alone,
// so it needs no lock.
type app struct {
	// lesson is the lesson open now, counted from 0.
	lesson int
	// opened says a lesson's page is mounted, so the next one has one
	// to unmount first.
	opened bool

	// clicks is lesson 2's count.
	clicks int
	// items is lesson 3's list, and nextID the ID the next item gets.
	items  []Item
	nextID int
	// light is lesson 5's theme choice.
	light bool
	// trash is lesson 6's number of files in the trash.
	trash int

	// ctx is serve's, which a run's sends wait on.
	ctx context.Context
	// sources are the lessons' files as edited, by lesson; a lesson
	// left out is as it was.
	sources map[int]string
	// runs are how each lesson's last run went, by lesson.
	runs map[int]RunState
	// running is the lesson running now, or -1; runID counts runs, and
	// stop stops the one going.
	running int
	runID   int
	stop    context.CancelFunc
	events  chan runEvent
}

// serve is the application half. It mounts the shell and the first
// lesson, then hears what the window sends and what a run reports,
// carries each out, and hands the lesson its state again.
func serve(ctx context.Context, c gunim.Client, start int) error {
	a := newApp(start)
	a.ctx = ctx
	defer a.stopRun()
	if err := c.Mount(gunim.Root, "shell", "shell", Shell{Lesson: a.lesson}); err != nil {
		return err
	}
	if err := a.open(c); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-a.events:
			a.ran(c, ev)
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			a.handle(c, ev.Intent)
		}
	}
}

// handle carries out one intent from the window.
func (a *app) handle(c gunim.Client, in gunim.Intent) {
	switch in := in.(type) {
	case Chose:
		if in.Lesson == a.lesson {
			return
		}
		a.lesson = in.Lesson
		_ = c.Update("shell", Shell{Lesson: a.lesson})
		_ = a.open(c)
		return
	case Edited:
		a.sources[in.Lesson] = in.Source
		return
	case RunAsked:
		a.startRun(in.Lesson)
		return
	case StopAsked:
		a.stopRun()
		return
	case FormatAsked:
		src, marks, err := formatSource(a.source(in.Lesson))
		if err != nil {
			a.show(c, in.Lesson, RunState{Status: "Format failed", Failed: true, Output: err.Error() + "\n", Marks: marks})
			return
		}
		a.sources[in.Lesson] = src
		_ = c.Patch("page", Code{Source: src})
		a.show(c, in.Lesson, RunState{})
		return
	case ResetAsked:
		delete(a.sources, in.Lesson)
		_ = c.Patch("page", Code{Source: lessons[in.Lesson].Source})
		a.show(c, in.Lesson, RunState{})
		return
	case gunim.CommandFailed:
		log.Printf("command %s failed: %s", in.Command, in.Reason)
		return
	}
	for _, l := range lessons {
		if l.Handle != nil && l.Handle(a, c, in) {
			break
		}
	}
	_ = c.Update("page", lessons[a.lesson].State(a))
}

// source returns lesson's file as edited, or as it was.
func (a *app) source(lesson int) string {
	if s, ok := a.sources[lesson]; ok {
		return s
	}
	return lessons[lesson].Source
}

// show keeps how lesson's run went, and shows it when its page is open.
func (a *app) show(c gunim.Client, lesson int, r RunState) {
	a.runs[lesson] = r
	if lesson == a.lesson {
		_ = c.Patch("page", r)
	}
}

// startRun stops any run going and starts lesson's.
func (a *app) startRun(lesson int) {
	a.stopRun()
	a.runID++
	a.running = lesson
	a.runs[lesson] = RunState{}
	ctx, stop := context.WithCancel(a.ctx)
	a.stop = stop
	go startRun(ctx, a.ctx, a.runID, lesson, a.source(lesson), a.events)
}

// stopRun stops the run going, if one is.
func (a *app) stopRun() {
	if a.stop != nil {
		a.stop()
		a.stop = nil
	}
}

// maxOutput is how many lines of output a page keeps.
const maxOutput = 300

// ran takes what the run going reports, and drops what an older one
// still says.
func (a *app) ran(c gunim.Client, ev runEvent) {
	if ev.ID != a.runID {
		return
	}
	r := a.runs[a.running]
	r.Status, r.Running, r.Failed = ev.Status, ev.Running, ev.Failed
	r.Output += ev.Output
	if lines := strings.Split(r.Output, "\n"); len(lines) > maxOutput {
		r.Output = strings.Join(lines[len(lines)-maxOutput:], "\n")
	}
	if ev.Marked {
		r.Marks = ev.Marks
	}
	if !ev.Running {
		a.stop = nil
	}
	a.show(c, a.running, r)
}

// newApp returns the application's state, open on lesson start.
func newApp(start int) *app {
	a := &app{
		lesson:  min(max(start, 0), len(lessons)-1),
		trash:   12,
		sources: map[int]string{},
		runs:    map[int]RunState{},
		running: -1,
		events:  make(chan runEvent, 64),
		ctx:     context.Background(),
	}
	for _, t := range []string{"Read lesson 3", "Add an item", "Remove one"} {
		a.add(t)
	}
	return a
}

// open mounts the lesson open now in the shell's slot. The page
// before it is unmounted first: it animates out while the new one
// animates in, and the two share the ID "page" for that moment.
func (a *app) open(c gunim.Client) error {
	if a.opened {
		if err := c.Unmount("page"); err != nil {
			return err
		}
	}
	a.opened = true
	l := lessons[a.lesson]
	if err := c.Mount("shell", "page", l.View, l.State(a)); err != nil {
		return err
	}
	// The page is built from the file as it was; edits made before
	// and the last run's result go back in.
	if src, ok := a.sources[a.lesson]; ok {
		if err := c.Patch("page", Code{Source: src}); err != nil {
			return err
		}
	}
	if r, ok := a.runs[a.lesson]; ok {
		return c.Patch("page", r)
	}
	return nil
}

// writeShot writes what the window shows to a PNG file.
func writeShot(ctx context.Context, c gunim.Client, path string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}

// lessonTitle is a lesson's title with its number.
func lessonTitle(i int) string { return fmt.Sprintf("%d. %s", i+1, lessons[i].Title) }
