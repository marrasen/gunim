//go:build linux

package desktop

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

// watchScreen follows the desktop's screensaver, which goes active as
// the screen blanks or locks, over the session bus: GNOME's, KDE's and
// Xfce's alike say so with ActiveChanged. It returns how to stop.
func watchScreen(d *Driver) (stop func()) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return func() {}
	}
	if err := conn.AddMatchSignal(dbus.WithMatchMember("ActiveChanged")); err != nil {
		_ = conn.Close()
		return func() {}
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	go func() {
		for s := range signals {
			if !strings.HasSuffix(s.Name, "ScreenSaver.ActiveChanged") || len(s.Body) != 1 {
				continue
			}
			if active, ok := s.Body[0].(bool); ok {
				d.post(func() { d.screenGone(active) })
			}
		}
	}()
	return func() {
		conn.RemoveSignal(signals)
		_ = conn.Close()
		close(signals)
	}
}

// screenGone notes the screen going off or locking, or coming back,
// for every window. It runs on the main thread.
func (d *Driver) screenGone(away bool) {
	d.screenAway = away
	for _, w := range d.windows {
		w.tellUnseen()
	}
}
