// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build freebsd || linux || netbsd

package glfw

import "math"

// _Unsorted is XShapeCombineRectangles' ordering for rectangles in no particular order.
const _Unsorted = 0

// platformSetInputRegion sets the window's input shape to its input region, or back to the whole window. A window
// the pointer passes through whole keeps its empty input shape.
func (w *Window) platformSetInputRegion() error {
	xs := &_glfw.platformWindow.xshape
	if !xs.available || w.mousePassthrough {
		return nil
	}
	if !w.inputRegionSet {
		xs.CombineMask(_glfw.platformWindow.display, w.platform.handle, _ShapeInput, 0, 0, _None, _ShapeSet)
		xFlush(_glfw.platformWindow.display)
		return nil
	}
	clamp := func(v, lo, hi int) int { return max(lo, min(v, hi)) }
	rects := make([]_XRectangle, 0, len(w.inputRegion))
	for _, r := range w.inputRegion {
		if r.Empty() {
			continue
		}
		x0, y0 := clamp(r.Min.X, math.MinInt16, math.MaxInt16), clamp(r.Min.Y, math.MinInt16, math.MaxInt16)
		x1, y1 := clamp(r.Max.X, x0, x0+math.MaxUint16), clamp(r.Max.Y, y0, y0+math.MaxUint16)
		rects = append(rects, _XRectangle{X: int16(x0), Y: int16(y0), Width: uint16(x1 - x0), Height: uint16(y1 - y0)})
	}
	var first *_XRectangle
	if len(rects) > 0 {
		first = &rects[0]
	}
	// No rectangles at all leave the window an empty input shape: the pointer passes through it everywhere
	xs.CombineRectangles(_glfw.platformWindow.display, w.platform.handle, _ShapeInput, 0, 0, first, int32(len(rects)),
		_ShapeSet, _Unsorted)
	xFlush(_glfw.platformWindow.display)
	return nil
}
