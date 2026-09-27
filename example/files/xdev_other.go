//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether err says a rename crossed volumes.
func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
