// Package instream reads a file another application hands over, and tells
// whether it came whole. It holds no system's calls, so its tests run
// anywhere.
package instream

import (
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/marrasen/gunim/input"
)

// Unstated is a size that was not stated.
const Unstated = -1

// An End is what the system knows of how a sender ended.
type End int

// The ways a sender ends: as it meant to, in a way the system cannot
// tell, or giving up.
const (
	Done End = iota
	Unknown
	Failed
)

// A Source is a file to read.
type Source struct {
	// R reads what the sender sends. Its end is the end of the bytes,
	// whole or not.
	R io.ReadCloser
	// Stated is the size the other application stated, 0 as good as none,
	// and Held the one the open file has, where it is one on a disk; each
	// is Unstated otherwise.
	Stated, Held int64
	// End asks, once, how the sender ended, after R has.
	End func() End
	// Closed is told as the file is let go, once.
	Closed func()
}

// size returns the size a stream says it has.
func (s *Source) size() int64 {
	if s.Stated != Unstated {
		return s.Stated
	}
	return s.Held
}

// New returns the stream of s. The type is what the other application
// stated.
func New(name, mime string, s Source) *input.Stream {
	if s.Stated <= 0 {
		// Some applications state 0 for a size they do not know.
		s.Stated = Unstated
	}
	if s.Held < 0 {
		s.Held = Unstated
	}
	return input.NewStream(name, Type(mime), s.size(), &reader{s: s})
}

// Failing returns the stream of a file that did not open.
func Failing(name string, err error) *input.Stream {
	return input.NewStream(name, "", Unstated, failing{err})
}

type failing struct{ err error }

// Read implements [io.Reader].
func (f failing) Read([]byte) (int, error) { return 0, f.err }

// Close implements [io.Closer].
func (f failing) Close() error { return nil }

// reader is a Source read to its end.
type reader struct {
	s Source
	// n is how many bytes have come, and end how the reading ended, once
	// it has. One goroutine reads at a time.
	n   int64
	end error

	mu     sync.Mutex
	closed bool
}

// Read implements [io.Reader].
func (r *reader) Read(p []byte) (int, error) {
	if r.end != nil || len(p) == 0 {
		return 0, r.end
	}
	n, err := r.s.R.Read(p)
	r.n += int64(n)
	switch {
	case r.isClosed():
		r.end = io.ErrClosedPipe
	case r.over():
		r.end = input.ErrNotWhole
	case errors.Is(err, io.EOF):
		r.end = r.ended()
	case err != nil:
		// The system's own words may name the file.
		r.end = input.ErrNotWhole
	}
	return n, r.end
}

// over reports whether more bytes have come than a size said.
func (r *reader) over() bool {
	return r.s.Held != Unstated && r.n > r.s.Held || r.s.Stated != Unstated && r.n > r.s.Stated
}

// ended tells how the bytes ended: io.EOF for a file that is whole.
func (r *reader) ended() error {
	end := r.s.End()
	if end == Failed {
		return input.ErrNotWhole
	}
	known := end == Done
	for _, size := range []int64{r.s.Held, r.s.Stated} {
		switch size {
		case Unstated:
		case r.n:
			known = true
		default:
			return input.ErrNotWhole
		}
	}
	if !known {
		return input.ErrMayNotBeWhole
	}
	return io.EOF
}

func (r *reader) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// Close implements [io.Closer].
func (r *reader) Close() error {
	r.mu.Lock()
	was := r.closed
	r.closed = true
	r.mu.Unlock()
	if was {
		return nil
	}
	err := r.s.R.Close()
	r.s.Closed()
	return err
}

// Type returns mime in lower case, without its parameters, or "" for
// what is no MIME type.
func Type(mime string) string {
	mime, _, _ = strings.Cut(strings.ToLower(mime), ";")
	mime = strings.TrimSpace(mime)
	kind, sub, ok := strings.Cut(mime, "/")
	if !ok || !token(kind) || !token(sub) || len(mime) > 255 {
		return ""
	}
	return mime
}

// token reports whether s is a token, as each half of a MIME type is.
func token(s string) bool {
	for _, c := range s {
		if c <= ' ' || c > '~' || strings.ContainsRune(`()<>@,;:\"/[]?=*`, c) {
			return false
		}
	}
	return s != ""
}
