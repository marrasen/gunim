//go:build linux

package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// A tray icon goes on the session bus with its menu, which the panel
// reads, and a line clicked there is picked. It needs a session bus
// with a tray on it, as a desktop has.
func TestATrayIconIsOnTheBusWithItsMenu(t *testing.T) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("no session bus:", err)
	}
	defer func() { _ = bus.Close() }()
	if err := bus.Object(sniWatcher, sniWatcherAt).Call("org.freedesktop.DBus.Peer.Ping", 0).Err; err != nil {
		t.Skip("no tray on the session bus:", err)
	}
	picked := make(chan int, 1)
	d := &Driver{}
	err = d.SetTray(driver.Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}, Tooltip: "test",
		Items:  []driver.TrayItem{{Title: "One", ID: 7}, {Separator: true}, {Title: "More", Items: []driver.TrayItem{{Title: "Two", ID: 8}}}},
		OnPick: func(id int) { picked <- id }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.SetTray(driver.Tray{}) }()
	menu := bus.Object(tray.s.name, sniMenuPath)
	var rev uint32
	var root menuLayout
	if err := menu.Call(ifDBusMenu+".GetLayout", 0, int32(0), int32(-1), []string{}).Store(&rev, &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 3 {
		t.Fatalf("the menu has %d lines, want 3", len(root.Children))
	}
	if err := menu.Call(ifDBusMenu+".Event", 0, int32(4), "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-picked:
		if id != 8 {
			t.Fatalf("the line clicked picked %d, want 8", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was picked")
	}
}
