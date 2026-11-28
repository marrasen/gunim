package gunim

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/instream"
)

// StreamsOf returns the files of a drop as streams to read, one for each,
// in order. In a browser they are the ones it handed over. Elsewhere they
// are the files at f.Paths, which open as they are first read, so those
// left unread hold nothing open. A folder's stream fails at its first Read
// with [input.ErrFolder], and one that is gone with the system's error. The
// caller closes each.
//
// It asks the system each file's size, which a slow disk can take a while
// over, so call it off the UI goroutine for many files.
func StreamsOf(f input.Files) []*input.Stream {
	if f.Streams != nil {
		return f.Streams
	}
	streams := make([]*input.Stream, 0, len(f.Paths))
	for _, p := range f.Paths {
		streams = append(streams, droppedStream(p))
	}
	return streams
}

// droppedStream returns the file at path as a stream that opens it as it is
// first read.
func droppedStream(path string) *input.Stream {
	name := filepath.Base(path)
	st, err := os.Stat(path)
	switch {
	case err != nil:
		return instream.Failing(name, err)
	case st.IsDir():
		return instream.Failing(name, input.ErrFolder)
	}
	held := int64(instream.Unstated)
	if st.Mode().IsRegular() {
		held = st.Size()
	}
	return instream.New(name, "", instream.Source{R: &laterFile{path: path}, Stated: instream.Unstated, Held: held,
		End: func(bool) instream.End { return instream.Unknown }, Closed: func() {}, Local: true})
}

// laterFile is a file opened at its first Read.
type laterFile struct {
	path string

	mu     sync.Mutex
	f      *os.File
	closed bool
}

// Read implements [io.Reader].
func (l *laterFile) Read(p []byte) (int, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return 0, os.ErrClosed
	}
	if l.f == nil {
		f, err := os.Open(l.path)
		if err != nil {
			l.mu.Unlock()
			return 0, err
		}
		l.f = f
	}
	f := l.f
	l.mu.Unlock()
	return f.Read(p)
}

// Close implements [io.Closer].
func (l *laterFile) Close() error {
	l.mu.Lock()
	f := l.f
	l.closed = true
	l.mu.Unlock()
	if f == nil {
		return nil
	}
	return f.Close()
}

// dropStreams returns the streams a drop of files from a browser carries,
// for gunim to close where no node takes them.
func dropStreams(ev any) []*input.Stream {
	if d, ok := ev.(input.Drop); ok {
		if f, ok := d.Data.(input.Files); ok {
			return f.Streams
		}
	}
	return nil
}
