// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

package glfw

// platformSetInputRegion has nothing to tell the system: the window answers WM_NCHITTEST from the region as it asks.
// See inputRegionHit.
func (w *Window) platformSetInputRegion() error { return nil }

// inputRegionHit answers a WM_NCHITTEST for a point outside the window's input region with HTTRANSPARENT, so the
// system asks the window under it, and reports false for a point inside, which the window answers as ever.
func (w *Window) inputRegionHit(lParam _LPARAM) (uintptr, bool) {
	if !w.inputRegionSet {
		return 0, false
	}
	x, y := w.clientPoint(lParam)
	if w.inInputRegion(int(x), int(y)) {
		return 0, false
	}
	return _HTTRANSPARENT, true
}
