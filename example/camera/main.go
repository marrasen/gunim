// Command camera shows what the camera sees, and says how bright it is, how many frames a second come, and which side they
// come from. The camera stops as the program leaves the screen.
//
//	go run ./tools/gunimapk -run -name Camera -permissions camera ./example/camera
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// doIntent is a button clicked, by its label.
type doIntent struct{ what string }

// scan is the button that shows the camera.
const scan = "Show the camera"

// page is what the window shows: the words under the picture, and whether the camera is let.
type page struct {
	Words string
	Let   bool
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim camera", Size: geom.Sz(420, 640), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		m := &meter{}
		gunim.RegisterView(w, "page", func(page) *view { return newView(m) }, (*view).set)
		c := w.Client()
		if err := c.Mount(gunim.Root, "page", "page", page{Words: "The camera is off."}); err != nil {
			return err
		}
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		answers := make(chan bool, 1)
		let := false
		tell := func(format string, args ...any) {
			words := fmt.Sprintf(format, args...)
			log.Print(words)
			_ = c.Update("page", page{Words: words, Let: let})
		}
		for {
			select {
			case <-tick.C:
				if words := m.words(); words != "" {
					tell("%s", words)
				}
			case let = <-answers:
				if let {
					tell("The camera starts.")
				} else {
					tell("The camera is not let.")
				}
			case e, ok := <-c.Intents():
				if !ok {
					return c.Err()
				}
				switch in := e.Intent.(type) {
				case doIntent:
					// The prompt shows over the window, which goes on.
					go func() { answers <- a.Ask(driver.PermissionCamera) }()
				case stopped:
					// It starts again once the window has been out of sight.
					tell("The camera does not run: %v", in.err)
				}
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

// stopped says that the camera does not run.
type stopped struct{ err error }

// meter counts the frames, and keeps how bright the last was.
type meter struct {
	frames atomic.Int64
	last   atomic.Pointer[string]
}

// read reads a frame, on a goroutine of the camera's.
func (m *meter) read(f gunim.CameraFrame) {
	sum := 0
	for _, b := range f.Luma {
		sum += int(b)
	}
	m.frames.Add(1)
	s := fmt.Sprintf("%d by %d, turned %d, mirrored %v, brightness %d", f.Width, f.Height, f.Turn, f.Mirrored, sum/max(len(f.Luma), 1))
	m.last.Store(&s)
}

// words says what came since the last call, or "" for nothing.
func (m *meter) words() string {
	n, last := m.frames.Swap(0), m.last.Load()
	if n == 0 || last == nil {
		return ""
	}
	return fmt.Sprintf("%d frames a second, %s", n, *last)
}

// view is the picture over two buttons and the words.
type view struct {
	finder     *widget.Viewfinder
	show, turn *widget.Button
	text       *widget.Label
}

func newView(m *meter) *view {
	v := &view{finder: widget.NewViewfinder(), show: widget.NewButton(scan), turn: widget.NewButton("Turn it round"), text: widget.NewLabel("")}
	v.finder.Label = "What the camera sees"
	v.finder.Radius = 12
	v.finder.ReadFrame = m.read
	v.finder.OnStopped = func(err error, _ *gunim.UI) gunim.Intent { return stopped{err} }
	v.finder.SetOn(false, nil)
	v.show.OnClick = widget.Sends(doIntent{scan})
	v.turn.OnClick = func(u *gunim.UI) gunim.Intent {
		v.finder.Front = !v.finder.Front
		u.Invalidate()
		return nil
	}
	return v
}

// set shows what the application says.
func (v *view) set(p page, u *gunim.UI) {
	v.text.Text = p.Words
	if v.finder.On() != p.Let {
		v.finder.SetOn(p.Let, u)
	}
}

// Children implements [gunim.Composite].
func (v *view) Children() []gunim.Node { return []gunim.Node{v.finder, v.show, v.turn, v.text} }

// Layout implements [gunim.Node]: the picture on top, then the buttons and the words.
func (v *view) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, buttonH, textH = 12, 48, 96
	w := box.W - 2*margin
	h := max(box.H-3*margin-textH-2*(buttonH+margin), 0)
	y := float32(margin)
	for i, kh := range []float32{h, buttonH, buttonH, textH} {
		kids.At(i).Layout(gunim.Tight(geom.Sz(w, kh)))
		kids.At(i).Place(geom.Pt(margin, y))
		y += kh + margin
	}
	return box
}

// Paint implements [gunim.Node].
func (v *view) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
