//go:build !android

package main

import (
	"context"
	"os"
	"path/filepath"
)

// schedule starts the work at once: a desktop lets a program work with its window out of sight.
func schedule() error {
	jobs <- job{on: true}
	return nil
}

// finish has no one to tell.
func finish() error { return nil }

// folder is where the count is kept.
func folder() (string, error) {
	d, err := os.UserCacheDir()
	return filepath.Join(d, "gunim-background"), err
}

// from says nothing: the system picks the network.
func from(context.Context, int64) string { return "" }
