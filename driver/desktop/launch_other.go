//go:build !windows && !linux

package desktop

import "github.com/marrasen/gunim/driver"

// Open implements [driver.Launcher]; this platform cannot open files in
// gunim yet.
func (w *Window) Open(string) error { return driver.ErrNoLauncher }

// Reveal implements [driver.Launcher]; this platform cannot show files in
// gunim yet.
func (w *Window) Reveal(string) error { return driver.ErrNoLauncher }
