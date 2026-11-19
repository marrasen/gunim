// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build freebsd || linux || netbsd

package glfw

import (
	"strings"

	"github.com/ebitengine/purego"
)

// gunim change: Mesa's d3d12 driver before 23.3, the one WSL draws GL with on
// the graphics card, overruns every buffer object it touches from its ninth
// context on. It numbers its first sixteen contexts 0 to 15 and keeps a
// byte for each context and batch in each buffer object, but sized the
// table [8][16] and indexed it [context][batch], so a context numbered 8 or
// more writes past the object's end. A window opening as the ninth context,
// as a plugin's window does beside marrdaw's spare popups, crashed in
// glXSwapBuffers, the heap already broken, in d3d12_bo_get_base or
// d3d12_process_batch_residency. Mesa 23.3 sized the table [16][8].
//
// On such a driver the first context made makes sixteen spare ones before
// it: with none made before, they take numbers 0 to 15 in order. The eight
// numbered 0 to 7 go again at once, for the windows' contexts. The eight
// numbered 8 to 15 stay, never current, so each context past the eighth gets
// no number, and Mesa keeps its byte in a table of the batch's own.
const (
	_GLX_RENDERER_DEVICE_ID_MESA = 0x8184
	_GLX_RENDERER_VERSION_MESA   = 0x8185

	// d3d12ContextIDs is how many contexts Mesa's d3d12 driver numbers, and
	// d3d12SafeContextIDs how many of those its table has room for.
	d3d12ContextIDs     = 16
	d3d12SafeContextIDs = 8
)

// d3d12OverrunsContextIDs reports whether a GLX renderer of this device and
// Mesa version is Mesa's d3d12 driver with the table too small for its
// context numbers.
func d3d12OverrunsContextIDs(device string, version [3]uint32) bool {
	if !strings.HasPrefix(device, "D3D12") {
		return false
	}
	return version[0] < 23 || version[0] == 23 && version[1] < 3
}

// glxRendererOverrunsContextIDs asks GLX_MESA_query_renderer, which needs no
// context, whether the screen draws with a d3d12 driver that overruns.
func glxRendererOverrunsContextIDs() bool {
	if !extensionSupportedGLX("GLX_MESA_query_renderer") {
		return false
	}
	stringProc := getProcAddressGLX("glXQueryRendererStringMESA")
	integerProc := getProcAddressGLX("glXQueryRendererIntegerMESA")
	if stringProc == 0 || integerProc == 0 {
		return false
	}
	var queryString func(display uintptr, screen, renderer, attribute int32) uintptr
	var queryInteger func(display uintptr, screen, renderer, attribute int32, value *uint32) bool
	purego.RegisterFunc(&queryString, stringProc)
	purego.RegisterFunc(&queryInteger, integerProc)

	display, screen := _glfw.platformWindow.display, int32(_glfw.platformWindow.screen)
	device := goString(queryString(display, screen, 0, _GLX_RENDERER_DEVICE_ID_MESA))
	var version [3]uint32
	if !queryInteger(display, screen, 0, _GLX_RENDERER_VERSION_MESA, &version[0]) {
		return false
	}
	return d3d12OverrunsContextIDs(device, version)
}

// spareD3D12ContextIDs, the first time a context is made, holds the context
// numbers past the table's room on a d3d12 driver that overruns, with
// contexts of fbconfig that are never made current. See the note above.
func spareD3D12ContextIDs(fbconfig uintptr) {
	glx := &_glfw.platformContext.glx
	if glx.d3d12Checked {
		return
	}
	glx.d3d12Checked = true
	if !glxRendererOverrunsContextIDs() {
		return
	}
	display := _glfw.platformWindow.display
	grabErrorHandlerX11()
	made := make([]uintptr, 0, d3d12ContextIDs)
	for range d3d12ContextIDs {
		ctx := glx.CreateNewContext(display, fbconfig, _GLX_RGBA_TYPE, 0, true)
		if ctx == 0 {
			break
		}
		made = append(made, ctx)
	}
	releaseErrorHandlerX11()
	free := min(d3d12SafeContextIDs, len(made))
	for _, ctx := range made[:free] {
		glx.DestroyContext(display, ctx)
	}
	glx.d3d12Spares = made[free:]
}

// releaseD3D12Spares lets the spare contexts go, before the display closes.
func releaseD3D12Spares() {
	glx := &_glfw.platformContext.glx
	for _, ctx := range glx.d3d12Spares {
		glx.DestroyContext(_glfw.platformWindow.display, ctx)
	}
	glx.d3d12Spares = nil
	glx.d3d12Checked = false
}
