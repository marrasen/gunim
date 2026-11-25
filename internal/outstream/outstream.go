// Package outstream writes a file that may be kept by another application,
// and that ends as whole or not at all. It holds no system's calls, so its
// tests run anywhere.
package outstream

import (
	"errors"
	"io"
	"sync"

	"github.com/marrasen/gunim/driver"
)

// A Target is where a file is written.
type Target struct {
	// Open opens the file, once, as the first bytes come or as it is
	// closed with none.
	Open func() (io.Writer, error)
	// Finish is called once all of n bytes are written: it makes sure that
	// they are kept. It may return [driver.ErrMayNotBeKept], with which
	// the file is committed all the same. Remove ends the waiting for it.
	Finish func(n int64) error
	// Commit then makes the file what it is to be, as by its name. Remove
	// does nothing meanwhile.
	Commit func() error
	// Abandon lets go of the file, whatever it waits for, and Remove takes
	// away what is there. Each is called once at most, in this order, and
	// may come while Open runs: what that opens goes too.
	Abandon func()
	Remove  func() error
}

// chunkSize is how much is handed over at a time.
const chunkSize = 256 << 10

// errEnded is returned for a file that Close or Remove has ended.
var errEnded = errors.New("the file has ended")

// errWhole is returned by Remove for a file that is whole.
var errWhole = errors.New("the file is whole")

// A Writer writes to a Target, on a goroutine of its own: a write to
// another application's file may wait for as long as that likes, and
// Remove ends the waiting for it.
type Writer struct {
	t Target

	// mu is held by Write and Close, one at a time.
	mu    sync.Mutex
	buf   []byte
	n     int64
	err   error
	whole bool

	open  sync.Once
	w     io.Writer
	ask   chan func() error
	done  chan error
	start sync.Once
	gone  chan struct{}
	// idle is closed once there is no more to write, and Remove may come.
	idle   chan struct{}
	end    sync.Mutex
	closed bool
	// naming says that Commit runs.
	naming bool
	remove error
}

// New returns a writer to t.
func New(t Target) *Writer {
	return &Writer{t: t, ask: make(chan func() error), done: make(chan error, 1), gone: make(chan struct{}), idle: make(chan struct{})}
}

// run has do run on the writer's goroutine, and waits for it, or for
// Remove.
func (w *Writer) run(do func() error) error {
	select {
	case <-w.gone:
		return errEnded
	default:
	}
	w.start.Do(func() {
		go func() {
			for {
				select {
				case do := <-w.ask:
					w.done <- do()
				case <-w.gone:
					return
				case <-w.idle:
					return
				}
			}
		}()
	})
	select {
	case w.ask <- do:
	case <-w.gone:
		return errEnded
	}
	select {
	case err := <-w.done:
		return err
	case <-w.gone:
		return errEnded
	}
}

// opened opens the target the first time, on the writer's goroutine.
func (w *Writer) opened() error {
	var err error
	w.open.Do(func() { w.w, err = w.t.Open() })
	return err
}

// Write implements [io.Writer]. An error is the last: the file can't be
// whole after it.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	total := 0
	for {
		if w.whole && w.err == nil {
			w.err = errEnded
		}
		if w.err != nil {
			return total, w.err
		}
		if len(p) == 0 {
			return total, nil
		}
		// The bytes are the writer's own, as the write may go on past
		// Remove.
		if w.buf == nil {
			w.buf = make([]byte, chunkSize)
		}
		b := w.buf[:copy(w.buf, p)]
		w.err = w.run(func() error {
			if err := w.opened(); err != nil {
				return err
			}
			n, err := w.w.Write(b)
			if err == nil && n < len(b) {
				err = io.ErrShortWrite
			}
			return err
		})
		if w.err != nil {
			return total, w.err
		}
		w.n += int64(len(b))
		total += len(b)
		p = p[len(b):]
	}
}

// errNaming is returned by Remove while the file is committed.
var errNaming = errors.New("the file is being kept")

// Close says that the file is whole.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.whole {
		return nil
	}
	if w.err != nil {
		return w.err
	}
	n := w.n
	w.err = w.run(func() error {
		if err := w.opened(); err != nil {
			return err
		}
		return w.t.Finish(n)
	})
	maybe := errors.Is(w.err, driver.ErrMayNotBeKept)
	if w.err != nil && !maybe {
		return w.err
	}
	// Remove may have come as it finished. From here on it comes too late.
	w.end.Lock()
	if w.closed {
		w.end.Unlock()
		w.err = errEnded
		return w.err
	}
	w.naming = true
	w.end.Unlock()
	err := w.run(w.t.Commit)
	w.end.Lock()
	defer w.end.Unlock()
	w.naming = false
	if err != nil {
		w.err = err
		return err
	}
	if maybe {
		// Whoever calls decides whether that will do, and may leave it.
		w.buf = nil
		close(w.idle)
		return w.err
	}
	w.closed, w.whole, w.remove = true, true, errWhole
	close(w.gone)
	return nil
}

// Remove takes away a file that is not whole. It may come at any time.
func (w *Writer) Remove() error {
	w.end.Lock()
	defer w.end.Unlock()
	if w.naming {
		return errNaming
	}
	if w.closed {
		return w.remove
	}
	w.closed, w.remove = true, nil
	close(w.gone)
	w.t.Abandon()
	err := w.t.Remove()
	w.remove = err
	return err
}
