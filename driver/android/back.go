//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"time"

	"github.com/marrasen/gunim/driver"
)

// SetBackLeaves implements [driver.BackKeeper].
func (w *Window) SetBackLeaves(leaves bool) {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	w.backLeaves = leaves
	w.d.keepBackLocked()
}

// LeaveApp implements [driver.BackKeeper].
func (w *Window) LeaveApp() { C.gunim_leave_app() }

// keepBackLocked tells Java whose the next Back is: the system's, where
// the window on top, of those that fill the screen, leaves it. It runs
// with mu held.
func (d *Driver) keepBackLocked() {
	leaves := false
	for _, w := range d.windows {
		if w.fills && !w.closed && !w.hidden {
			leaves = w.backLeaves
		}
	}
	if leaves != d.backLeft {
		d.backLeft = leaves
		C.gunim_back_leaves(cBool(leaves))
	}
}

// back hands Back to the window that takes the keyboard.
func (d *Driver) back() {
	if w := d.keyWindow(); w != nil {
		w.in.Push(driver.BackAsked{Time: time.Now()})
	}
}

//export goBack
func goBack() { theDriver.back() }
