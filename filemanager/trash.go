package filemanager

import (
	"errors"
	"time"
)

// trasher moves items to the system's trash and back.
type trasher interface {
	// Trash moves path to the trash and returns where it went there, or
	// "" where the system does not say.
	Trash(path string) (string, error)
	// Restore puts the item trashed from original at time at, which went
	// to trashed, back at to. Nothing may be at to.
	Restore(original, trashed string, at time.Time, to string) error
	// Describe says what the item that went to trashed is, for a question
	// about restoring it.
	Describe(trashed string) string
}

// errNoRestore is returned where the system cannot put trashed items back
// for the app.
var errNoRestore = errors.New("the app cannot take items back out of this system's trash; restore them from the trash itself")
