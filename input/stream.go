package input

import (
	"errors"
	"io"
	"runtime"
	"sync"
)

// ErrNotWhole ends a [Stream] whose file did not come whole: the
// application that sends it gave up or ended, or it sent another number
// of bytes than was stated. What was read is not the file.
var ErrNotWhole = errors.New("the file did not come whole")

// ErrMayNotBeWhole ends a [Stream] where nothing tells the end of the
// file from a sender that gave up: no size was stated, and the system
// carries no word of the end. Every byte that came has been read. A
// program that takes the file all the same says so to the user.
var ErrMayNotBeWhole = errors.New("the file may not be whole")

// A Stream is a file another application hands this one, read as it
// comes, with no copy on the disk. What it says of itself comes from the
// other application, which may say anything.
type Stream struct {
	// Name is the file's name, safe as one name in a folder.
	Name string
	// Type is its MIME type in lower case, or "" where none was stated.
	Type string
	// Size is its size as stated, or -1 where it was not.
	Size int64

	r    io.ReadCloser
	once sync.Once
	err  error
	drop runtime.Cleanup
}

// NewStream returns a stream that reads r, for a driver or a test. r's
// Read returns [io.EOF] only once the file is whole. Closing the stream
// closes r, as does letting go of it.
func NewStream(name, mime string, size int64, r io.ReadCloser) *Stream {
	s := &Stream{Name: name, Type: mime, Size: size, r: r}
	s.drop = runtime.AddCleanup(s, func(r io.ReadCloser) { _ = r.Close() }, r)
	return s
}

// Read reads the file. It returns [io.EOF] only where the file has come
// whole: at the end of one that has not, it returns [ErrNotWhole], and
// [ErrMayNotBeWhole] where nothing can tell. So [io.ReadAll] and
// [io.Copy] fail for a file cut short. A file the system failed to open
// fails at the first Read. Read waits for the sender, for as long as it
// takes, until Close.
func (s *Stream) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	runtime.KeepAlive(s)
	return n, err
}

// Close lets the sender go, wherever the reading has come, and ends a
// Read that waits. A program that takes a stream closes it.
func (s *Stream) Close() error {
	s.once.Do(func() {
		s.drop.Stop()
		s.err = s.r.Close()
	})
	return s.err
}
