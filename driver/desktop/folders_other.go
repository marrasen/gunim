//go:build !linux && !windows

package desktop

import "github.com/marrasen/gunim/driver"

// systemFolder leaves the folder to its usual name in the home.
func systemFolder(driver.UserFolder) string { return "" }
