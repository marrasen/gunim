//go:build windows && (amd64 || arm64)

package filemanager

import (
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSHAssocEnumHandlers = shell32.NewProc("SHAssocEnumHandlers")
	procSHOpenWithDialog    = shell32.NewProc("SHOpenWithDialog")
)

// The shell's IDs Open with uses.
var (
	bhidDataObject = windows.GUID{Data1: 0xB8C0BD9F, Data2: 0xED24, Data3: 0x455C,
		Data4: [8]byte{0x83, 0xE6, 0xD5, 0x39, 0x0C, 0x4F, 0xE8, 0xC4}}
	iidDataObject = windows.GUID{Data1: 0x0000010E, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

const (
	// assocFilterRecommended keeps the programs the system offers in its
	// own Open with menu.
	assocFilterRecommended = 0x1
	// OPENASINFO's flags: offer to always use the program chosen, keep
	// that for the extension, and open the file with it.
	oaifAllowRegistration = 0x1
	oaifRegisterExt       = 0x2
	oaifExec              = 0x4
	// errCancelled is HRESULT_FROM_WIN32(ERROR_CANCELLED), as the dialog
	// says when closed with no program chosen.
	errCancelled = 0x800704C7

	// IAssocHandler's methods, by their place in its table.
	vtAssocGetName   = 3
	vtAssocGetUIName = 4
	vtAssocInvoke    = 8
)

// openAsInfo is OPENASINFO.
type openAsInfo struct {
	file  *uint16
	class *uint16
	flags uint32
}

func sysOpenWithWorks() bool {
	return procSHAssocEnumHandlers.Find() == nil && procSHOpenWithDialog.Find() == nil
}

func sysOpenWithApps(ext string) ([]OpenWithApp, error) {
	var apps []OpenWithApp
	err := onCOM(func() error {
		return eachHandler(ext, func(h unsafe.Pointer) bool {
			id, err := handlerString(h, vtAssocGetName)
			if err != nil {
				return true
			}
			name, err := handlerString(h, vtAssocGetUIName)
			if err != nil {
				return true
			}
			apps = append(apps, OpenWithApp{ID: id, Name: name})
			return true
		})
	})
	return apps, err
}

func sysOpenWithApp(path, id string) error {
	return onCOM(func() error {
		found := false
		var err error
		ext := extOf(filepath.Base(path))
		if eerr := eachHandler(ext, func(h unsafe.Pointer) bool {
			if got, gerr := handlerString(h, vtAssocGetName); gerr != nil || got != id {
				return true
			}
			found = true
			err = invokeHandler(h, path)
			return false
		}); eerr != nil {
			return eerr
		}
		if !found {
			return errors.New("the program is no longer offered for this file")
		}
		return err
	})
}

func sysOpenWithDialog(path string) error {
	return onCOM(func() error {
		file, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		info := openAsInfo{file: file, flags: oaifAllowRegistration | oaifRegisterExt | oaifExec}
		hr, _, _ := procSHOpenWithDialog.Call(0, uintptr(unsafe.Pointer(&info)))
		if uint32(hr) == errCancelled || !failed(uint32(hr)) {
			return nil
		}
		return fmt.Errorf("the Open with dialog: HRESULT %#x", uint32(hr))
	})
}

// eachHandler calls fn with each program the system offers for files of
// the extension ext, until fn returns false. It runs on COM's thread.
func eachHandler(ext string, fn func(h unsafe.Pointer) bool) error {
	e, err := windows.UTF16PtrFromString(ext)
	if err != nil {
		return err
	}
	var enum unsafe.Pointer
	hr, _, _ := procSHAssocEnumHandlers.Call(uintptr(unsafe.Pointer(e)), assocFilterRecommended,
		uintptr(unsafe.Pointer(&enum)))
	if failed(uint32(hr)) {
		return fmt.Errorf("listing the programs for %s: HRESULT %#x", ext, uint32(hr))
	}
	if enum == nil {
		return nil
	}
	defer release(enum)
	for {
		var h unsafe.Pointer
		var got uint32
		hr := comCall(enum, vtEnumNext, 1, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(&got)))
		if failed(hr) {
			return fmt.Errorf("listing the programs for %s: HRESULT %#x", ext, hr)
		}
		if got == 0 || h == nil {
			return nil
		}
		more := fn(h)
		release(h)
		if !more {
			return nil
		}
	}
}

// handlerString returns what the IAssocHandler h's method index says: a
// string the shell allocates, which it frees.
func handlerString(h unsafe.Pointer, index int) (string, error) {
	var s *uint16
	if hr := comCall(h, index, uintptr(unsafe.Pointer(&s))); failed(hr) {
		return "", fmt.Errorf("HRESULT %#x", hr)
	}
	if s == nil {
		return "", errors.New("no name")
	}
	defer func() { _, _, _ = procCoTaskMemFree.Call(uintptr(unsafe.Pointer(s))) }()
	return windows.UTF16PtrToString(s), nil
}

// invokeHandler opens the file at path with the IAssocHandler h.
func invokeHandler(h unsafe.Pointer, path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var item unsafe.Pointer
	hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if failed(uint32(hr)) {
		return fmt.Errorf("finding %s: HRESULT %#x", path, uint32(hr))
	}
	defer release(item)
	var data unsafe.Pointer
	if hr := comCall(item, vtBindToHandler, 0, uintptr(unsafe.Pointer(&bhidDataObject)),
		uintptr(unsafe.Pointer(&iidDataObject)), uintptr(unsafe.Pointer(&data))); failed(hr) {
		return fmt.Errorf("reading %s: HRESULT %#x", path, hr)
	}
	defer release(data)
	if hr := comCall(h, vtAssocInvoke, uintptr(data)); failed(hr) {
		return fmt.Errorf("HRESULT %#x", hr)
	}
	return nil
}
