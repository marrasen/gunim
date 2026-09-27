package main

import (
	"path/filepath"
	"strings"
)

// volumeOf names the volume the folder at path is on: its drive or its
// share.
func volumeOf(path string) (string, error) {
	return strings.ToUpper(filepath.VolumeName(path)), nil
}
