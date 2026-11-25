package driver

import (
	"errors"
	"io"
	"sync"
)

// Served is a file that other applications read from this one while it
// runs, with no copy on the disk. Each read another application makes
// calls Data, for the bytes it asks for and no more than the system reads
// ahead, so a player that seeks has only what it plays read.
//
// It lasts until Stop, or until the process ends: another application may
// open it again for as long as the user lets it, as to play a song again.
// Its fields are set before it is handed over, and stay.
type Served struct {
	// Name is the file's name, as the other application sees it, and Type
	// its MIME type, or "" for the one that goes with the name's
	// extension.
	Name, Type string
	// Size is how many bytes Data has.
	Size int64
	// Data has the file's bytes. It is read from several goroutines at
	// once. The other application waits for each read, for as long as it
	// takes, and hears of an error as one of the disk's. Data that ends
	// before Size is an error too, and never a shorter file.
	Data io.ReaderAt
	// OnUse, where it is set, is told how many times the file is open in
	// other applications each time that changes, 0 for none. It is called
	// on another goroutine, and returns soon. Another application that
	// shows may read on with this one out of sight, so a program with
	// work that a read needs, as a connection, keeps it while the file is
	// open.
	OnUse func(open int)

	mu      sync.Mutex
	stopped chan struct{}
	open    int
}

// ErrServedStopped is returned for a read of a [Served] that has stopped.
var ErrServedStopped = errors.New("driver: the file is no longer served")

// stopping returns the channel that Stop closes.
func (s *Served) stopping() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped == nil {
		s.stopped = make(chan struct{})
	}
	return s.stopped
}

// Stop ends the serving: no application opens the file from here on, and
// one that has it open fails as it reads, a read that waits included. It
// takes back nothing that was read: the other application may have kept
// it, and so may the system, for it to read again.
func (s *Served) Stop() {
	ch := s.stopping()
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// Stopped is closed by Stop, for a driver.
func (s *Served) Stopped() <-chan struct{} { return s.stopping() }

// Use tells of the file opened, by 1, or closed, by -1, for a driver.
func (s *Served) Use(by int) {
	s.mu.Lock()
	s.open = max(0, s.open+by)
	n, tell := s.open, s.OnUse
	s.mu.Unlock()
	if tell != nil {
		tell(n)
	}
}

// ReadAt reads p whole from off, or to the file's end, for a driver. It
// returns io.EOF where p reaches past the end, as [io.ReaderAt] does, and
// any other error with no bytes. It returns as the serving stops, where
// Data has yet to.
func (s *Served) ReadAt(p []byte, off int64) (int, error) {
	stopped := s.stopping()
	select {
	case <-stopped:
		return 0, ErrServedStopped
	default:
	}
	if off < 0 {
		return 0, errors.New("driver: a read before the file")
	}
	if off >= s.Size {
		return 0, io.EOF
	}
	want := p
	if left := s.Size - off; int64(len(p)) > left {
		want = p[:left]
	}
	if len(want) == 0 {
		return 0, nil
	}
	// Data reads into bytes of its own, as it may go on past a stop.
	type got struct {
		b   []byte
		err error
	}
	ch := make(chan got, 1)
	go func() {
		b := make([]byte, len(want))
		n, err := s.Data.ReadAt(b, off)
		if n == len(b) {
			err = nil
		} else if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		ch <- got{b, err}
	}()
	select {
	case g := <-ch:
		if g.err != nil {
			return 0, g.err
		}
		copy(want, g.b)
	case <-stopped:
		return 0, ErrServedStopped
	}
	if len(want) < len(p) {
		return len(want), io.EOF
	}
	return len(want), nil
}

// Check returns an error for a file that can't be served.
func (s *Served) Check() error {
	if s == nil || s.Size < 0 || s.Data == nil {
		return errors.New("driver: a served file needs its size and its data")
	}
	return nil
}

// An Opener is a [Window] that hands a file this program serves to another
// application, for the user to look at or to play. OpenServed returns once
// the system has taken it. It may be called from any goroutine but the main
// one.
type Opener interface {
	OpenServed(s *Served) error
}

// ErrNoOpener is returned where gunim can hand no file that it serves to
// another application.
var ErrNoOpener = errors.New("driver: no way to serve a file to other programs on this platform")

// ErrNoneOpens is returned where no application opens a file of its type.
var ErrNoneOpens = errors.New("driver: no program here opens a file of this type")
