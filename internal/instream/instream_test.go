package instream

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/marrasen/gunim/input"
)

func source(text string, stated, held int64, end End) Source {
	return Source{R: io.NopCloser(strings.NewReader(text)), Stated: stated, Held: held,
		End: func() End { return end }, Closed: func() {}}
}

func TestAFileIsWholeOnlyWhereSomethingSaysSo(t *testing.T) {
	const u = Unstated
	for name, c := range map[string]struct {
		stated, held int64
		end          End
		want         error
	}{
		"the sender says so":                       {u, u, Done, nil},
		"the sender says so, and the size":         {5, u, Done, nil},
		"the size alone":                           {5, u, Unknown, nil},
		"the open file's size alone":               {u, 5, Unknown, nil},
		"both sizes":                               {5, 5, Unknown, nil},
		"a size of 0 for an unknown one":           {0, u, Done, nil},
		"a size of 0 with an open file's":          {0, 5, Unknown, nil},
		"nothing":                                  {u, u, Unknown, input.ErrMayNotBeWhole},
		"a size of 0 for an unknown one, and more": {0, u, Unknown, input.ErrMayNotBeWhole},
		"a size below any":                         {-7, -9, Unknown, input.ErrMayNotBeWhole},
		"a sender that gave up":                    {u, u, Failed, input.ErrNotWhole},
		"a sender that gave up at the size":        {5, u, Failed, input.ErrNotWhole},
		"fewer bytes than stated":                  {6, u, Unknown, input.ErrNotWhole},
		"fewer bytes than stated, and no failure":  {6, u, Done, input.ErrNotWhole},
		"more bytes than stated":                   {4, u, Done, input.ErrNotWhole},
		"fewer bytes than the open file had":       {5, 6, Unknown, input.ErrNotWhole},
		"more bytes than the open file had":        {u, 4, Unknown, input.ErrNotWhole},
		"sizes that differ":                        {6, 5, Unknown, input.ErrNotWhole},
	} {
		for way, wrap := range map[string]func(io.Reader) io.Reader{
			"whole":        func(r io.Reader) io.Reader { return r },
			"by the byte":  iotest.OneByteReader,
			"end and data": iotest.DataErrReader,
		} {
			s := source("hello", c.stated, c.held, c.end)
			s.R = io.NopCloser(wrap(strings.NewReader("hello")))
			st := New("a", "", s)
			got, err := io.ReadAll(st)
			if !errors.Is(err, c.want) {
				t.Errorf("%s, %s: %v, want %v", name, way, err, c.want)
			}
			if err == nil && string(got) != "hello" {
				t.Errorf("%s, %s: read %q", name, way, got)
			}
			if n, again := st.Read(make([]byte, 1)); n != 0 || c.want != nil && !errors.Is(again, c.want) || c.want == nil && !errors.Is(again, io.EOF) {
				t.Errorf("%s, %s: reading past the end gives %d, %v", name, way, n, again)
			}
		}
	}
}

func TestAnEmptyFileIsWhole(t *testing.T) {
	// A stated 0 may stand for a size not known.
	if _, err := io.ReadAll(New("a", "", source("", 0, Unstated, Unknown))); !errors.Is(err, input.ErrMayNotBeWhole) {
		t.Errorf("stated alone: %v", err)
	}
	for name, s := range map[string]Source{
		"the open file":  source("", Unstated, 0, Unknown),
		"sender says so": source("", Unstated, Unstated, Done),
	} {
		if got, err := io.ReadAll(New("a", "", s)); err != nil || len(got) != 0 {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
}

// A sender that closes its pipe with an error looks, to a plain read,
// like the end of the file.
func TestAPipeClosedWithAnErrorIsNoEndOfTheFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("half of i"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	closed := 0
	st := New("a", "", Source{R: r, Stated: Unstated, Held: Unstated, End: func() End { return Failed }, Closed: func() { closed++ }})
	var out bytes.Buffer
	if n, err := io.Copy(&out, st); !errors.Is(err, input.ErrNotWhole) {
		t.Fatalf("copied %d bytes, %v, want an error", n, err)
	}
	for range 2 {
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if closed != 1 {
		t.Errorf("let go of %d times", closed)
	}
}

// An error of the system's may name the file, and goes no further.
func TestAnErrorOfTheSystemsIsToldAsNoName(t *testing.T) {
	s := source("", 5, Unstated, Done)
	s.R = io.NopCloser(iotest.ErrReader(&os.PathError{Op: "read", Path: "/secret/name", Err: os.ErrPermission}))
	if _, err := io.ReadAll(New("a", "", s)); !errors.Is(err, input.ErrNotWhole) || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}

func TestClosingEndsAReadThatWaits(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	st := New("a", "", Source{R: r, Stated: Unstated, Held: Unstated, End: func() End { return Done }, Closed: func() {}})
	got := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(st)
		got <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-got:
		if err == nil {
			t.Error("a stream closed half way reads as whole")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the read still waits")
	}
}

func TestAStreamLetGoOfIsClosed(t *testing.T) {
	closed := make(chan struct{})
	func() {
		s := source("x", 1, Unstated, Done)
		s.Closed = func() { close(closed) }
		_, _ = New("a", "", s).Read(make([]byte, 1))
	}()
	for range 100 {
		runtime.GC()
		select {
		case <-closed:
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("it is still open")
}

func TestAFileThatDidNotOpenFailsAsItIsRead(t *testing.T) {
	boom := errors.New("boom")
	st := Failing("a", boom)
	if _, err := io.ReadAll(st); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if st.Size != Unstated || st.Close() != nil {
		t.Fatalf("%+v", st)
	}
}

func TestTheSizeIsTheOneStated(t *testing.T) {
	for _, c := range []struct{ stated, held, want int64 }{
		{Unstated, Unstated, -1}, {0, Unstated, -1}, {7, Unstated, 7}, {Unstated, 9, 9}, {0, 9, 9}, {7, 9, 7}, {-4, -4, -1},
	} {
		if got := New("a", "", source("", c.stated, c.held, Done)).Size; got != c.want {
			t.Errorf("stated %d, held %d: %d, want %d", c.stated, c.held, got, c.want)
		}
	}
}

func TestOnlyATypeIsTakenAsOne(t *testing.T) {
	for in, want := range map[string]string{
		"image/JPEG": "image/jpeg", "text/plain; charset=utf-8": "text/plain", "application/vnd.a+xml": "application/vnd.a+xml",
		"": "", "image": "", "image/": "", "/x": "", "a/b/c": "", "*/*": "", "a/b\n": "a/b", "a/\x00": "", "å/b": "",
		"a/" + strings.Repeat("b", 300): "",
	} {
		if got := Type(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
