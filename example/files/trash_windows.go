//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = shell32.NewProc("SHFileOperationW")
)

// SHFileOperation's function and flags.
const (
	foDelete           = 3
	fofSilent          = 0x4
	fofNoConfirmation  = 0x10
	fofAllowUndo       = 0x40
	fofNoErrorUI       = 0x400
	fofWantNukeWarning = 0x4000
)

// shFileOpStruct is SHFILEOPSTRUCTW as 64-bit Windows lays it out.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// recycleBin is the Windows Recycle Bin.
type recycleBin struct{}

// systemTrash returns the Recycle Bin.
func systemTrash() (trasher, error) { return recycleBin{}, nil }

// Trash implements [trasher] with SHFileOperation, which asks first where
// an item is too large for the Recycle Bin and would be deleted for good.
func (recycleBin) Trash(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("moving %s to the Recycle Bin: %w", path, err)
	}
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return "", fmt.Errorf("moving %s to the Recycle Bin: %w", abs, err)
	}
	// The list of paths ends with a second NUL.
	from = append(from, 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofSilent | fofNoConfirmation | fofAllowUndo | fofNoErrorUI | fofWantNukeWarning,
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	switch {
	case r != 0:
		return "", fmt.Errorf("moving %s to the Recycle Bin failed with code %#x", abs, r)
	case op.fAnyOperationsAborted != 0:
		return "", fmt.Errorf("moving %s to the Recycle Bin was stopped", abs)
	}
	return inRecycleBin, nil
}

// inRecycleBin is where Trash says an item went; Restore finds it again by
// where it came from and when.
const inRecycleBin = "the Recycle Bin"
