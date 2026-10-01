//go:build !windows

package filemanager

import "io/fs"

// onlineOnly reports whether the item info describes keeps its contents online only. Outside Windows no file does:
// a cloud folder there is a mount that reads like any other.
func onlineOnly(fs.FileInfo) bool { return false }
