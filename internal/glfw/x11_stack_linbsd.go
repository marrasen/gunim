// SPDX-License-Identifier: Apache-2.0

//go:build freebsd || linux || netbsd

package glfw

import "unsafe"

// platformDepths ranks the windows by the root window's children,
// which X lists from the back to the front. A window manager puts each
// window in a frame of its own, so a window stands where its top-level
// ancestor, the child of the root holding it, stands.
func platformDepths(ws []*Window, depths []int) {
	display, root := _glfw.platformWindow.display, _glfw.platformWindow.root
	at := map[_XID][]int{}
	for i, w := range ws {
		if w == nil || w.platform.handle == 0 || !w.platformWindowVisible() {
			continue
		}
		if top := topLevel(display, root, w.platform.handle); top != 0 {
			at[top] = append(at[top], i)
		}
	}
	if len(at) == 0 {
		return
	}
	_, kids, ok := queryTree(display, root)
	if !ok {
		return
	}
	for i, k := range kids {
		for _, j := range at[k] {
			depths[j] = len(kids) - 1 - i
		}
	}
}

// topLevel returns the child of root that holds w, or w itself when it
// is one, and 0 when X cannot say.
func topLevel(display uintptr, root, w _XID) _XID {
	// A window lies only a few frames deep; the bound keeps a broken
	// tree from holding the main thread.
	for range 64 {
		parent, _, ok := queryTree(display, w)
		if !ok || parent == 0 {
			return 0
		}
		if parent == root {
			return w
		}
		w = parent
	}
	return 0
}

// queryTree returns w's parent and its children, from the back to the
// front.
func queryTree(display uintptr, w _XID) (parent _XID, kids []_XID, ok bool) {
	var root _XID
	var list *_XID
	var n uint32
	if xQueryTree(display, w, &root, &parent, &list, &n) == 0 {
		return 0, nil, false
	}
	if list != nil {
		kids = append(kids, unsafe.Slice(list, n)...)
		xFree(uintptr(unsafe.Pointer(list)))
	}
	return parent, kids, true
}
