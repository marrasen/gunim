package glfw

import "github.com/ebitengine/purego/objc"

// SystemFullScreen reports whether the window is in the system's own
// full screen, as the green button puts it on macOS: not the full
// screen an application asks for with a monitor. It runs on the main
// thread.
func (w *Window) SystemFullScreen() bool {
	mask := uintptr(objc.Send[uint64](w.platform.object, sel_styleMask))
	return mask&NSWindowStyleMaskFullScreen != 0
}
