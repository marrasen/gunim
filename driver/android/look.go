//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"sync"

	"github.com/marrasen/gunim/driver"
)

// lookKicks nudges each that watches the system's look, by a number of its
// own.
var lookKicks struct {
	mu   sync.Mutex
	next int
	to   map[int]chan struct{}
}

// SystemLook implements [driver.LookReader]: the user's font size, dark
// theme and animations. Android has no contrast theme of colours.
func (d *Driver) SystemLook() (driver.SystemLook, error) {
	var v [3]C.float
	if C.gunim_look(&v[0]) == 0 {
		return driver.SystemLook{}, driver.ErrNoSystemLook
	}
	return driver.SystemLook{TextScale: float32(v[0]), Dark: v[1] != 0, LessMotion: v[2] != 0}, nil
}

// WatchSystemLook implements [driver.LookReader].
func (d *Driver) WatchSystemLook(fn func(driver.SystemLook, error)) (func(), error) {
	kick := make(chan struct{}, 1)
	k := &lookKicks
	k.mu.Lock()
	if k.to == nil {
		k.to = map[int]chan struct{}{}
	}
	k.next++
	id := k.next
	k.to[id] = kick
	k.mu.Unlock()
	C.gunim_watch_look()
	last, err := d.SystemLook()
	done, ended := make(chan struct{}), make(chan struct{})
	forget := func() {
		k.mu.Lock()
		defer k.mu.Unlock()
		delete(k.to, id)
	}
	if err != nil {
		forget()
		return func() {}, err
	}
	go func() {
		defer close(ended)
		for {
			select {
			case <-done:
				return
			case <-kick:
			}
			l, err := d.SystemLook()
			select {
			case <-done:
				return
			default:
			}
			if err != nil || l != last {
				if err == nil {
					last = l
				}
				fn(l, err)
			}
		}
	}()
	stop := sync.OnceFunc(func() {
		forget()
		close(done)
	})
	return func() {
		stop()
		<-ended
	}, nil
}

//export goLook
func goLook() {
	lookKicks.mu.Lock()
	defer lookKicks.mu.Unlock()
	for _, c := range lookKicks.to {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
