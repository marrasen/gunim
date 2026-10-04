package vst3

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// library is a DLL loaded.
type library struct {
	h windows.Handle
}

// openLibrary loads the DLL at path and runs its entry, InitDll, where it
// has one.
func openLibrary(path string) (library, error) {
	h, err := windows.LoadLibraryEx(path, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return library{}, err
	}
	l := library{h: h}
	if entry, err := windows.GetProcAddress(h, "InitDll"); err == nil {
		if r, _, _ := syscall.SyscallN(entry); r&0xff == 0 {
			_ = windows.FreeLibrary(h)
			return library{}, errEntry
		}
	}
	return l, nil
}

func (l library) symbol(name string) (uintptr, error) { return windows.GetProcAddress(l.h, name) }

// close runs the module's exit, ExitDll, and unloads it.
func (l library) close() error {
	if l.h == 0 {
		return nil
	}
	if exit, err := windows.GetProcAddress(l.h, "ExitDll"); err == nil {
		_, _, _ = syscall.SyscallN(exit)
	}
	return windows.FreeLibrary(l.h)
}

// callFunc calls the function fn with no arguments.
func callFunc(fn uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn)
	return r
}
