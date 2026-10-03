//go:build android

package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/android"
)

// openDriver opens the Android driver, which the activity has started.
func openDriver() (driver.Driver, error) { return android.Open() }
