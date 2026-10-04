// SPDX-License-Identifier: Apache-2.0

package glfw

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is a gunim change: files dragged in from other programs,
// through OLE, so a window hears them while they are over it and can
// show what a drop would do. Each window registers a drop target, a
// COM object written here, which reads the files from the drag's data
// object as it enters and reports them wherever the drag moves.
//
// An elevated process keeps GLFW's WM_DROPFILES, which the message
// filter lets through from Explorer running unelevated; OLE drags stop
// at that boundary.

var (
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32.NewProc("ReleaseStgMedium")
)

const (
	dropEffectNone  = 0
	dropEffectLink  = 4
	cfHDrop         = 15
	tymedHGlobal    = 1
	dvAspectContent = 1
)

var iidIDropTarget = windows.GUID{Data1: 0x00000122, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

// dropTarget is an IDropTarget for one window: a pointer to its method
// table first, as COM lays an object out.
type dropTarget struct {
	vtbl   *dropTargetVtbl
	refs   int32
	window *Window
	// paths holds the files of the drag over the window, read as it
	// entered.
	paths []string
}

type dropTargetVtbl struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	dragEnter      uintptr
	dragOver       uintptr
	dragLeave      uintptr
	drop           uintptr
}

var (
	dropTargetMethods     dropTargetVtbl
	dropTargetMethodsOnce sync.Once
	// dropTargets keeps each window's drop target alive while OLE
	// holds it, by window handle.
	dropTargets = map[windows.HWND]*dropTarget{}
)

// initOLE starts OLE on the main thread, once.
func initOLE() error {
	oleOnce.Do(func() {
		if hr, _, _ := procOleInitialize.Call(0); int32(hr) < 0 {
			oleErr = fmt.Errorf("glfw: OleInitialize: HRESULT %#x", uint32(hr))
		}
	})
	return oleErr
}

// pointOf unpacks a POINTL passed by value, which arrives in one
// register.
func pointOf(pt uintptr) _POINT {
	return _POINT{x: int32(uint32(pt)), y: int32(uint32(uint64(pt) >> 32))}
}

// effectFor picks what a drop of files does, of what the source
// allows: a copy, or a link where it allows no copy.
func effectFor(allowed uint32, files bool) uint32 {
	switch {
	case !files:
		return dropEffectNone
	case allowed&dropEffectCopy != 0:
		return dropEffectCopy
	}
	return allowed & dropEffectLink
}

func dropTargetTable() *dropTargetVtbl {
	dropTargetMethodsOnce.Do(func() {
		dropTargetMethods = dropTargetVtbl{
			queryInterface: syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
				id := (*windows.GUID)(unsafe.Pointer(riid))
				if *id != iidIUnknown && *id != iidIDropTarget {
					*(*uintptr)(unsafe.Pointer(ppv)) = 0
					return eNoInterface
				}
				*(*uintptr)(unsafe.Pointer(ppv)) = this
				(*dropTarget)(unsafe.Pointer(this)).refs++
				return sOK
			}),
			addRef: syscall.NewCallback(func(this uintptr) uintptr {
				t := (*dropTarget)(unsafe.Pointer(this))
				t.refs++
				return uintptr(t.refs)
			}),
			release: syscall.NewCallback(func(this uintptr) uintptr {
				t := (*dropTarget)(unsafe.Pointer(this))
				t.refs--
				return uintptr(t.refs)
			}),
			dragEnter: syscall.NewCallback(func(this, data, keys, pt, effect uintptr) uintptr {
				t := (*dropTarget)(unsafe.Pointer(this))
				t.paths = filesOf(data)
				t.over(pt, effect)
				return sOK
			}),
			dragOver: syscall.NewCallback(func(this, keys, pt, effect uintptr) uintptr {
				(*dropTarget)(unsafe.Pointer(this)).over(pt, effect)
				return sOK
			}),
			dragLeave: syscall.NewCallback(func(this uintptr) uintptr {
				t := (*dropTarget)(unsafe.Pointer(this))
				if t.paths != nil {
					t.window.inputDragOver(0, 0, nil, false)
				}
				t.paths = nil
				return sOK
			}),
			drop: syscall.NewCallback(func(this, data, keys, pt, effect uintptr) uintptr {
				t := (*dropTarget)(unsafe.Pointer(this))
				paths := filesOf(data)
				t.paths = nil
				e := (*uint32)(unsafe.Pointer(effect))
				*e = effectFor(*e, paths != nil)
				if paths == nil {
					return sOK
				}
				p := pointOf(pt)
				if err := _ScreenToClient(t.window.platform.handle, &p); err == nil {
					// The pointer moves to where the files were let go
					// first, as GLFW's own drop does.
					t.window.inputCursorPos(float64(p.x), float64(p.y))
				}
				t.window.inputDrop(paths)
				return sOK
			}),
		}
	})
	return &dropTargetMethods
}

// over reports the drag at pt, in the screen, and answers what a drop
// there would do.
func (t *dropTarget) over(pt, effect uintptr) {
	e := (*uint32)(unsafe.Pointer(effect))
	*e = effectFor(*e, t.paths != nil)
	if t.paths == nil {
		return
	}
	p := pointOf(pt)
	if err := _ScreenToClient(t.window.platform.handle, &p); err != nil {
		return
	}
	t.window.inputDragOver(float64(p.x), float64(p.y), t.paths, true)
}

// filesOf returns the files a data object carries, or nil where it
// carries none.
func filesOf(obj uintptr) []string {
	if obj == 0 {
		return nil
	}
	format := struct {
		cf     uint16
		ptd    uintptr
		aspect uint32
		index  int32
		tymed  uint32
	}{cf: cfHDrop, aspect: dvAspectContent, index: -1, tymed: tymedHGlobal}
	var medium struct {
		tymed  uint32
		handle uintptr
		unk    uintptr
	}
	vtbl := *(*[4]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj))))
	hr, _, _ := syscall.SyscallN(vtbl[3], obj, uintptr(unsafe.Pointer(&format)), uintptr(unsafe.Pointer(&medium)))
	if int32(hr) < 0 || medium.handle == 0 {
		return nil
	}
	defer func() { _, _, _ = procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&medium))) }()
	drop := _HDROP(medium.handle)
	count := _DragQueryFileW(drop, 0xffffffff, nil)
	if count == 0 {
		return nil
	}
	paths := make([]string, count)
	for i := range paths {
		length := _DragQueryFileW(drop, uint32(i), nil)
		buffer := make([]uint16, length+1)
		_DragQueryFileW(drop, uint32(i), buffer)
		paths[i] = windows.UTF16ToString(buffer)
	}
	return paths
}

// acceptDrops has the window take files dropped on it: through a drop
// target, which hears them as they move over it, or through GLFW's
// WM_DROPFILES in an elevated process or where OLE fails.
func (w *Window) acceptDrops() {
	if windows.GetCurrentProcessToken().IsElevated() || initOLE() != nil {
		_DragAcceptFiles(w.platform.handle, true)
		return
	}
	t := &dropTarget{vtbl: dropTargetTable(), refs: 1, window: w}
	hr, _, _ := procRegisterDragDrop.Call(uintptr(w.platform.handle), uintptr(unsafe.Pointer(t)))
	if int32(hr) < 0 {
		_DragAcceptFiles(w.platform.handle, true)
		return
	}
	dropTargets[w.platform.handle] = t
}

// stopDrops takes the window's drop target away, as it closes.
func (w *Window) stopDrops() {
	if _, ok := dropTargets[w.platform.handle]; !ok {
		return
	}
	_, _, _ = procRevokeDragDrop.Call(uintptr(w.platform.handle))
	delete(dropTargets, w.platform.handle)
}
