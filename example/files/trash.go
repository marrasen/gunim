package main

import "errors"

// trasher moves items to the system's trash and back.
type trasher interface {
	// Trash moves path to the trash and returns where it went there, or
	// "" where the system does not say.
	Trash(path string) (string, error)
	// Restore puts back an item that was trashed from original and went to
	// trashed.
	Restore(original, trashed string) error
}

// errNoRestore is returned where the system cannot put trashed items back
// for the app.
var errNoRestore = errors.New("the app cannot take items back out of this system's trash; restore them from the trash itself")
