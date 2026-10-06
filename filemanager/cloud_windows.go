package filemanager

import (
	"io/fs"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	cldapi                      = windows.NewLazySystemDLL("cldapi.dll")
	procCfGetSyncRootInfoByPath = cldapi.NewProc("CfGetSyncRootInfoByPath")
)

// onlineOnly reports whether the item info describes keeps its contents online only, so reading them downloads them.
func onlineOnly(info fs.FileInfo) bool {
	return !info.IsDir() && cloudOf("", info) == CloudOnline
}

// cloudOf is the cloud state of the item info describes in the folder dir; "" for a folder not known.
func cloudOf(dir string, info fs.FileInfo) CloudState {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return CloudNone
	}
	return WindowsCloud(d.FileAttributes, info.IsDir(), dir != "" && inSyncRoot(dir))
}

// InCloudFolder reports whether the folder dir lies in a cloud provider's folder, a sync root, as OneDrive's does:
// for a program that sends Windows' attributes on, to say so beside them.
func InCloudFolder(dir string) bool { return inSyncRoot(dir) }

// syncRoots remembers which folders lie in a cloud provider's folder, as a folder's items are each asked about.
var syncRoots struct {
	sync.Mutex
	in map[string]bool
}

// inSyncRoot reports whether the folder dir lies in a cloud provider's folder, a sync root, as OneDrive's does.
func inSyncRoot(dir string) bool {
	syncRoots.Lock()
	in, ok := syncRoots.in[dir]
	syncRoots.Unlock()
	if ok {
		return in
	}
	in = askSyncRoot(dir)
	syncRoots.Lock()
	if syncRoots.in == nil || len(syncRoots.in) > 256 {
		syncRoots.in = map[string]bool{}
	}
	syncRoots.in[dir] = in
	syncRoots.Unlock()
	return in
}

// askSyncRoot asks Windows' cloud files API whether path lies in a sync root. Before that API, none does.
func askSyncRoot(path string) bool {
	if procCfGetSyncRootInfoByPath.Find() != nil {
		return false
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	// CF_SYNC_ROOT_INFO_BASIC, whose answer is the root's file ID.
	var basic [64]byte
	var got uint32
	hr, _, _ := procCfGetSyncRootInfoByPath.Call(uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&basic[0])), uintptr(len(basic)), uintptr(unsafe.Pointer(&got)))
	return hr == 0
}
