// SPDX-License-Identifier: Apache-2.0

package glfw

import "github.com/ebitengine/purego/objc"

var sel_orderedIndex = objc.RegisterName("orderedIndex")

// platformDepths asks each window its orderedIndex, its place among the
// application's windows from the front.
// platformCovered cannot say on macOS yet.
func platformCovered([]*Window, int, int) (bool, bool) { return false, false }

func platformDepths(ws []*Window, depths []int) {
	for i, w := range ws {
		if w == nil || w.platform.object == 0 || !objc.Send[bool](w.platform.object, sel_isVisible) {
			continue
		}
		if n := objc.Send[int](w.platform.object, sel_orderedIndex); n >= 0 {
			depths[i] = n
		}
	}
}
