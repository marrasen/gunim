package gunim

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// sent is a stream of text that says when it is closed.
type sent struct {
	io.Reader
	closed int
}

func (s *sent) Close() error {
	s.closed++
	return nil
}

func sentStream(text string) (*input.Stream, *sent) {
	s := &sent{Reader: strings.NewReader(text)}
	return input.NewStream("a.txt", "text/plain", int64(len(text)), s), s
}

// hearsStreams records the streams it hears of, and takes them as take
// says.
type hearsStreams struct {
	take bool
	got  []*input.Stream
}

func (*hearsStreams) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (*hearsStreams) Paint(*paint.Painter, Frame, geom.Size, Children)    {}
func (h *hearsStreams) Handle(e input.Event, _ *UI) bool {
	if o, ok := e.(input.StreamsOpened); ok {
		h.got = append(h.got, o.Streams...)
		return h.take
	}
	return false
}

func TestStreamsWaitForAViewAndReachItOnceInOrder(t *testing.T) {
	w := NewOffscreen(geom.Sz(200, 200), rootOf{})
	s, held := sentStream("hello")
	w.Input(driver.StreamsOpened{Streams: []*input.Stream{s}})
	w.Input(driver.FilesOpened{Paths: []string{"/a.hej"}})
	for range 5 {
		w.Frame(time.Second / 60)
	}
	h := &hearsStreams{take: true}
	RegisterView(w, "v", func(struct{}) *hearsStreams { return h }, nil)
	if err := w.Client().Mount(Root, "v", "v", struct{}{}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		w.Frame(time.Second / 60)
	}
	if len(h.got) != 1 || h.got[0] != s {
		t.Fatalf("heard %v", h.got)
	}
	if b, err := io.ReadAll(s); err != nil || string(b) != "hello" {
		t.Errorf("read %q, %v", b, err)
	}
	w.Close()
	if held.closed != 0 {
		t.Error("the window closed a stream a node took")
	}
}

func TestAWindowClosesTheStreamsNoNodeTook(t *testing.T) {
	w := NewOffscreen(geom.Sz(200, 200), rootOf{})
	s, held := sentStream("hello")
	w.Input(driver.StreamsOpened{Streams: []*input.Stream{s}})
	w.Frame(time.Second / 60)
	if held.closed != 0 {
		t.Fatal("closed while a node may still take it")
	}
	w.Close()
	if held.closed != 1 {
		t.Errorf("closed %d times", held.closed)
	}
}

func TestChooseStreamsReadsTheFilesChosen(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 300), nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Offscreen().SetChooser(func(driver.ChooseOptions) ([]string, error) { return []string{path, filepath.Join(dir, "gone")}, nil })
	got, err := w.Client().ChooseStreams(context.Background(), driver.ChooseOptions{Multiple: true})
	if err != nil || len(got) != 2 {
		t.Fatalf("%v, %v", got, err)
	}
	if b, err := io.ReadAll(got[0]); err != nil || string(b) != "hello" || got[0].Name != "a.txt" || got[0].Size != 5 {
		t.Errorf("%q, %v, %+v", b, err, got[0])
	}
	if _, err := io.ReadAll(got[1]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file that is gone: %v", err)
	}
	for _, s := range got {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}
	if _, err := w.Client().ChooseStreams(context.Background(), driver.ChooseOptions{Folders: true}); err == nil {
		t.Error("a folder passes as a stream")
	}
}

// A file cut short while it is read is not the file chosen.
func TestAChosenFileCutShortIsNotWhole(t *testing.T) {
	w := NewOffscreen(geom.Sz(400, 300), nil)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Offscreen().SetChooser(func(driver.ChooseOptions) ([]string, error) { return []string{path}, nil })
	got, err := w.Client().ChooseStreams(context.Background(), driver.ChooseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got[0].Close() }()
	if err := os.Truncate(path, 3); err != nil {
		t.Skip(err)
	}
	if _, err := io.ReadAll(got[0]); !errors.Is(err, input.ErrNotWhole) {
		t.Errorf("%v", err)
	}
}

// streamWindow is a window whose own chooser hands over streams.
type streamWindow struct {
	*driver.OffscreenWindow
	choose func() ([]*input.Stream, error)
}

func (w streamWindow) ChooseStreams(driver.ChooseOptions) ([]*input.Stream, error) { return w.choose() }

func TestStreamsChosenOnceTheCallerHasLeftAreClosed(t *testing.T) {
	answer := make(chan struct{})
	closed := make(chan struct{})
	w := newWindow(streamWindow{driver.Offscreen(geom.Sz(100, 100)), func() ([]*input.Stream, error) {
		<-answer
		return []*input.Stream{input.NewStream("a", "", -1, closesChan{closed})}, nil
	}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.Client().ChooseStreams(ctx, driver.ChooseOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(answer)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream is still open")
	}
}

type closesChan struct{ c chan struct{} }

func (closesChan) Read([]byte) (int, error) { return 0, io.EOF }
func (c closesChan) Close() error {
	close(c.c)
	return nil
}
