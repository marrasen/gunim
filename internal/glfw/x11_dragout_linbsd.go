// SPDX-License-Identifier: Apache-2.0

//go:build freebsd || linux || netbsd

package glfw

import (
	"net/url"
	"strings"
	"unsafe"
)

// This file is a gunim change: dragging files out of a window, to other
// programs, over XDND. GLFW takes drops; this makes a window the
// source of one.
//
// The drag rides on the pointer the window already holds: X gives the
// window every pointer event while a button is down, so the moves and
// the release arrive here, and each move finds the window under the
// pointer that takes drops and tells it where the drag is. The window
// owns XdndSelection for the length of the drag, and hands the files
// over as text/uri-list when the target asks for them.

// dragOut is the drag out of a window, if one is in progress.
type dragOut struct {
	// source is the window the drag comes from, or nil when there is
	// no drag.
	source *Window
	uris   []byte
	// target is the window under the pointer that takes drops, or
	// None, and version the XDND version it speaks. accepted says it
	// will take the drop.
	target   _XID
	version  int
	accepted bool
	// dropped is set once the drop is sent, while the target reads the
	// files and says it has finished.
	dropped bool
	end     func(taken bool)
}

var dragOutState dragOut

func (w *Window) platformStartDragOut(paths []string, end func(taken bool)) error {
	finishDragOut(false)
	var b strings.Builder
	for _, p := range paths {
		u := url.URL{Scheme: "file", Path: p}
		b.WriteString(u.String())
		b.WriteString("\r\n")
	}
	dragOutState = dragOut{source: w, uris: []byte(b.String()), end: end}
	xSetSelectionOwner(_glfw.platformWindow.display, _glfw.platformWindow.XdndSelection,
		w.platform.handle, _CurrentTime)

	// Start from where the pointer is now.
	var root, child _XID
	var rx, ry, wx, wy int32
	var mask uint32
	xQueryPointer(_glfw.platformWindow.display, w.platform.handle, &root, &child, &rx, &ry, &wx, &wy, &mask)
	dragOutMove(rx, ry, _CurrentTime)
	return nil
}

func (w *Window) platformCancelDragOut() {
	if dragOutState.source == w {
		finishDragOut(false)
	}
}

// finishDragOut ends the drag, telling the source how it went.
func finishDragOut(taken bool) {
	d := dragOutState
	dragOutState = dragOut{}
	if d.source != nil && d.end != nil {
		d.end(taken)
	}
}

// dragOutMove steers the drag to (x, y) on the root window.
func dragOutMove(x, y int32, t _Time) {
	d := &dragOutState
	if d.source == nil || d.dropped {
		return
	}
	target, version := dragOutTarget()
	if target != d.target {
		if d.target != _None {
			sendXdnd(d.target, xdndLeave(), [5]_Clong{_Clong(d.source.platform.handle)})
		}
		d.target, d.version, d.accepted = target, version, false
		if target != _None {
			sendXdnd(target, _glfw.platformWindow.XdndEnter, [5]_Clong{
				_Clong(d.source.platform.handle),
				_Clong(version) << 24,
				_Clong(_glfw.platformWindow.text_uri_list),
			})
		}
	}
	if d.target != _None {
		sendXdnd(d.target, _glfw.platformWindow.XdndPosition, [5]_Clong{
			_Clong(d.source.platform.handle),
			0,
			_Clong(x)<<16 | _Clong(y)&0xffff,
			_Clong(t),
			_Clong(_glfw.platformWindow.XdndActionCopy),
		})
	}
}

// dragOutRelease drops, when the window under the pointer said it
// would take the drop, and gives up otherwise.
func dragOutRelease(t _Time) {
	d := &dragOutState
	if d.source == nil || d.dropped {
		return
	}
	if d.target == _None || !d.accepted {
		if d.target != _None {
			sendXdnd(d.target, xdndLeave(), [5]_Clong{_Clong(d.source.platform.handle)})
		}
		finishDragOut(false)
		return
	}
	d.dropped = true
	sendXdnd(d.target, _glfw.platformWindow.XdndDrop, [5]_Clong{
		_Clong(d.source.platform.handle), 0, _Clong(t),
	})
}

// dragOutMessage handles the target's replies, and reports whether m
// was one.
func dragOutMessage(m *_XClientMessageEvent) bool {
	d := &dragOutState
	if d.source == nil || _XID(m.Data[0]) != d.target {
		return false
	}
	switch m.MessageType {
	case _glfw.platformWindow.XdndStatus:
		d.accepted = m.Data[1]&1 != 0
		return true
	case _glfw.platformWindow.XdndFinished:
		// Before version 5, finishing says nothing of whether it took
		// the drop; it was asked only after saying it would.
		finishDragOut(d.version < 5 || m.Data[1]&1 != 0)
		return true
	}
	return false
}

// dragOutTarget returns the window under the pointer that takes drops,
// and the XDND version it speaks, walking down from the root through
// the windows under the pointer.
func dragOutTarget() (_XID, int) {
	w := _glfw.platformWindow.root
	for range 32 {
		var root, child _XID
		var rx, ry, wx, wy int32
		var mask uint32
		if !xQueryPointer(_glfw.platformWindow.display, w, &root, &child, &rx, &ry, &wx, &wy, &mask) || child == _None {
			return _None, 0
		}
		if v, ok := xdndAware(child); ok {
			return child, v
		}
		w = child
	}
	return _None, 0
}

// xdndAware reports whether w takes drops, and the version it speaks,
// at most GLFW's.
func xdndAware(w _XID) (int, bool) {
	var data uintptr
	n := getWindowPropertyX11(w, _glfw.platformWindow.XdndAware, _XA_ATOM, &data)
	if data == 0 {
		return 0, false
	}
	defer xFree(data)
	if n == 0 {
		return 0, false
	}
	return min(int(*(*_Atom)(unsafe.Pointer(data))), _GLFW_XDND_VERSION), true
}

// dragOutProperty writes the files to the requestor's property, for a
// request to convert XdndSelection, and returns the property to reply
// with, or None.
func dragOutProperty(request *_XSelectionRequestEvent) _Atom {
	d := &dragOutState
	if d.source == nil || request.Property == _None {
		return _None
	}
	switch request.Target {
	case _glfw.platformWindow.TARGETS:
		xChangePropertyGeneric(_glfw.platformWindow.display, request.Requestor, request.Property,
			_XA_ATOM, _PropModeReplace,
			[]_Atom{_glfw.platformWindow.TARGETS, _glfw.platformWindow.text_uri_list})
		return request.Property
	case _glfw.platformWindow.text_uri_list:
		xChangePropertyGeneric(_glfw.platformWindow.display, request.Requestor, request.Property,
			_glfw.platformWindow.text_uri_list, _PropModeReplace, d.uris)
		return request.Property
	}
	return _None
}

// sendXdnd sends an XDND client message to w.
func sendXdnd(w _XID, typ _Atom, data [5]_Clong) {
	var ev _XEvent
	m := ev.xclient()
	m.Type = _ClientMessage
	m.Window = w
	m.MessageType = typ
	m.Format = 32
	m.Data = data
	xSendEvent(_glfw.platformWindow.display, w, false, _NoEventMask, &ev)
	xFlush(_glfw.platformWindow.display)
}

var xdndLeaveAtom _Atom

func xdndLeave() _Atom {
	if xdndLeaveAtom == 0 {
		xdndLeaveAtom = xInternAtom(_glfw.platformWindow.display, "XdndLeave", false)
	}
	return xdndLeaveAtom
}
