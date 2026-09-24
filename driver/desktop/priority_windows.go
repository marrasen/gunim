//go:build windows

package desktop

import "golang.org/x/sys/windows"

var procSetThreadPriority = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadPriority")

// threadPriorityHighest is THREAD_PRIORITY_HIGHEST: two above normal,
// which in a normal process matches the base priority of a normal
// thread in an above-normal process.
const threadPriorityHighest = 2

// raiseThread raises the calling OS thread's priority. A frame costs a
// few hundred microseconds, and at normal priority a burst of other
// work, such as antivirus scanning a flurry of short-lived processes,
// delays it by milliseconds and costs frames, while the compositor keeps
// its rate. Raising the threads that make frames keeps them on time and
// leaves the application's own goroutines at normal priority.
//
// The caller must hold its goroutine on the thread with
// runtime.LockOSThread until it exits, so the thread ends with it
// rather than returning, raised, to Go's pool.
func raiseThread() {
	_, _, _ = procSetThreadPriority.Call(uintptr(windows.CurrentThread()), threadPriorityHighest)
}
