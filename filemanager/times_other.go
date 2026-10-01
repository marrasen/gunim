//go:build !windows && !linux

package filemanager

import (
	"io/fs"
	"time"
)

// fileTimes reports that the system's times are not read here.
func fileTimes(fs.FileInfo) (created, accessed time.Time, ok bool) {
	return time.Time{}, time.Time{}, false
}
