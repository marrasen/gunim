//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"bufio"
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"syscall"
	"unsafe"
)

// takeEnv copies the variables the activity sets, where Go keeps its
// home, caches and settings, into Go's environment. Go reads the
// environment once, as the library loads, and on Android the loader
// hands it none; the activity sets these through the C library.
func takeEnv() {
	for _, k := range []string{"HOME", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME"} {
		ck := C.CString(k)
		if v := C.getenv(ck); v != nil {
			_ = os.Setenv(k, C.GoString(v))
		}
		C.free(unsafe.Pointer(ck))
	}
}

// toLogcat sends what the program writes to standard output and
// standard error to the system log, where adb logcat shows it. Android
// throws both away otherwise, and with them a panic's trace.
func toLogcat() {
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	for _, fd := range []int{1, 2} {
		if err := syscall.Dup3(int(w.Fd()), fd, 0); err != nil {
			return
		}
	}
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			line := C.CString(sc.Text())
			C.gunim_log(line)
			C.free(unsafe.Pointer(line))
		}
	}()
}

// hush keeps what the program writes out of the system's log: standard
// output and standard error, which Android throws away, and what the
// runtime writes as it dies, which it sends to the log by itself on
// Android. In place of a panic's message and its stacks the log gets one
// fixed line, where the thread that writes it gets to. A program that
// wants them calls [debug.SetCrashOutput], as package crashlog does, with
// a file of its own.
func hush() {
	// The runtime opens its way to the log as it first writes, and keeps
	// it: the socket that appears as it does leads nowhere from here on.
	before := logSockets()
	print("gunim: the rest goes into no log\n")
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		log.Fatal("gunim: android: no way to keep quiet")
	}
	after := logSockets()
	for fd := range before {
		if len(after) > len(before) {
			delete(after, fd)
		}
	}
	// With none new, the runtime has written before, and any may be its
	// own.
	for fd := range after {
		if err := syscall.Dup3(int(null.Fd()), fd, syscall.O_CLOEXEC); err != nil {
			os.Exit(2)
		}
	}
	_ = null.Close()
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	if err := debug.SetCrashOutput(w, debug.CrashOptions{}); err == nil {
		fd, err := syscall.Dup(int(r.Fd()))
		if err == nil {
			C.gunim_watch_crash(C.int(fd))
		}
	}
	// The runtime holds a descriptor of its own.
	_, _ = w.Close(), r.Close()
}

// logSockets returns the process's sockets to the system's log.
func logSockets() map[int]bool {
	fds := map[int]bool{}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return fds
	}
	for _, e := range entries {
		fd, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if a, err := syscall.Getpeername(fd); err == nil {
			if u, ok := a.(*syscall.SockaddrUnix); ok && u.Name == "/dev/socket/logdw" {
				fds[fd] = true
			}
		}
	}
	return fds
}
