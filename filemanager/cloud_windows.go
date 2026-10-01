package filemanager

import (
	"io/fs"
	"syscall"
)

// The attributes Windows gives a file whose contents a cloud provider keeps online, such as OneDrive's Files
// On-Demand: reading the contents, or for recallOnOpen opening the file, downloads it.
const (
	fileAttributeOffline            = 0x00001000
	fileAttributeRecallOnOpen       = 0x00040000
	fileAttributeRecallOnDataAccess = 0x00400000
)

// onlineOnly reports whether the item info describes keeps its contents online only, so reading them downloads them.
func onlineOnly(info fs.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || info.IsDir() {
		return false
	}
	return d.FileAttributes&(fileAttributeOffline|fileAttributeRecallOnOpen|fileAttributeRecallOnDataAccess) != 0
}
