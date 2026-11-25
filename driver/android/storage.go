//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// errNoDir is returned where Android gives a program no folder.
var errNoDir = errors.New("android: no folder to keep files in")

// NoBackupDir returns the program's own folder that Android never backs
// up, and never takes to the user's next phone, for what is of no use
// there, as a key that names the device. It lasts as long as the program
// is installed. What a program keeps under its home is backed up, unless
// gunimapk -backup=false keeps that out too.
//
// The folder is there once main runs. A package's init may come before it.
func NoBackupDir() (string, error) {
	var n C.int
	p := C.gunim_no_backup_dir(&n)
	if p == nil {
		return "", errNoDir
	}
	defer C.free(unsafe.Pointer(p))
	return goString(p, n), nil
}
