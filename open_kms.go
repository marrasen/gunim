//go:build gunimkms && linux

package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/kms"
)

// openDriver opens the KMS driver with its default options: the screen
// straight through the kernel, with no window system, as on a Raspberry
// Pi with a touch screen. A program that sets options opens the driver
// itself, with kms.Open, and runs with [MainWith].
func openDriver() (driver.Driver, error) { return kms.Open(kms.Options{}) }
