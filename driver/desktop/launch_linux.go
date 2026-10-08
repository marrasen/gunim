//go:build linux

package desktop

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/marrasen/gunim/driver"
)

var _ driver.Launcher = (*Window)(nil)

// Open implements [driver.Launcher] with the desktop portal's OpenURI,
// which opens the file with the program the desktop keeps for it.
func (w *Window) Open(path string) error { return w.openURI("OpenFile", path) }

// Reveal implements [driver.Launcher] with the desktop portal's OpenURI,
// which opens the folder holding path in the desktop's file manager.
func (w *Window) Reveal(path string) error { return w.openURI("OpenDirectory", path) }

var _ driver.LinkOpener = (*Window)(nil)

// OpenLink implements [driver.LinkOpener] with the desktop portal's
// OpenURI, which opens the page in the desktop's browser.
func (w *Window) OpenLink(url string) error {
	parent, err := w.portalParent()
	if err != nil {
		return err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("desktop: reaching the session bus to open %s: %w", url, err)
	}
	defer func() { _ = conn.Close() }()
	var handle dbus.ObjectPath
	desk := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	if err := desk.Call("org.freedesktop.portal.OpenURI.OpenURI", 0,
		parent, url, map[string]dbus.Variant{}).Store(&handle); err != nil {
		return fmt.Errorf("desktop: opening %s: %w", url, err)
	}
	return nil
}

// openURI hands path to the OpenURI portal's method, owned by w.
func (w *Window) openURI(method, path string) error {
	parent, err := w.portalParent()
	if err != nil {
		return err
	}
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("desktop: opening %s: %w", path, err)
	}
	defer func() { _ = unix.Close(fd) }()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("desktop: reaching the session bus to open %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()
	var handle dbus.ObjectPath
	desk := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	if err := desk.Call("org.freedesktop.portal.OpenURI."+method, 0,
		parent, dbus.UnixFD(fd), map[string]dbus.Variant{}).Store(&handle); err != nil {
		return fmt.Errorf("desktop: opening %s: %w", path, err)
	}
	return nil
}
