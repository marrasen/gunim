// Command notify shows messages of its own on a phone: a message, one with progress, and what a tap on one brings back.
// It says how the network is, too.
//
//	go run ./tools/gunimapk -run -name Notify -permissions notifications,network-state,internet ./example/notify
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// doIntent is a button clicked, by its label.
type doIntent struct{ what string }

// The buttons.
const (
	ask      = "Let it notify"
	message  = "Show a message"
	progress = "Show progress"
	end      = "Take both away"
)

// The messages' tags.
const (
	messageTag  = "message:1"
	progressTag = "work"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim notify", Size: geom.Sz(420, 640), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "out", func(string) *out { return newOut() }, func(v *out, s string, _ *gunim.UI) { v.text.Text = s })
		c := w.Client()
		if err := c.Mount(gunim.Root, "out", "out", ""); err != nil {
			return err
		}
		var mu sync.Mutex
		lines := ""
		tell := func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			s := fmt.Sprintf(format, args...)
			log.Print(s)
			lines = s + "\n" + lines
			_ = c.Update("out", lines)
		}
		a.OnNotification(func(tag string) { tell("Tapped: %q", tag) })
		// What an earlier run left half way goes on no more.
		_ = a.EndNotify(progressTag)
		stop, err := a.WatchNetwork(func(s gunim.NetworkState) {
			tell("Network: online %v, internet %v, metered %v, blocked %v, changed %v", s.Online, s.Internet, s.Metered, s.Blocked,
				s.Changed)
		})
		if err != nil {
			tell("No news of the network: %v", err)
		} else {
			defer stop()
		}
		icon := ring()
		shown := 0
		for e := range c.Intents() {
			in, ok := e.Intent.(doIntent)
			if !ok {
				continue
			}
			switch in.what {
			case ask:
				go func() { tell("Let: %v", a.Ask(driver.PermissionNotifications)) }()
			case message:
				shown++
				tell("Shown: %v", a.Notify(gunim.Notification{
					Tag:     messageTag,
					Channel: gunim.NotifyChannel{ID: "messages", Name: "Messages", Description: "What others wrote"},
					Title:   "Ada", Body: fmt.Sprintf("Message %d, for your eyes alone", shown),
					PublicTitle: "A message", Icon: icon,
				}))
			case progress:
				go func() { tell("Work: %v", work(ctx, a, icon)) }()
			case end:
				tell("Taken away: %v", errors.Join(a.EndNotify(messageTag), a.EndNotify(progressTag)))
			}
		}
		return c.Err()
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

// work shows how ten seconds of work come along, and that it is done.
func work(ctx context.Context, a *gunim.App, icon image.Image) error {
	n := gunim.Notification{
		Tag:     progressTag,
		Channel: gunim.NotifyChannel{ID: "work", Name: "Work", Description: "How far work has come", Importance: driver.NotifyQuiet},
		Title:   "Working", Ongoing: true, Lock: driver.LockSecret, Icon: icon,
	}
	const total = 1000
	for i := range total + 1 {
		n.Progress = &gunim.NotifyProgress{Done: int64(i), Total: total}
		n.Body = fmt.Sprintf("%d of %d", i, total)
		if err := a.Notify(n); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return a.EndNotify(progressTag)
		case <-time.After(10 * time.Millisecond):
		}
	}
	n.Title, n.Body, n.Progress, n.Ongoing = "Done", "All of the work", nil, false
	return a.Notify(n)
}

// ring is the messages' icon: a ring.
func ring() image.Image {
	const side = 96
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			dx, dy := float64(x)-side/2+0.5, float64(y)-side/2+0.5
			if d := dx*dx + dy*dy; d < 40*40 && d > 24*24 {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	return img
}

// out is buttons over what came of them.
type out struct {
	buttons []gunim.Node
	text    *widget.Label
}

func newOut() *out {
	v := &out{text: widget.NewLabel("")}
	for _, what := range []string{ask, message, progress, end} {
		b := widget.NewButton(what)
		b.OnClick = widget.Sends(doIntent{what})
		v.buttons = append(v.buttons, b)
	}
	return v
}

// Children implements [gunim.Composite].
func (v *out) Children() []gunim.Node { return append(append([]gunim.Node{}, v.buttons...), v.text) }

// Layout implements [gunim.Node]: the buttons on top and the text filling the rest. The surface under the view keeps it
// clear of the system's bars.
func (v *out) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, buttonH = 12, 48
	w := box.W - 2*margin
	y := float32(margin)
	for i := range v.buttons {
		kids.At(i).Layout(gunim.Tight(geom.Sz(w, buttonH)))
		kids.At(i).Place(geom.Pt(margin, y))
		y += buttonH + margin
	}
	last := kids.At(len(v.buttons))
	last.Layout(gunim.Tight(geom.Sz(w, max(box.H-y-margin, 0))))
	last.Place(geom.Pt(margin, y))
	return box
}

// Paint implements [gunim.Node].
func (v *out) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
