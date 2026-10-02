//go:build darwin

package desktop

import "github.com/marrasen/gunim/driver"

// closeTray has no icon to take away.
func closeTray() {}

// SetTray implements [driver.Trayer]: macOS's status items are not done
// yet.
func (d *Driver) SetTray(driver.Tray) error { return driver.ErrNoTray }

// TrayNotify implements [driver.TrayNotifier]: there is no tray here.
func (d *Driver) TrayNotify(string, string) error { return driver.ErrNoTray }
