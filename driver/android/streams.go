//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/instream"
)

// errNotOpened is how a file the system failed to open ends.
var errNotOpened = errors.New("the system failed to open the file")

// goStreams hands over files Java opened to be read as they come, n of
// them: a file descriptor of Go's own, the id Java keeps the stream by,
// the size the other application stated and the size of a file on a disk,
// -1 for none. A code of 0 is another application opening them, for the
// extensions in opens, or any for *; any other is the chooser waiting on
// that code.
//
//export goStreams
func goStreams(code, n C.int, fds *C.int, ids, stated, held *C.longlong, names *C.uint16_t, nn C.int, types *C.uint16_t, nt C.int,
	opens *C.uint16_t, no C.int) {
	nameList := strings.Split(goString(names, nn), "\x00")
	typeList := strings.Split(goString(types, nt), "\x00")
	var exts []string
	if code == 0 {
		exts = filecopy.Exts(goString(opens, no))
	}
	var streams []*input.Stream
	for i := range int(n) {
		var name, mime string
		if i < len(nameList) {
			name = nameList[i]
		}
		if i < len(typeList) {
			mime = typeList[i]
		}
		s := stream(filecopy.SafeName(name), mime, int(at(fds, n, i)), int64(at(ids, n, i)), int64(at(stated, n, i)), int64(at(held, n, i)))
		if exts != nil && !filecopy.Matches(s.Name, exts) {
			_ = s.Close()
			continue
		}
		streams = append(streams, s)
	}
	switch {
	case code != 0:
		go func() {
			if !choseFiles(int(code), fileAnswer{streams: streams}) {
				for _, s := range streams {
					_ = s.Close()
				}
			}
		}()
	case len(streams) > 0:
		go theDriver.filesOpened(driver.StreamsOpened{Streams: streams})
	}
}

// at returns number i of the n that p points at.
func at[T any](p *T, n C.int, i int) T { return unsafe.Slice(p, int(n))[i] }

// stream returns the stream Java keeps as id, read from fd.
func stream(name, mime string, fd int, id, stated, held int64) *input.Stream {
	if fd < 0 {
		return instream.Failing(name, errNotOpened)
	}
	if held == instream.Unstated {
		// A read that waits on a pipe then takes no thread, and ends as
		// the pipe is closed. Where the sender has undone it, as it can
		// with a pipe it has kept, the stream ends without the read.
		_ = syscall.SetNonblock(fd, true)
	}
	return instream.New(name, mime, instream.Source{
		R: os.NewFile(uintptr(fd), "stream"), Stated: stated, Held: held,
		End: func(wait bool) instream.End {
			switch C.gunim_stream_end(C.longlong(id), cBool(wait)) {
			case 0:
				return instream.Done
			case 1:
				return instream.Unknown
			}
			return instream.Failed
		},
		Closed: func() { C.gunim_close_stream(C.longlong(id)) },
	})
}
