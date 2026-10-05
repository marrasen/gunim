//go:build linux && !android

package install

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// offscreen runs the installer's window on s offscreen, its work's
// reports handled between frames, as serve would.
type offscreen struct {
	t *testing.T
	w *gunim.Window
	r *runner
}

func openOffscreen(t *testing.T, s *Session) *offscreen {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(640, 600), &gunim.Box{})
	r, err := attach(w, s, nil, accentFor(&s.App))
	if err != nil {
		t.Fatal(err)
	}
	o := &offscreen{t: t, w: w, r: r}
	o.frames(10)
	return o
}

// frames draws n frames, taking the work's reports between them.
func (o *offscreen) frames(n int) {
	for range n {
		for more := true; more; {
			select {
			case fn := <-o.r.events:
				fn()
			default:
				more = false
			}
		}
		o.w.Frame(time.Second / 60)
	}
}

// until draws frames until the scene is on page, or fails.
func (o *offscreen) until(page string) {
	o.t.Helper()
	for range 600 {
		if o.r.sc.Page == page {
			o.frames(60)
			return
		}
		o.frames(1)
		time.Sleep(time.Millisecond)
	}
	o.t.Fatalf("the installer is on %q, not %q (problem %q)", o.r.sc.Page, page, o.r.sc.Problem)
}

// The window takes an install from the welcome to done, with the ring
// and the pages crossing over, and an uninstall from asking to
// removed; what it shows is the scene the work made.
func TestWindowInstallsAndRemoves(t *testing.T) {
	leastWork = 0
	t.Cleanup(func() { leastWork = 1600 * time.Millisecond })
	_, program := testHome(t)
	a := testApp()
	s, err := newSession(a, program, false)
	if err != nil {
		t.Fatal(err)
	}
	o := openOffscreen(t, s)
	if o.r.sc.Page != pageWelcome || len(o.r.sc.Offers) != 5 {
		t.Fatalf("opened on %q with %d offers", o.r.sc.Page, len(o.r.sc.Offers))
	}
	ctx := context.Background()
	o.r.handle(ctx, started{Picks: map[string]bool{PickDesktop: true}})
	if o.r.sc.Page != pageWorking {
		t.Fatalf("pressing Install went to %q", o.r.sc.Page)
	}
	o.until(pageDone)
	if _, err := os.Stat(s.Exe); err != nil {
		t.Fatal("done, and the program is not installed:", err)
	}
	o.r.handle(ctx, opened{})
	if s.next != nextOpen {
		t.Error("Open did not leave the installed program to start")
	}

	gone, err := newSession(a, program, true)
	if err != nil {
		t.Fatal(err)
	}
	o = openOffscreen(t, gone)
	if o.r.sc.Page != pageRemove {
		t.Fatalf("uninstall opened on %q", o.r.sc.Page)
	}
	o.r.handle(ctx, removed{})
	o.until(pageRemoved)
	if _, err := os.Stat(s.Exe); err == nil {
		t.Fatal("removed, and the program is still there")
	}
}

// A failure shows, and Try Again goes back to the choices.
func TestWindowFailsAndGoesBack(t *testing.T) {
	leastWork = 0
	t.Cleanup(func() { leastWork = 1600 * time.Millisecond })
	_, program := testHome(t)
	a := testApp()
	a.Installed = func(context.Context, Installation) error { return os.ErrPermission }
	s, err := newSession(a, program, false)
	if err != nil {
		t.Fatal(err)
	}
	o := openOffscreen(t, s)
	o.r.handle(context.Background(), started{})
	o.until(pageFailed)
	if o.r.sc.Problem == "" {
		t.Error("the failure says nothing")
	}
	o.r.handle(context.Background(), retried{})
	o.until(pageWelcome)
}

// What the two halves say to each other would survive a socket.
func TestWire(t *testing.T) {
	if err := gunim.CheckWire(
		scene{Name: "x", Offers: []Offer{{Key: "k", Label: "l", On: true}}, Mode: Upgrade, Progress: 0.5},
		started{Picks: map[string]bool{"a": true}}, opened{}, ranHere{}, closed{}, retried{}, quitThem{}, removed{Data: true},
	); err != nil {
		t.Fatal(err)
	}
}

// installedOnce installs testApp, and returns a session to install it
// over itself, as a second download does.
func installedOnce(t *testing.T, a App) *Session {
	t.Helper()
	_, program := testHome(t)
	first, err := newSession(a, program, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Install(context.Background(), nil, nil); err != nil {
		t.Fatal(err)
	}
	s, err := newSession(a, program, false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// fastWatch makes the installer look for closed copies often, and wait
// little for one asked to close.
func fastWatch(t *testing.T) {
	was, wasClose, wasWork := watchEvery, closeWait, leastWork
	watchEvery, closeWait, leastWork = 5*time.Millisecond, 200*time.Millisecond, 0
	t.Cleanup(func() { watchEvery, closeWait, leastWork = was, wasClose, wasWork })
}

// A copy running holds the install up until it closes, and the install
// then goes on by itself.
func TestTheInstallWaitsForTheProgramToClose(t *testing.T) {
	fastWatch(t)
	s := installedOnce(t, testApp())
	o := openOffscreen(t, s)
	var open atomic.Bool
	open.Store(true)
	o.r.running = func() []int {
		if open.Load() {
			return []int{42}
		}
		return nil
	}
	o.r.handle(context.Background(), started{})
	o.until(pageRunning)
	open.Store(false)
	o.until(pageDone)
}

// Asked to, the program closes, with the ring turning meanwhile, and
// the install goes on; one that will not close is said to have stayed.
func TestCloseTheProgramFromTheInstaller(t *testing.T) {
	fastWatch(t)
	var open atomic.Bool
	open.Store(true)
	a := testApp()
	a.Quit = func(context.Context) error { open.Store(false); return nil }
	s := installedOnce(t, a)
	o := openOffscreen(t, s)
	o.r.running = func() []int {
		if open.Load() {
			return []int{42}
		}
		return nil
	}
	o.r.handle(context.Background(), started{})
	o.until(pageRunning)
	o.r.handle(context.Background(), quitThem{})
	if o.r.sc.Page != pageClosing {
		t.Fatalf("asked to close it, the installer is on %q", o.r.sc.Page)
	}
	o.until(pageDone)

	// One that stays open.
	stubborn := testApp()
	stubborn.Quit = func(context.Context) error { return nil }
	s = installedOnce(t, stubborn)
	o = openOffscreen(t, s)
	o.r.running = func() []int { return []int{42} }
	o.r.handle(context.Background(), started{})
	o.until(pageRunning)
	o.r.handle(context.Background(), quitThem{})
	o.until(pageRunning)
	if o.r.sc.Problem == "" {
		t.Fatal("a program that didn't close is not said to have stayed")
	}
	// Cancel still closes the window, and nothing was installed over it.
	o.r.handle(context.Background(), closed{})
}
