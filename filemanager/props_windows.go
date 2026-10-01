package filemanager

import (
	"fmt"
	"io/fs"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// fileTimes returns when info's file was made and last read.
func fileTimes(info fs.FileInfo) (created, accessed time.Time, ok bool) {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	return time.Unix(0, d.CreationTime.Nanoseconds()), time.Unix(0, d.LastAccessTime.Nanoseconds()), true
}

// readAttrs reads whether the item at path is read-only and hidden.
func readAttrs(path string) (readOnly, hidden, ok bool, err error) {
	attrs, err := getAttrs(path)
	if err != nil {
		return false, false, false, err
	}
	return attrs&windows.FILE_ATTRIBUTE_READONLY != 0, attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0, true, nil
}

// setAttrs makes the item at path read-only and hidden, or not.
func setAttrs(path string, readOnly, hidden bool) error {
	attrs, err := getAttrs(path)
	if err != nil {
		return err
	}
	for bit, on := range map[uint32]bool{windows.FILE_ATTRIBUTE_READONLY: readOnly, windows.FILE_ATTRIBUTE_HIDDEN: hidden} {
		if on {
			attrs |= bit
		} else {
			attrs &^= bit
		}
	}
	if attrs == 0 {
		attrs = windows.FILE_ATTRIBUTE_NORMAL
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("setting the attributes of %s: %w", path, err)
	}
	if err := windows.SetFileAttributes(p, attrs); err != nil {
		return fmt.Errorf("setting the attributes of %s: %w", path, err)
	}
	return nil
}

func getAttrs(path string) (uint32, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("reading the attributes of %s: %w", path, err)
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return 0, fmt.Errorf("reading the attributes of %s: %w", path, err)
	}
	return attrs, nil
}
