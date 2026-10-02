// SPDX-License-Identifier: Apache-2.0

package glfw

// Depths returns, for each of ws, how deep it lies in the system's
// stack of windows on the screen: 0 for the frontmost, larger further
// back. Only the order of the numbers means anything. A window the
// system cannot place, such as a hidden one, gets -1, and so does
// every window where the system cannot say. Call it on the main
// thread. It is a gunim change.
func Depths(ws []*Window) []int {
	depths := make([]int, len(ws))
	for i := range depths {
		depths[i] = -1
	}
	if _glfw.initialized {
		platformDepths(ws, depths)
	}
	return depths
}
