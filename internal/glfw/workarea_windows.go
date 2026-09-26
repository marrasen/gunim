package glfw

// WorkareaAt is the work area of the monitor showing the point x, y in
// screen coordinates, or of the one nearest it: the part windows may
// use, without the taskbar. It asks Windows rather than the monitor
// list, so a monitor the list matched to no handle, as happens with some
// docks, still answers.
func WorkareaAt(x, y int) (xpos, ypos, width, height int, ok bool) {
	mi, ok := _GetMonitorInfoW(_MonitorFromPoint(int32(x), int32(y), _MONITOR_DEFAULTTONEAREST))
	if !ok {
		return 0, 0, 0, 0, false
	}
	return int(mi.rcWork.left), int(mi.rcWork.top), int(mi.rcWork.right - mi.rcWork.left), int(mi.rcWork.bottom - mi.rcWork.top), true
}
