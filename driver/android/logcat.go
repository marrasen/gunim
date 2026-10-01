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
