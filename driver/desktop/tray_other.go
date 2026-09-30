//go:build darwin

package desktop

import "github.com/marrasen/gunim/driver"

// SetTray implements [driver.Trayer]: macOS's status items are not done
// yet.
func (d *Driver) SetTray(driver.Tray) error { return driver.ErrNoTray }
