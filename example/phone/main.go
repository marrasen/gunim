// Command phone is a program laid out for a phone held upright: a bar of three places along the bottom, which
// stands as a rail down the left in a window 600 or more wide. The inbox refreshes as it is pulled down, and every
// third refresh fails. The second place opens a bottom sheet with a field and a long list, which drags between half
// and full and closes by a fling down, a tap above it or Back. The text is as large as the user has it in the
// system's settings.
//
//	go run ./example/phone -size 390x800
//	go run ./tools/gunimapk -run -name Phone ./example/phone
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

func main() {
	var w, h int
	size := flag.String("size", "390x800", "the window's size, as WIDTHxHEIGHT")
	flag.Parse()
	if _, err := fmt.Sscanf(*size, "%dx%d", &w, &h); err != nil {
		log.Fatalf("-size %q: %v", *size, err)
	}
	if err := run(geom.Sz(float32(w), float32(h))); err != nil {
		log.Fatal(err)
	}
}

// look is the theme for the user's settings.
func look(l gunim.SystemLook, err error) theme.Theme {
	if err != nil {
		return widget.Dark().With(widget.Touch()...)
	}
	base := widget.Light()
	if l.Dark {
		base = widget.Dark()
	}
	return widget.ForLook(base.With(widget.Touch()...), l)
}

// asked is a refresh the user asked for, and answer the program's answer to it.
type asked struct{ N int }

type answer struct {
	N      int
	Failed bool
	At     time.Time
}

func run(size geom.Size) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim phone", Size: size, Root: widget.NewSurface(), Theme: look(a.SystemLook()), LeaveOnBack: true,
		})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "phone", func(answer) *phone { return newPhone() },
			func(v *phone, s answer, u *gunim.UI) { v.answered(s, u) })
		c := w.Client()
		if err := c.Mount(gunim.Root, "phone", "phone", answer{}); err != nil {
			return err
		}
		if stop, err := a.WatchSystemLook(func(l gunim.SystemLook, err error) {
			th := look(l, err)
			w.RegisterTheme(th)
			_ = c.SetTheme(th.Name)
		}); err == nil {
			defer stop()
		}
		for in := range c.Intents() {
			q, ok := in.Intent.(asked)
			if !ok {
				continue
			}
			// The work a refresh stands for: a moment's wait, and every third one fails.
			go func() {
				time.Sleep(1500 * time.Millisecond)
				_ = c.Update("phone", answer{N: q.N, Failed: q.N%3 == 0, At: time.Now()})
			}()
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

// phone is the program's one view: the places and their pages.
type phone struct {
	*widget.BottomBar
	inbox *widget.PullRefresh
	when  *widget.Label
}

func newPhone() *phone {
	v := &phone{when: widget.NewLabel("Pull the list down to refresh it")}
	rows := make([]gunim.Node, 0, 41)
	rows = append(rows, v.when)
	for i := range 40 {
		rows = append(rows, widget.NewLabel(fmt.Sprintf("Message %d", i+1)))
	}
	v.inbox = widget.NewPullRefresh(widget.NewScroll(widget.NewPad(widget.Column(rows...))))
	v.inbox.Label = "Inbox"
	v.inbox.OnRefresh = func(n int, _ *gunim.UI) gunim.Intent { return asked{n} }

	share := widget.NewButton("Share a note")
	share.OnClick = func(u *gunim.UI) gunim.Intent {
		u.Insert(u.Root(), noteSheet())
		return nil
	}
	places := []widget.BarItem{
		{Key: "inbox", Label: "Inbox", Icon: icon.Inbox, Count: 3, CountLabel: "3 unread"},
		{Key: "notes", Label: "Notes", Icon: icon.NotebookPen},
		{Key: "you", Label: "You", Icon: icon.User, Dot: true},
	}
	you := widget.NewPad(widget.NewLabel("Make the window 600 wide or more, and the bar stands as a rail."))
	v.BottomBar = widget.NewBottomBar(places, v.inbox, widget.NewPad(widget.Column(share)), you)
	v.Label = "Places"
	v.OnSelect = func(i int, u *gunim.UI) gunim.Intent {
		if i == 0 {
			// The inbox has been seen.
			items := v.Items()
			items[0].Count = 0
			v.SetItems(items, u)
		}
		return nil
	}
	return v
}

// answered ends a refresh as the program answers it.
func (v *phone) answered(s answer, u *gunim.UI) {
	if s.N == 0 {
		return
	}
	if s.Failed {
		v.inbox.Fail(s.N, "The inbox could not be refreshed", u)
		return
	}
	v.when.Text = "Refreshed at " + s.At.Format("15:04:05")
	v.inbox.Done(s.N, u)
	u.Invalidate()
}

// noteSheet is a sheet with a field to write a note in and a long list of people to share it with.
func noteSheet() *widget.BottomSheet {
	note := widget.NewTextField()
	note.Placeholder = "Write a note"
	rows := make([]gunim.Node, 0, 31)
	rows = append(rows, note)
	for i := range 30 {
		rows = append(rows, widget.NewButton(fmt.Sprintf("Share with friend %d", i+1)))
	}
	s := widget.NewBottomSheet("Share a note", widget.NewPad(widget.Column(rows...)))
	s.OnDismiss = func(*gunim.UI) gunim.Intent {
		log.Print("The sheet closes")
		return nil
	}
	return s
}
