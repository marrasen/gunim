//go:build !linux && !(windows && (amd64 || arm64))

package filemanager

import (
	"errors"
	"time"
)

// errNoTrash is returned where the app knows no trash for the system.
var errNoTrash = errors.New("moving to the trash is not supported on this system; delete permanently instead")

// noTrash is the trash of a system the app knows none for.
type noTrash struct{}

func (noTrash) Trash(string) (string, error) { return "", errNoTrash }

func (noTrash) Restore(string, string, time.Time, string) error { return errNoTrash }

func (noTrash) Describe(string) string { return "" }

// systemTrash returns a trash that says the system has none the app knows.
func systemTrash() (trasher, error) { return noTrash{}, nil }
