// Command background works on while it is out of sight. Its button starts a job of Android's, which a count to 300
// runs in, a step a second. The job goes on as the user leaves the program. If Android ends the program's process, it
// starts the process again for the job, with no window, and the count goes on from where it was. A window opened
// later shows it.
//
//	go run ./tools/gunimapk -run -name Background -min-sdk 34 -permissions internet,network-state,notifications,jobs,boot \
//		-manifest example/background/manifest.xml -java example/background/java ./example/background
//
// On a desktop the count runs as long as the window is open.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// startIntent is the button clicked.
type startIntent struct{}

// job says that the job runs, over network by Android's number for it, or that it stopped.
type job struct {
	on      bool
	network int64
}

// jobs brings the job's news, on Android from its UI thread.
var jobs = make(chan job, 16)

// steps is how far the count goes.
const steps = 300

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	news := make(chan string, 1)
	// The work starts apart from the window, which waits for an activity where Android started the program for the
	// job alone.
	go work(ctx, news)
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, werr := a.NewWindow(gunim.WindowOptions{Title: "gunim background", Size: geom.Sz(360, 240), Root: widget.NewSurface()})
		if werr != nil {
			return werr
		}
		gunim.RegisterView(w, "worker", func(string) *worker { return newWorker() }, func(v *worker, s string, _ *gunim.UI) {
			v.text.Text = s
		})
		c := w.Client()
		if merr := c.Mount(gunim.Root, "worker", "worker", "Not started."); merr != nil {
			return merr
		}
		for {
			select {
			case e, ok := <-c.Intents():
				if !ok {
					return c.Err()
				}
				if _, ok := e.Intent.(startIntent); ok {
					if serr := schedule(); serr != nil {
						_ = c.Update("worker", serr.Error())
					}
				}
			case s := <-news:
				_ = c.Update("worker", s)
			}
		}
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// work counts while the job runs, and tells news of it.
func work(ctx context.Context, news chan string) {
	tell := func(s string) {
		log.Print(s)
		// A window that has not come yet finds the latest alone.
		select {
		case <-news:
		default:
		}
		news <- s
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	on := false
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-jobs:
			on = j.on
			if !on {
				tell("Android stopped the job.")
				break
			}
			tell("The job runs.")
			go func() { log.Print(from(ctx, j.network)) }()
		case <-tick.C:
			if !on {
				break
			}
			n, err := step()
			switch {
			case err != nil:
				on = false
				tell(err.Error())
			case n < steps:
				tell(fmt.Sprintf("Step %d of %d.", n, steps))
				continue
			default:
				on = false
				tell("Done.")
			}
			if err := finish(); err != nil {
				log.Print(err)
			}
		}
	}
}

// step adds one to the count on disk, which a killed process leaves for the next, and returns it. The last step starts
// the count over.
func step() (int, error) {
	d, err := folder()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return 0, err
	}
	file := filepath.Join(d, "steps")
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	// A file that holds no number starts the count over.
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	n++
	return n, os.WriteFile(file, []byte(strconv.Itoa(n%steps)), 0o600)
}

// worker is a button over what the work says.
type worker struct {
	button *widget.Button
	text   *widget.Label
}

func newWorker() *worker {
	b := widget.NewButton("Start")
	b.OnClick = widget.Sends(startIntent{})
	return &worker{button: b, text: widget.NewLabel("")}
}

// Children implements [gunim.Composite].
func (v *worker) Children() []gunim.Node { return []gunim.Node{v.button, v.text} }

// Layout implements [gunim.Node]: the button on top and the text filling the rest. The surface under the view keeps it
// clear of the system's bars.
func (v *worker) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, buttonH = 16, 48
	w := box.W - 2*margin
	kids.At(0).Layout(gunim.Tight(geom.Sz(w, buttonH)))
	kids.At(0).Place(geom.Pt(margin, margin))
	y := float32(margin + buttonH + margin)
	kids.At(1).Layout(gunim.Tight(geom.Sz(w, max(box.H-y-margin, 0))))
	kids.At(1).Place(geom.Pt(margin, y))
	return box
}

// Paint implements [gunim.Node].
func (v *worker) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
