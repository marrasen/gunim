//go:build !darwin || ios

package speaker

import "github.com/marrasen/gunim/audio/asio"

// openSystem opens the system's own sound as a device that asks for its
// sound, where the system has one: oto plays it here.
var openSystem func(asio.Config) (pullDevice, error)
