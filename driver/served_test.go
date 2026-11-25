package driver

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"
)

type readsAt func(p []byte, off int64) (int, error)

func (f readsAt) ReadAt(p []byte, off int64) (int, error) { return f(p, off) }

func TestAServedFileReadsWhatIsAsked(t *testing.T) {
	data := []byte("hello, world")
	var asked [][2]int64
	s := &Served{Size: int64(len(data)), Data: readsAt(func(p []byte, off int64) (int, error) {
		asked = append(asked, [2]int64{off, int64(len(p))})
		return bytes.NewReader(data).ReadAt(p, off)
	})}
	for _, c := range []struct {
		off  int64
		n    int
		want string
		err  error
	}{{0, 5, "hello", nil}, {7, 5, "world", nil}, {7, 50, "world", io.EOF}, {12, 4, "", io.EOF}, {99, 4, "", io.EOF}, {3, 0, "", nil}} {
		p := make([]byte, c.n)
		n, err := s.ReadAt(p, c.off)
		if string(p[:n]) != c.want || !errors.Is(err, c.err) || (err == nil) != (c.err == nil) {
			t.Errorf("%d from %d: %q, %v", c.n, c.off, p[:n], err)
		}
	}
	// No byte past the end, and none unasked, is read from Data.
	if want := [][2]int64{{0, 5}, {7, 5}, {7, 5}}; !slices.Equal(asked, want) {
		t.Errorf("asked %v, want %v", asked, want)
	}
	if _, err := s.ReadAt(make([]byte, 1), -1); err == nil {
		t.Error("a read before the file passes")
	}
}

// What comes short is an error, and never a shorter file.
func TestAServedFileCutShortIsAnError(t *testing.T) {
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		data readsAt
		want error
	}{
		"ends early":         {func(p []byte, _ int64) (int, error) { return len(p) - 1, io.EOF }, io.ErrUnexpectedEOF},
		"ends early, silent": {func([]byte, int64) (int, error) { return 0, nil }, io.ErrUnexpectedEOF},
		"fails half way":     {func(p []byte, _ int64) (int, error) { return len(p) / 2, boom }, boom},
	} {
		s := &Served{Size: 100, Data: c.data}
		if n, err := s.ReadAt(make([]byte, 10), 0); n != 0 || !errors.Is(err, c.want) {
			t.Errorf("%s: %d, %v", name, n, err)
		}
	}
	// io.EOF with every byte is the end of the file.
	s := &Served{Size: 10, Data: readsAt(func(p []byte, _ int64) (int, error) { return len(p), io.EOF })}
	if n, err := s.ReadAt(make([]byte, 10), 0); n != 10 || err != nil {
		t.Errorf("all of it, with io.EOF: %d, %v", n, err)
	}
}

func TestStopEndsAReadThatWaits(t *testing.T) {
	free := make(chan struct{})
	defer close(free)
	began := make(chan struct{})
	s := &Served{Size: 10, Data: readsAt(func(p []byte, _ int64) (int, error) {
		close(began)
		<-free
		return len(p), nil
	})}
	got := make(chan error, 1)
	go func() {
		_, err := s.ReadAt(make([]byte, 10), 0)
		got <- err
	}()
	<-began
	s.Stop()
	s.Stop()
	select {
	case err := <-got:
		if !errors.Is(err, ErrServedStopped) {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the read still waits")
	}
	if _, err := s.ReadAt(make([]byte, 1), 0); !errors.Is(err, ErrServedStopped) {
		t.Errorf("a read after Stop: %v", err)
	}
}

func TestOnUseTellsHowManyHoldTheFile(t *testing.T) {
	var told []int
	s := &Served{OnUse: func(n int) { told = append(told, n) }}
	for _, by := range []int{1, 1, -1, -1, -1} {
		s.Use(by)
	}
	if want := []int{1, 2, 1, 0, 0}; !slices.Equal(told, want) {
		t.Errorf("told %v, want %v", told, want)
	}
	new(Served).Use(1) // With no one to tell.
}

func TestOnUseTellsInOrder(t *testing.T) {
	var last, open int
	s := &Served{OnUse: func(n int) { last, open = open, n }}
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			s.Use(1)
			s.Use(-1)
		})
	}
	wg.Wait()
	// Each is told by one step from the one before, so the last is true.
	if open != 0 || last != 1 {
		t.Errorf("told %d after %d, want 0 after 1", open, last)
	}
}

func TestAStoppedFileIsNotServedAgain(t *testing.T) {
	s := &Served{Data: bytes.NewReader(nil)}
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	if err := s.Check(); !errors.Is(err, ErrServedStopped) {
		t.Errorf("%v", err)
	}
}
