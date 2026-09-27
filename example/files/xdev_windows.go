package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isCrossDevice reports whether err says a rename crossed volumes.
func isCrossDevice(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }
