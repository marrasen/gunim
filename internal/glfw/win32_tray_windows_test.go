package glfw

import (
	"testing"

	"golang.org/x/sys/windows"
)

// A click on the tray's message reaches OnNotifyClick, once, and not the icon's own click.
func TestAClickOnTheTraysMessageCallsOnNotifyClick(t *testing.T) {
	var notified, clicked int
	const hwnd = windows.HWND(0x1234)
	trayNow = &trayState{hwnd: hwnd, t: TrayIcon{
		OnNotifyClick: func() { notified++ },
		OnClick:       func() { clicked++ },
	}}
	defer func() { trayNow = nil }()
	if r := trayProc(hwnd, trayMessage, 0, ninBalloonUserClick); r != 0 {
		t.Fatalf("the tray answered the click with %d, want 0", r)
	}
	if notified != 1 || clicked != 0 {
		t.Fatalf("a click on the message called OnNotifyClick %d times and OnClick %d; want once and never",
			notified, clicked)
	}
	// With no OnNotifyClick, the click does nothing.
	trayNow.t.OnNotifyClick = nil
	trayProc(hwnd, trayMessage, 0, ninBalloonUserClick)
	if clicked != 0 {
		t.Fatalf("with no OnNotifyClick, a click on the message called OnClick %d times", clicked)
	}
}
