//go:build !android

package main

import (
	"os"
	"path/filepath"
)

// noBackupDir is a folder for what is of no use on another machine: only Android has one that its backups leave out.
func noBackupDir() (string, error) {
	d, err := os.UserCacheDir()
	return filepath.Join(d, "gunim-backup"), err
}
