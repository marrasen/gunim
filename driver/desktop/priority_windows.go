//go:build windows

package desktop

import "golang.org/x/sys/windows"

var procSetThreadPriority = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadPriority")

// threadPriorityHighest is THREAD_PRIORITY_HIGHEST: two above the
// process's normal.
const threadPriorityHighest = 2

// raiseProcess puts the process in the above-normal priority class. A
// frame costs a few hundred microseconds, and at normal priority a burst
// of other work, such as antivirus scanning a flurry of short-lived
// processes, delays it by milliseconds and costs frames, while the
// compositor keeps its rate.
//
// Raising only gunim's own threads falls short, because threads it
// does not start are on the frame path too: the GL driver's, such as
// the one NVIDIA's threaded optimisation runs each context's commands
// on, and the Go runtime's, which hand goroutines between the raised
// threads. An error leaves the priority as it was.
func raiseProcess() {
	_ = windows.SetPriorityClass(windows.CurrentProcess(), windows.ABOVE_NORMAL_PRIORITY_CLASS)
}

// raiseThread raises the calling OS thread's priority, which keeps the
// threads that make frames ahead of the application's own goroutines.
//
// The caller must hold its goroutine on the thread with
// runtime.LockOSThread until it exits, so the thread ends with it
// rather than returning, raised, to Go's pool.
func raiseThread() {
	_, _, _ = procSetThreadPriority.Call(uintptr(windows.CurrentThread()), threadPriorityHighest)
}
