package gunim

import (
	"errors"
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

func TestOpenAndRevealGoToTheWindowsLauncher(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 300), nil)
	var opened, revealed string
	w.Offscreen().SetLauncher(
		func(p string) error { opened = p; return nil },
		func(p string) error { revealed = p; return errors.New("no file manager") })
	if err := w.Client().Open("a/report.pdf"); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Reveal("a/b"); err == nil || err.Error() != "no file manager" {
		t.Fatalf("Reveal returned %v, want the launcher's error", err)
	}
	if opened != "a/report.pdf" || revealed != "a/b" {
		t.Fatalf("opened %q and revealed %q", opened, revealed)
	}
}

func TestOpenSaysWhenThereIsNoLauncher(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 300), nil)
	if err := w.Client().Open("x"); !errors.Is(err, driver.ErrNoLauncher) {
		t.Fatalf("Open returned %v, want ErrNoLauncher", err)
	}
	if err := w.Client().Reveal("x"); !errors.Is(err, driver.ErrNoLauncher) {
		t.Fatalf("Reveal returned %v, want ErrNoLauncher", err)
	}
}
