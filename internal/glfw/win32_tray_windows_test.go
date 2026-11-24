package glfw

import (
	"testing"

	"golang.org/x/sys/windows"
)

// The tests here set the package's tray, trayNow, and the foreground hook, so none of them may run in parallel.

// stubTray puts a tray with t up on the window hwnd for a test, with the foreground hook recording what it is asked,
// and takes both away as the test ends.
func stubTray(t *testing.T, hwnd windows.HWND, icon TrayIcon) *[]windows.HWND {
	t.Helper()
	var asked []windows.HWND
	was := takeForeground
	takeForeground = func(h windows.HWND) { asked = append(asked, h) }
	trayNow = &trayState{hwnd: hwnd, t: icon}
	t.Cleanup(func() {
		trayNow = nil
		takeForeground = was
	})
	return &asked
}

// A click on the tray's message reaches OnNotifyClick, once, and not the icon's own click, with the program in the
// foreground before it runs.
func TestAClickOnTheTraysMessageCallsOnNotifyClick(t *testing.T) {
	var notified, clicked int
	const hwnd = windows.HWND(0x1234)
	var foregroundFirst bool
	var asked *[]windows.HWND
	asked = stubTray(t, hwnd, TrayIcon{
		OnNotifyClick: func() { notified++; foregroundFirst = len(*asked) == 1 },
		OnClick:       func() { clicked++ },
	})
	if r := trayProc(hwnd, trayMessage, 0, ninBalloonUserClick); r != 0 {
		t.Fatalf("the tray answered the click with %d, want 0", r)
	}
	if notified != 1 || clicked != 0 {
		t.Fatalf("a click on the message called OnNotifyClick %d times and OnClick %d; want once and never",
			notified, clicked)
	}
	if !foregroundFirst || (*asked)[0] != hwnd {
		t.Fatalf("the tray took the foreground through %v before OnNotifyClick ran: %v; want its own window first",
			*asked, foregroundFirst)
	}
	// With no OnNotifyClick, the click does nothing.
	trayNow.t.OnNotifyClick = nil
	trayProc(hwnd, trayMessage, 0, ninBalloonUserClick)
	if clicked != 0 {
		t.Fatalf("with no OnNotifyClick, a click on the message called OnClick %d times", clicked)
	}
}

// A message that leaves the screen, closed or timed out, reaches OnNotifyGone.
func TestAMessageLeavingTheScreenCallsOnNotifyGone(t *testing.T) {
	gone := 0
	const hwnd = windows.HWND(0x1234)
	stubTray(t, hwnd, TrayIcon{OnNotifyGone: func() { gone++ }})
	trayProc(hwnd, trayMessage, 0, ninBalloonHide)
	trayProc(hwnd, trayMessage, 0, ninBalloonTimeout)
	if gone != 2 {
		t.Fatalf("a message closed and one timed out called OnNotifyGone %d times, want twice", gone)
	}
}

// stubShell has the shell take each message, or refuse it with fail, and records what it was handed.
func stubShell(t *testing.T, fail error) *[]notifyIconData {
	t.Helper()
	var shown []notifyIconData
	was := notifyShell
	notifyShell = func(msg uint32, d *notifyIconData) error {
		if fail != nil {
			return fail
		}
		shown = append(shown, *d)
		return nil
	}
	t.Cleanup(func() { notifyShell = was })
	return &shown
}

// A click on a tagged message reaches OnNotifyTag with the tag of the message shown last, after OnNotifyClick and
// with the program in the foreground, as the balloon's click names no message. The tag stays in the process: the
// shell gets the title and body alone.
func TestAClickOnATaggedMessageCallsOnNotifyTagWithTheLastTag(t *testing.T) {
	const hwnd = windows.HWND(0x1234)
	var order []string
	var asked *[]windows.HWND
	asked = stubTray(t, hwnd, TrayIcon{
		OnNotifyClick: func() { order = append(order, "click") },
		OnNotifyTag: func(tag string) {
			order = append(order, "tag:"+tag)
			if len(*asked) != 1 {
				t.Errorf("OnNotifyTag ran with the foreground taken %d times, want once", len(*asked))
			}
		},
	})
	shown := stubShell(t, nil)
	tag := "<toast launch=\"x&y\">\x00'\"日本 🎉"
	if err := trayNow.notify("first", "body", "chat:1"); err != nil {
		t.Fatal(err)
	}
	if err := trayNow.notify("second", "body", tag); err != nil {
		t.Fatal(err)
	}
	trayProc(hwnd, trayMessage, 0, ninBalloonUserClick)
	if want := []string{"click", "tag:" + tag}; len(order) != 2 || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("a click on the message ran %q, want %q", order, want)
	}
	for _, d := range *shown {
		for _, field := range [][]uint16{d.szInfo[:], d.szInfoTitle[:], d.szTip[:]} {
			if s := windows.UTF16ToString(field); s == tag || s == "chat:1" {
				t.Fatalf("the shell was handed the tag %q", s)
			}
		}
	}
}

// A message the shell refuses keeps the tag of the one shown before it, as that one is still the last on screen.
func TestARefusedMessageKeepsTheTagBeforeIt(t *testing.T) {
	const hwnd = windows.HWND(0x1234)
	var heard []string
	stubTray(t, hwnd, TrayIcon{OnNotifyTag: func(tag string) { heard = append(heard, tag) }})
	stubShell(t, nil)
	if err := trayNow.notify("first", "body", "one"); err != nil {
		t.Fatal(err)
	}
	stubShell(t, windows.ERROR_ACCESS_DENIED)
	if err := trayNow.notify("second", "body", "two"); err == nil {
		t.Fatal("a refused message reported no error")
	}
	trayProc(hwnd, trayMessage, 0, ninBalloonUserClick)
	if len(heard) != 1 || heard[0] != "one" {
		t.Fatalf("a click after a refused message heard %q, want [one]", heard)
	}
}

// A message shown with no tag is heard with "".
func TestAnUntaggedMessageIsHeardWithAnEmptyTag(t *testing.T) {
	const hwnd = windows.HWND(0x1234)
	heard := []string{}
	stubTray(t, hwnd, TrayIcon{OnNotifyTag: func(tag string) { heard = append(heard, tag) }})
	stubShell(t, nil)
	if err := trayNow.notify("tagged", "body", "one"); err != nil {
		t.Fatal(err)
	}
	if err := trayNow.notify("plain", "body", ""); err != nil {
		t.Fatal(err)
	}
	trayProc(hwnd, trayMessage, 0, ninBalloonUserClick)
	if len(heard) != 1 || heard[0] != "" {
		t.Fatalf("a click on an untagged message heard %q, want [\"\"]", heard)
	}
}
