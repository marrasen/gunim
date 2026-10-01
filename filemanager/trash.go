package filemanager

import "errors"

// errNoRestore is returned where the system cannot put trashed items back
// for the app.
var errNoRestore = errors.New("the app cannot take items back out of this system's trash; restore them from the trash itself")
