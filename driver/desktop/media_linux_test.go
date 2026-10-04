//go:build linux

package desktop

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

func TestWhatPlaysShowsOnTheSessionBusAndLeaves(t *testing.T) {
	reader, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = reader.Close() }()
	d := &Driver{}
	np := &driver.NowPlaying{Title: "Night Drive", Artist: "The Oscillators", Album: "Made in Code",
		Cover: []byte("\x89PNG not really"), Length: 96 * time.Second, Position: 10 * time.Second, Playing: true}
	if err := d.SetNowPlaying(np); err != nil {
		t.Fatal(err)
	}
	name := player.name
	defer func() { _ = d.SetNowPlaying(nil) }()

	obj := reader.Object(name, mprisPath)
	var all map[string]dbus.Variant
	if err := obj.Call(dbusProps+".GetAll", 0, mprisPlayer).Store(&all); err != nil {
		t.Fatal(err)
	}
	md, _ := all["Metadata"].Value().(map[string]dbus.Variant)
	if md["xesam:title"].Value() != "Night Drive" || all["PlaybackStatus"].Value() != "Playing" {
		t.Fatalf("the player shows %v, %v; want Night Drive, playing", md["xesam:title"], all["PlaybackStatus"])
	}
	if l, _ := md["mpris:length"].Value().(int64); l != 96_000_000 {
		t.Fatalf("its length is %d µs, want 96 s", l)
	}
	art, _ := md["mpris:artUrl"].Value().(string)
	if _, err := os.Stat(art[len("file://"):]); err != nil {
		t.Fatalf("its cover %q: %v", art, err)
	}
	// The position is worked out as it is asked for, as it plays on.
	time.Sleep(200 * time.Millisecond)
	var pos dbus.Variant
	if err := obj.Call(dbusProps+".Get", 0, mprisPlayer, "Position").Store(&pos); err != nil {
		t.Fatal(err)
	}
	if p, _ := pos.Value().(int64); p < 10_150_000 || p > 11_000_000 {
		t.Fatalf("the position is %d µs, want a little past 10 s", p)
	}

	if err := d.SetNowPlaying(nil); err != nil {
		t.Fatal(err)
	}
	var owned bool
	if err := reader.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&owned); err != nil || owned {
		t.Fatalf("after nil the name %s is still owned (%v)", name, err)
	}
	if _, err := os.Stat(art[len("file://"):]); err == nil {
		t.Fatal("after nil the cover's file is still there")
	}
}
