//go:build !windows

package main

import "io/fs"

// hiddenAttr reports whether the system marks the item hidden, which only
// Windows does apart from the leading dot.
func hiddenAttr(fs.FileInfo) bool { return false }
