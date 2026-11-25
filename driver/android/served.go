//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/internal/instream"
)

// servedFile is a file served, with how many times it is open, and whether
// it has stopped.
type servedFile struct {
	*driver.Served
	open    int
	stopped bool
}

// The files served, by the id Java knows them by, and their ids. One that
// has stopped is there while it is open, for what has it open to hear.
var (
	serving   sync.Mutex
	servedID  int64
	served    = map[int64]*servedFile{}
	servedIDs = map[*driver.Served]int64{}
)

// serve returns the id of s, which is served until it stops.
func serve(s *driver.Served) int64 {
	serving.Lock()
	defer serving.Unlock()
	if id, ok := servedIDs[s]; ok {
		return id
	}
	servedID++
	id := servedID
	f := &servedFile{Served: s}
	served[id], servedIDs[s] = f, id
	go func() {
		<-s.Stopped()
		C.gunim_stop_served(C.longlong(id))
		serving.Lock()
		defer serving.Unlock()
		f.stopped = true
		delete(servedIDs, s)
		if f.open == 0 {
			delete(served, id)
		}
	}()
	return id
}

// servedName returns s's name as one that another application takes, and
// servedType its type, or "".
func servedName(s *driver.Served) string { return filecopy.SafeName(s.Name) }
func servedType(s *driver.Served) string { return instream.Type(s.Type) }

// OpenServed implements [driver.Opener].
func (w *Window) OpenServed(s *driver.Served) error {
	n, np := utf16Of(servedName(s))
	t, tp := utf16Of(servedType(s))
	r := C.gunim_open_served(C.longlong(serve(s)), np, C.int(len(n)), tp, C.int(len(t)), C.longlong(s.Size))
	runtime.KeepAlive(n)
	runtime.KeepAlive(t)
	switch r {
	case 0:
		return nil
	case 1:
		return driver.ErrNoneOpens
	}
	return driver.ErrNoOpener
}

//export goServedUse
func goServedUse(id C.longlong, by C.int) {
	serving.Lock()
	f := served[int64(id)]
	if f != nil {
		f.open += int(by)
		if f.open == 0 && f.stopped {
			delete(served, int64(id))
		}
	}
	serving.Unlock()
	if f != nil {
		f.Use(int(by))
	}
}

//export goServedRead
func goServedRead(id, offset C.longlong, buf unsafe.Pointer, n C.int) C.int {
	serving.Lock()
	s := served[int64(id)]
	serving.Unlock()
	if s == nil || n < 0 {
		return -1
	}
	got, err := s.ReadAt(unsafe.Slice((*byte)(buf), int(n)), int64(offset))
	if err != nil && !errors.Is(err, io.EOF) {
		return -1
	}
	return C.int(got)
}

// fields splits what Java says at its NULs.
func fields(s string) []string { return strings.Split(s, "\x00") }
