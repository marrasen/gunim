//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/marrasen/gunim/driver"
)

// What Java answers a prompt with, as GunimConfirm has it.
const (
	confirmYes = iota
	confirmNo
	confirmNoLock
	confirmFailed
	confirmRefused
	// confirmFailedWith and up are confirmFailed, with Android's code.
	confirmFailedWith = 1000
)

// errConfirm is returned where Android shows no prompt, and
// errConfirmPermission where the manifest has no permission for one.
var (
	errConfirm           = errors.New("android: the prompt to confirm could not be shown")
	errConfirmPermission = errors.New("android: no permission for the prompt to confirm; gunimapk's -permissions confirm gives it")
)

// confirming is the prompt that is up, by a number of its own, so that an
// answer to one before it is none to it.
var confirming struct {
	mu     sync.Mutex
	run    int64
	answer chan int
}

// HasScreenLock implements [driver.Confirmer].
func (d *Driver) HasScreenLock() bool { return C.gunim_has_screen_lock() != 0 }

// ConfirmPerson implements [driver.Confirmer], with Android's own prompt.
// Before Android 11 it is the screen of the lock's own, which takes the
// PIN, the pattern or the password, and a finger where Android lets it
// there. It stays as the user leaves the program and comes back.
func (d *Driver) ConfirmPerson(ctx context.Context, o driver.ConfirmOptions) error {
	if err := driver.CheckConfirm(o); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c := &confirming
	c.mu.Lock()
	if c.answer != nil {
		c.mu.Unlock()
		return driver.ErrConfirming
	}
	c.run++
	run, answer := c.run, make(chan int, 1)
	c.answer = answer
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.answer = nil
	}()
	t, tp := utf16Of(o.Title)
	x, xp := utf16Of(o.Text)
	C.gunim_confirm(C.longlong(run), tp, C.int(len(t)), xp, C.int(len(x)), cBool(o.Strong))
	runtime.KeepAlive(t)
	runtime.KeepAlive(x)
	select {
	case a := <-answer:
		return confirmError(a)
	case <-ctx.Done():
		C.gunim_cancel_confirm(C.longlong(run))
		return ctx.Err()
	}
}

// confirmError is what an answer of Java's comes to: nil for the one that
// confirms, and an error for any other.
func confirmError(a int) error {
	switch a {
	case confirmYes:
		return nil
	case confirmNo:
		return driver.ErrNotConfirmed
	case confirmNoLock:
		return driver.ErrNoScreenLock
	case confirmRefused:
		return errConfirmPermission
	}
	if a >= confirmFailedWith {
		return fmt.Errorf("%w: Android's error %d", errConfirm, a-confirmFailedWith)
	}
	return errConfirm
}

//export goConfirmed
func goConfirmed(run C.longlong, a C.int) {
	c := &confirming
	c.mu.Lock()
	defer c.mu.Unlock()
	if int64(run) != c.run || c.answer == nil {
		return
	}
	select {
	case c.answer <- int(a):
	default:
		// The first answer counts.
	}
}
