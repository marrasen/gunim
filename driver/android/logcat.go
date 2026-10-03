//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"bufio"
	"os"
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
