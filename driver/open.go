package driver

import "errors"

// ErrNoDriver is returned by [Open] until a platform driver is wired
// in.
var ErrNoDriver = errors.New("driver: no platform driver built in; see the package doc for the plan")

// Open connects to the display server.
//
// The real implementation is a build-tagged file per platform, each
// returning a driver built on the forked Ebitengine GLFW port. This
// stub holds that place while the layers above it are designed, so they
// compile and their tests run on a machine with no display.
func Open() (Driver, error) { return nil, ErrNoDriver }
