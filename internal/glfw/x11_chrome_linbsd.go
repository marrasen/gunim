// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build freebsd || linux || netbsd

// gunim change: this file is gunim's. A chromeless window on X11 has no
// decorations, and the application hands moves and resizes to the
// window manager with _NET_WM_MOVERESIZE, as a title bar would.

package glfw

func (w *Window) platformSetChromeless(on bool) error {
	if w.platform.chromeless == on {
		return nil
	}
	w.platform.chromeless = on
	w.decorated = !on
	return w.platformSetWindowDecorated(!on)
}

func (w *Window) platformChromeless() bool { return w.platform.chromeless }

func (w *Window) platformStartMoveResize(direction int) error {
	display := _glfw.platformWindow.display
	var root, child _XID
	var rootX, rootY, winX, winY int32
	var mask uint32
	xQueryPointer(display, w.platform.handle, &root, &child, &rootX, &rootY, &winX, &winY, &mask)
	// The press that started this holds the pointer; the window manager
	// needs it.
	xUngrabPointer(display, _CurrentTime)
	atom := xInternAtom(display, "_NET_WM_MOVERESIZE", false)
	// Button 1, and a source that is an ordinary application.
	sendEventToWM(w, atom, int(rootX), int(rootY), direction, 1, 1)
	xFlush(display)
	return nil
}
