//go:build !windows

package filemanager

import "io/fs"

// onlineOnly reports whether the item info describes keeps its contents online only. Outside Windows no file does:
// a cloud folder there is a mount that reads like any other.
func onlineOnly(fs.FileInfo) bool { return false }

// InCloudFolder reports whether the folder dir lies in a cloud provider's folder: none does, outside Windows.
func InCloudFolder(string) bool { return false }

// cloudOf is the cloud state of an item: none, outside Windows.
func cloudOf(string, fs.FileInfo) CloudState { return CloudNone }
