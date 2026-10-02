// SPDX-License-Identifier: Apache-2.0

package glfw

import "golang.org/x/sys/windows"

// stackWalk bounds the walk down the stack of top-level windows, which
// may change under it.
const stackWalk = 1 << 16

// platformDepths walks the top-level windows from the front, as
// GetTopWindow and GetWindow order them, topmost windows first, and
// counts the steps to each of ws. Nothing in the walk looks at the
// pointer, so neither a window under it that lets it through, such as
// a drag's picture, nor a window holding it on a button press changes
// the answer.
func platformDepths(ws []*Window, depths []int) {
	at := map[windows.HWND][]int{}
	for i, w := range ws {
		if w == nil || w.platform.handle == 0 || !_IsWindowVisible(w.platform.handle) || _IsIconic(w.platform.handle) {
			continue
		}
		at[w.platform.handle] = append(at[w.platform.handle], i)
	}
	if len(at) == 0 {
		return
	}
	found := 0
	h := _GetTopWindow()
	for step := 0; h != 0 && found < len(at) && step < stackWalk; step++ {
		if is, ok := at[h]; ok {
			for _, j := range is {
				depths[j] = step
			}
			found++
		}
		h = _GetWindow(h, _GW_HWNDNEXT)
	}
}
