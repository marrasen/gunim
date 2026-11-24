package gunim

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/marrasen/gunim/driver"
)

// trayDriver is a driver with a tray kept in memory, as an application's test builds one.
type trayDriver struct {
	pumpDriver
	driver.OffscreenTray
}

// An application shows several tagged messages, and a click on one hears that message's tag, after OnNotifyClick.
func TestAClickOnATaggedMessageHearsItsTag(t *testing.T) {
	d := &trayDriver{}
	var heard []string
	err := MainWith(context.Background(), d, func(a *App) error {
		err := a.SetTray(Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))},
			OnNotifyClick: func() { heard = append(heard, "click") },
			OnNotifyTag:   func(tag string) { heard = append(heard, "tag:"+tag) }})
		if err != nil {
			return err
		}
		for _, tag := range []string{"chat:1", "chat:2 <&\"'>"} {
			if err := a.TrayNotifyTagged("New message", "body", tag); err != nil {
				return err
			}
		}
		return a.TrayNotify("Plain", "body")
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{1, 0, 2} {
		if err := d.Click(i); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"click", "tag:chat:2 <&\"'>", "click", "tag:chat:1", "click", "tag:"}
	if strings.Join(heard, "|") != strings.Join(want, "|") {
		t.Fatalf("clicks on messages 1, 0 and 2 heard %q, want %q", heard, want)
	}
	if n := d.Notices(); len(n) != 3 || n[2] != (driver.Notice{Title: "Plain", Body: "body"}) {
		t.Fatalf("the tray showed %+v", n)
	}
}

// A tag longer than MaxNotifyTag fails with ErrNotifyTag and shows nothing, and a driver with no tray fails with
// ErrNoTray.
func TestATaggedMessageFailsWithATagTooLongOrNoTray(t *testing.T) {
	d := &trayDriver{}
	err := MainWith(context.Background(), d, func(a *App) error {
		if err := a.TrayNotifyTagged("t", "b", "x"); !errors.Is(err, ErrNoTray) {
			t.Errorf("a message with no icon up returned %v, want ErrNoTray", err)
		}
		if err := a.SetTray(Tray{Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}}); err != nil {
			return err
		}
		if err := a.TrayNotifyTagged("t", "b", strings.Repeat("x", MaxNotifyTag)); err != nil {
			t.Errorf("a tag of MaxNotifyTag bytes returned %v", err)
		}
		if err := a.TrayNotifyTagged("t", "b", strings.Repeat("x", MaxNotifyTag+1)); !errors.Is(err, ErrNotifyTag) {
			t.Errorf("a tag of MaxNotifyTag+1 bytes returned %v, want ErrNotifyTag", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(d.Notices()); n != 1 {
		t.Fatalf("the tray showed %d messages, want 1", n)
	}
	if err := d.Click(1); err == nil {
		t.Fatal("a click on a message past those shown returned no error")
	}
	err = MainWith(context.Background(), &pumpDriver{}, func(a *App) error {
		if err := a.TrayNotifyTagged("t", "b", "x"); !errors.Is(err, ErrNoTray) {
			t.Errorf("a driver with no tray returned %v, want ErrNoTray", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
