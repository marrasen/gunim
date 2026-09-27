package main

import (
	"io/fs"
	"syscall"
)

// hiddenAttr reports whether Windows marks the item hidden.
func hiddenAttr(info fs.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
