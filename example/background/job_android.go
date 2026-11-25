package main

/*
#include <jni.h>
int schedule_work(JNIEnv *env, jobject context);
void finish_work(JNIEnv *env);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"unsafe"

	"github.com/marrasen/gunim/driver/android"
)

// schedule asks Android to start the job.
func schedule() error {
	ok := false
	err := android.Env(func(env unsafe.Pointer, context uintptr) {
		ok = C.schedule_work((*C.JNIEnv)(env), C.jobject(context)) != 0
	})
	if err == nil && !ok {
		err = errors.New("android refused the job")
	}
	return err
}

// finish tells Android that the job's work is done.
func finish() error {
	return android.Env(func(env unsafe.Pointer, _ uintptr) { C.finish_work((*C.JNIEnv)(env)) })
}

//export workStarted
func workStarted(network C.jlong) { jobs <- job{on: true, network: int64(network)} }

//export workStopped
func workStopped() { jobs <- job{} }

// folder is where the count is kept. It means nothing on another phone.
func folder() (string, error) { return android.NoBackupDir() }

// from says which address the device has on network, and where a server's name leads there, for a program that
// reaches its server over the job's network alone. It sends nothing to the server.
func from(ctx context.Context, network int64) string {
	n := android.Network(network)
	addrs, err := n.LookupNetIP(ctx, "ip4", "example.org")
	if err != nil {
		return err.Error()
	}
	d := net.Dialer{Control: n.Control}
	c, err := d.DialContext(ctx, "udp", netip.AddrPortFrom(addrs[0], 9).String())
	if err != nil {
		return err.Error()
	}
	defer func() { _ = c.Close() }()
	return fmt.Sprintf("from %s to %s", c.LocalAddr(), addrs[0])
}
