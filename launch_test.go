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

func TestOpenLinkGoesToTheBrowserForWebAddressesAlone(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 300), nil)
	if err := w.Client().OpenLink("https://example.com/privacy"); !errors.Is(err, driver.ErrNoLinkOpener) {
		t.Fatalf("with no browser, OpenLink returned %v, want ErrNoLinkOpener", err)
	}
	var opened []string
	w.Offscreen().SetLinkOpener(func(url string) error { opened = append(opened, url); return nil })
	for _, url := range []string{"https://example.com/privacy", "http://example.com/"} {
		if err := w.Client().OpenLink(url); err != nil {
			t.Fatalf("OpenLink(%q) returned %v", url, err)
		}
	}
	for _, url := range []string{"file:///etc/passwd", "/usr/bin/calc", "javascript:alert(1)", "intent://x"} {
		if err := w.Client().OpenLink(url); err == nil {
			t.Errorf("OpenLink(%q) went to the browser, want an error", url)
		}
	}
	if len(opened) != 2 || opened[0] != "https://example.com/privacy" || opened[1] != "http://example.com/" {
		t.Fatalf("the browser got %q, want the two web addresses alone", opened)
	}
}
