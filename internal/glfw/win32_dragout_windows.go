// SPDX-License-Identifier: Apache-2.0

package glfw

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is a gunim change: dragging files out of a window, to other
// programs, through OLE. The shell makes the data object, from the
// files' item IDs, so Explorer and every other drop target read it the
// way they read a drag from Explorer itself. The drop source, which
// OLE asks whether to go on, is a COM object written here.
//
// DoDragDrop runs a message loop of its own until the drop, on the main
// thread, and GLFW's windows keep getting their messages through it.
// It swallows the button's release, so the release is reported once it
// returns.

var (
	ole32                  = windows.NewLazySystemDLL("ole32.dll")
	shell32dnd             = windows.NewLazySystemDLL("shell32.dll")
	procOleInitialize      = ole32.NewProc("OleInitialize")
	procDoDragDrop         = ole32.NewProc("DoDragDrop")
	procCoTaskMemFree      = ole32.NewProc("CoTaskMemFree")
	procSHParseDisplayName = shell32dnd.NewProc("SHParseDisplayName")
	procSHCreateDataObject = shell32dnd.NewProc("SHCreateDataObject")
	procILFindLastID       = shell32dnd.NewProc("ILFindLastID")
)

const (
	dragdropSDrop              = 0x00040100
	dragdropSCancel            = 0x00040101
	dragdropSUseDefaultCursors = 0x00040102
	dropEffectCopy             = 1
	mkLButton                  = 0x0001
	eNoInterface               = 0x80004002
	sOK                        = 0
)

var (
	iidIUnknown    = windows.GUID{Data1: 0x00000000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropSource = windows.GUID{Data1: 0x00000121, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDataObject = windows.GUID{Data1: 0x0000010e, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// dropSource is an IDropSource: a pointer to its method table first,
// as COM lays an object out.
type dropSource struct {
	vtbl *dropSourceVtbl
	refs int32
}

type dropSourceVtbl struct {
	queryInterface    uintptr
	addRef            uintptr
	release           uintptr
	queryContinueDrag uintptr
	giveFeedback      uintptr
}

var (
	dropSourceMethods     dropSourceVtbl
	dropSourceMethodsOnce sync.Once
	oleOnce               sync.Once
	oleErr                error
)

func dropSourceTable() *dropSourceVtbl {
	dropSourceMethodsOnce.Do(func() {
		dropSourceMethods = dropSourceVtbl{
			queryInterface: syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
				id := (*windows.GUID)(unsafe.Pointer(riid))
				if *id != iidIUnknown && *id != iidIDropSource {
					*(*uintptr)(unsafe.Pointer(ppv)) = 0
					return eNoInterface
				}
				*(*uintptr)(unsafe.Pointer(ppv)) = this
				(*dropSource)(unsafe.Pointer(this)).refs++
				return sOK
			}),
			addRef: syscall.NewCallback(func(this uintptr) uintptr {
				s := (*dropSource)(unsafe.Pointer(this))
				s.refs++
				return uintptr(s.refs)
			}),
			release: syscall.NewCallback(func(this uintptr) uintptr {
				s := (*dropSource)(unsafe.Pointer(this))
				s.refs--
				return uintptr(s.refs)
			}),
			queryContinueDrag: syscall.NewCallback(func(this, escape, keys uintptr) uintptr {
				switch {
				case escape != 0:
					return dragdropSCancel
				case keys&mkLButton == 0:
					return dragdropSDrop
				}
				return sOK
			}),
			giveFeedback: syscall.NewCallback(func(this, effect uintptr) uintptr {
				return dragdropSUseDefaultCursors
			}),
		}
	})
	return &dropSourceMethods
}

func (w *Window) platformStartDragOut(paths []string, end func(taken bool)) error {
	oleOnce.Do(func() {
		if hr, _, _ := procOleInitialize.Call(0); int32(hr) < 0 {
			oleErr = fmt.Errorf("glfw: OleInitialize: HRESULT %#x", uint32(hr))
		}
	})
	if oleErr != nil {
		return oleErr
	}

	// The shell makes the data object from the folder holding the files
	// and the files' item IDs within it, the way Explorer describes a
	// selection. The files must share a folder.
	if len(paths) == 0 {
		return nil
	}
	dir := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		if filepath.Dir(p) != dir {
			return fmt.Errorf("glfw: files dragged out must share a folder: %s and %s", paths[0], p)
		}
	}
	var pidls []uintptr
	defer func() {
		for _, p := range pidls {
			_, _, _ = procCoTaskMemFree.Call(p)
		}
	}()
	parse := func(path string) (uintptr, error) {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return 0, err
		}
		var pidl uintptr
		hr, _, _ := procSHParseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0,
			uintptr(unsafe.Pointer(&pidl)), 0, 0)
		dragOutLog("SHParseDisplayName %s: HRESULT %#x", path, uint32(hr))
		if int32(hr) < 0 {
			return 0, fmt.Errorf("glfw: SHParseDisplayName %s: HRESULT %#x", path, uint32(hr))
		}
		pidls = append(pidls, pidl)
		return pidl, nil
	}
	folder, err := parse(dir)
	if err != nil {
		return err
	}
	children := make([]uintptr, 0, len(paths))
	for _, path := range paths {
		abs, err := parse(path)
		if err != nil {
			return err
		}
		// The last item of a file's full ID is its ID within its folder.
		child, _, _ := procILFindLastID.Call(abs)
		children = append(children, child)
	}
	var data uintptr
	hr, _, _ := procSHCreateDataObject.Call(folder, uintptr(len(children)), uintptr(unsafe.Pointer(&children[0])),
		0, uintptr(unsafe.Pointer(&iidIDataObject)), uintptr(unsafe.Pointer(&data)))
	dragOutLog("SHCreateDataObject: HRESULT %#x", uint32(hr))
	if int32(hr) < 0 {
		return fmt.Errorf("glfw: SHCreateDataObject: HRESULT %#x", uint32(hr))
	}
	defer comRelease(data)
	dragOutLog("the data object offers files (CF_HDROP): HRESULT %#x", queryHDrop(data))

	src := &dropSource{vtbl: dropSourceTable(), refs: 1}
	var effect uint32
	hr, _, _ = procDoDragDrop.Call(data, uintptr(unsafe.Pointer(src)), dropEffectCopy,
		uintptr(unsafe.Pointer(&effect)))
	dragOutLog("DoDragDrop: HRESULT %#x, effect %d", uint32(hr), effect)
	// OLE kept the source until here.
	src.refs = 0

	// DoDragDrop took the button's release; report it, so the window
	// sees the button up.
	for b := MouseButton(0); b <= MouseButtonLast; b++ {
		if w.mouseButtons[b] == Press {
			w.inputMouseClick(b, Release, getKeyMods())
		}
	}
	_ = _ReleaseCapture()

	if end != nil {
		end(uint32(hr) == dragdropSDrop && effect != 0)
	}
	return nil
}

// platformCancelDragOut does nothing on Windows, where DoDragDrop has
// returned by the time anything could ask.
func (w *Window) platformCancelDragOut() {}

// dragOutDebug is set by GUNIM_DEBUG_DRAGOUT=1, which logs each step of
// a drag out to standard error.
var dragOutDebug = os.Getenv("GUNIM_DEBUG_DRAGOUT") == "1"

func dragOutLog(format string, args ...any) {
	if dragOutDebug {
		fmt.Fprintf(os.Stderr, "gunim drag out: "+format+"\n", args...)
	}
}

// queryHDrop asks a data object whether it can give its files as a
// CF_HDROP, and returns its answer: 0 for yes.
func queryHDrop(obj uintptr) uint32 {
	format := struct {
		cf     uint16
		ptd    uintptr
		aspect uint32
		index  int32
		tymed  uint32
	}{cf: 15, aspect: 1, index: -1, tymed: 1}
	vtbl := *(*[6]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj))))
	hr, _, _ := syscall.SyscallN(vtbl[5], obj, uintptr(unsafe.Pointer(&format)))
	return uint32(hr)
}

// comRelease calls Release on a COM object.
func comRelease(obj uintptr) {
	if obj == 0 {
		return
	}
	vtbl := *(*[3]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj))))
	_, _, _ = syscall.SyscallN(vtbl[2], obj)
}
