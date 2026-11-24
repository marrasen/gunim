//go:build linux || windows

package desktop

import (
	"image"

	"github.com/marrasen/gunim/driver"
)

// notifyIcon is the picture of t's messages.
func notifyIcon(t *driver.Tray) []image.Image {
	if len(t.NotifyIcon) > 0 {
		return t.NotifyIcon
	}
	return t.Icon
}
