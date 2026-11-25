//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// errNoJava is returned by Env before Java has loaded the program.
var errNoJava = errors.New("android: no Java yet")

// Env runs f on a thread attached to the Java VM, and returns once f
// has, for a program that calls Java, its own or Android's, with cgo of
// its own. env is the thread's JNIEnv, a *C.JNIEnv to a program that
// includes jni.h, and context a jobject, the application's
// android.content.Context, which outlives any activity:
//
//	err := android.Env(func(env unsafe.Pointer, context uintptr) {
//		C.start_work((*C.JNIEnv)(env), C.jobject(context))
//	})
//
// env and context, and every local reference f makes, are good until f
// returns and on its goroutine alone, which keeps to the thread until
// then. JNI's FindClass in f finds the program's own classes, as well as
// Android's, on any goroutine. A Java exception f leaves pending is
// cleared, and comes back as Env's error. f must return: a panic or
// [runtime.Goexit] may not leave it.
//
// The thread is not the UI thread, which Android wants some calls on.
// Env may be called from several goroutines at once, and from within f.
func Env(f func(env unsafe.Pointer, context uintptr)) error {
	h := cgo.NewHandle(f)
	defer h.Delete()
	var n C.int
	var thrown *C.uint16_t
	if C.gunim_env(C.uintptr_t(h), &thrown, &n) == 0 {
		return errNoJava
	}
	if thrown == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(thrown))
	return fmt.Errorf("android: %s", goString(thrown, n))
}

//export goEnv
func goEnv(env unsafe.Pointer, context, h C.uintptr_t) {
	f, ok := cgo.Handle(h).Value().(func(unsafe.Pointer, uintptr))
	if !ok {
		return
	}
	f(env, uintptr(context))
}
