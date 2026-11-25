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
	// End asks, once, how the sender ended, after R has. With wait it may
	// wait a while for the sender's word, where nothing else tells.
	End func(wait bool) End
	// Closed is told as the file is let go, once.
	Closed func()
	// Local says the file is the device's own, quick to close, and that
	// closing the stream waits for it. Closing another's may take as long
	// as the other application likes, and no one waits.
	Local bool
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
	return input.NewStream(name, Type(mime), s.size(), &reader{
		s: s, ask: make(chan struct{}), got: make(chan chunk, 1), closed: make(chan struct{}),
	})
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

// chunkSize is how much is read at a time.
const chunkSize = 64 << 10

// chunk is what one read of a Source's R gave, and how the reading ended,
// where it has.
type chunk struct {
	b   []byte
	end error
}

// reader is a Source read to its end. A goroutine of its own reads R, as a
// read of another application's file may wait for as long as that
// application likes, and Close ends no such read: it ends the waiting for
// it.
type reader struct {
	s Source
	// ask has the goroutine read once more, and got brings what it read,
	// which is good until the next ask. start starts it.
	ask   chan struct{}
	got   chan chunk
	start sync.Once

	// left is what Read has yet to give of the last chunk, and end how
	// the reading ended. One goroutine reads at a time.
	left []byte
	end  error

	once   sync.Once
	closed chan struct{}
}

// Read implements [io.Reader].
func (r *reader) Read(p []byte) (int, error) {
	if r.isClosed() {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.left) == 0 && r.end == nil {
		r.start.Do(func() { go r.run() })
		select {
		case r.ask <- struct{}{}:
		case <-r.closed:
			return 0, io.ErrClosedPipe
		}
		select {
		case c := <-r.got:
			r.left, r.end = c.b, c.end
		case <-r.closed:
			return 0, io.ErrClosedPipe
		}
	}
	n := copy(p, r.left)
	r.left = r.left[n:]
	if len(r.left) == 0 {
		return n, r.end
	}
	return n, nil
}

// run reads a chunk for each ask, until the end or until the stream is
// closed.
func (r *reader) run() {
	buf := make([]byte, chunkSize)
	total := int64(0)
	for {
		select {
		case <-r.ask:
		case <-r.closed:
			return
		}
		c := chunk{}
		for len(c.b) == 0 && c.end == nil {
			n, err := r.s.R.Read(buf)
			total += int64(n)
			c.b = buf[:n]
			switch {
			case r.over(total):
				c.end = input.ErrNotWhole
			case errors.Is(err, io.EOF):
				c.end = r.ended(total)
			case err != nil:
				// The system's own words may name the file.
				c.end = input.ErrNotWhole
			}
		}
		r.got <- c
		if c.end != nil {
			return
		}
	}
}

// over reports whether the n bytes that have come are more than a size
// said.
func (r *reader) over(n int64) bool {
	return r.s.Held != Unstated && n > r.s.Held || r.s.Stated != Unstated && n > r.s.Stated
}

// ended tells how n bytes ended: io.EOF for a file that is whole.
func (r *reader) ended(n int64) error {
	known := false
	for _, size := range []int64{r.s.Held, r.s.Stated} {
		switch size {
		case Unstated:
		case n:
			known = true
		default:
			return input.ErrNotWhole
		}
	}
	switch end := r.s.End(!known); {
	case end == Failed:
		return input.ErrNotWhole
	case end == Unknown && !known:
		return input.ErrMayNotBeWhole
	}
	return io.EOF
}

func (r *reader) isClosed() bool {
	select {
	case <-r.closed:
		return true
	default:
		return false
	}
}

// Close implements [io.Closer].
func (r *reader) Close() error {
	var err error
	r.once.Do(func() {
		close(r.closed)
		done := make(chan error, 1)
		go func() {
			err := r.s.R.Close()
			r.s.Closed()
			done <- err
		}()
		if r.s.Local {
			err = <-done
		}
	})
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
