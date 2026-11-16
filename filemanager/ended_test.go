package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// refusingFS is the computer's own files, on which Chmod and Chtimes
// fail with what modeErr and timeErr say, where they are set.
type refusingFS struct {
	FS
	modeErr, timeErr error
}

func (r refusingFS) Chmod(path string, mode fs.FileMode) error {
	if r.modeErr != nil {
		return &fs.PathError{Op: "chmod", Path: path, Err: r.modeErr}
	}
	return os.Chmod(path, mode)
}

func (r refusingFS) Chtimes(path string, mod time.Time) error {
	if r.timeErr != nil {
		return &fs.PathError{Op: "chtimes", Path: path, Err: r.timeErr}
	}
	return os.Chtimes(path, time.Time{}, mod)
}

// errOpUnsupported is a server's "operation unsupported", as a program
// hands it on.
var errOpUnsupported = fmt.Errorf("the server says: %w", errors.ErrUnsupported)

// endings keeps what Options.Ended hears.
type endings struct {
	mu  sync.Mutex
	got []OpEnd
}

func (e *endings) ended(_ *Window, end OpEnd) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.got = append(e.got, end)
}

func (e *endings) all() []OpEnd {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.got)
}

// refusingHarness opens a window on a refusingFS, with modeErr and
// timeErr, whose Ended is kept in the endings returned.
func refusingHarness(t *testing.T, modeErr, timeErr error, spec ...string) (*harness, *endings, func() []Notice) {
	t.Helper()
	e := &endings{}
	h := newHarnessWith(t, func(o *Options) {
		o.FS = refusingFS{FS: LocalFS(), modeErr: modeErr, timeErr: timeErr}
		o.Ended = e.ended
	}, spec...)
	return h, e, notices(h)
}

// parts lists the part files left in dir.
func parts(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".part") {
			left = append(left, e.Name())
		}
	}
	return left
}

// warnings are the warning notices of got.
func warnings(got []Notice) []Notice {
	var w []Notice
	for _, n := range got {
		if n.Kind == "warning" {
			w = append(w, n)
		}
	}
	return w
}

// A copy to a file system that refuses to set permissions keeps each
// file, tells the program it ended with both written and each refused,
// and shows one warning with the file system's answer.
func TestACopyWherePermissionsAreRefusedKeepsTheFilesAndWarnsOnce(t *testing.T) {
	h, e, got := refusingHarness(t, errOpUnsupported, nil, "a.txt", "b.txt", "web/")
	web := filepath.Join(h.dir, "web")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt"), filepath.Join(h.dir, "b.txt")}, dest: web}, "Copying")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err != nil || end.Kind != OpCopy || end.Done != 2 || end.Failed != 0 || end.Into != web {
		t.Fatalf("the copy ended as %+v", end)
	}
	if len(end.Warnings) != 2 {
		t.Fatalf("the warnings are %+v, want one for each file", end.Warnings)
	}
	for _, w := range end.Warnings {
		if !w.Mode || w.Time || !errors.Is(w.Err, errors.ErrUnsupported) || filepath.Dir(w.Path) != web {
			t.Fatalf("a warning is %+v", w)
		}
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if got := contents(t, filepath.Join(web, name)); got != name {
			t.Fatalf("%s holds %q", name, got)
		}
	}
	if left := parts(t, web); len(left) > 0 {
		t.Fatalf("part files are left: %q", left)
	}
	h.until("the warning shows", func() bool { return len(warnings(got())) > 0 })
	h.frames(4)
	ws := warnings(got())
	if len(ws) != 1 || !strings.Contains(ws[0].Body, errOpUnsupported.Error()) {
		t.Fatalf("the warnings shown are %+v, want one with the file system's answer", ws)
	}
	if len(h.a.ops.dialogs) > 0 {
		t.Fatalf("a dialog shows: %+v", h.a.ops.dialogs[0].state)
	}
}

// A time the file system refuses with "permission denied" is a warning
// too, for a folder as for a file.
func TestARefusedTimeIsAWarning(t *testing.T) {
	h, e, _ := refusingHarness(t, nil, fs.ErrPermission, "src/a.txt", "web/")
	web := filepath.Join(h.dir, "web")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "src")}, dest: web}, "Copying src")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err != nil || end.Done != 1 {
		t.Fatalf("the copy ended as %+v", end)
	}
	paths := make([]string, 0, len(end.Warnings))
	for _, w := range end.Warnings {
		if w.Mode || !w.Time {
			t.Fatalf("a warning is %+v", w)
		}
		paths = append(paths, w.Path)
	}
	slices.Sort(paths)
	if want := []string{filepath.Join(web, "src"), filepath.Join(web, "src", "a.txt")}; !slices.Equal(paths, want) {
		t.Fatalf("the warnings are for %q, want %q", paths, want)
	}
	if got := contents(t, filepath.Join(web, "src", "a.txt")); got != "src/a.txt" {
		t.Fatalf("a.txt holds %q", got)
	}
}

// A refused permissions change of a file only its owner could read says
// others may now read the copy.
func TestARefusedPrivateFileSaysOthersMayReadIt(t *testing.T) {
	h, e, got := refusingHarness(t, errOpUnsupported, nil, "a.txt", "web/")
	if err := os.Chmod(filepath.Join(h.dir, "a.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt")}, dest: filepath.Join(h.dir, "web")}, "Copying")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	if ws := e.all()[0].Warnings; len(ws) != 1 || !ws[0].Private {
		t.Fatalf("the warnings are %+v, want the file marked private", ws)
	}
	h.until("the warning shows", func() bool { return len(warnings(got())) > 0 })
	if w := warnings(got())[0]; w.Body == errOpUnsupported.Error() {
		t.Fatalf("the warning says only %q", w.Body)
	}
}

// A Chmod that fails for another reason still fails the copy, which
// takes its part file away, and the program hears of the failure.
func TestAChmodThatFailsOtherwiseStillFailsTheCopy(t *testing.T) {
	h, e, _ := refusingHarness(t, errors.New("disk on fire"), nil, "a.txt", "web/")
	web := filepath.Join(h.dir, "web")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt")}, dest: web}, "Copying a.txt")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err == nil || end.Failed != 1 || end.Done != 0 || len(end.Warnings) != 0 {
		t.Fatalf("the copy ended as %+v", end)
	}
	if _, err := os.Stat(filepath.Join(web, "a.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the file is there: %v", err)
	}
	if left := parts(t, web); len(left) > 0 {
		t.Fatalf("part files are left: %q", left)
	}
	h.until("the failure shows", func() bool { return len(h.a.ops.dialogs) == 1 })
}

// An archive extracted where permissions are refused keeps its files,
// and warns of them.
func TestAnExtractWherePermissionsAreRefusedKeepsTheFiles(t *testing.T) {
	h, e, _ := refusingHarness(t, errOpUnsupported, nil, "keep.txt")
	writeZip(t, filepath.Join(h.dir, "x.zip"), map[string]string{"a.txt": "one", "sub/b.txt": "two"})
	h.a.post(func() {
		h.a.startOp(job{kind: OpExtract, srcs: []string{filepath.Join(h.dir, "x.zip")}, dest: h.dir, name: "x"}, "Extracting")
	})
	h.until("the extract ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err != nil || end.Into != filepath.Join(h.dir, "x") || len(end.Warnings) != 2 {
		t.Fatalf("the extract ended as %+v", end)
	}
	if got := contents(t, filepath.Join(h.dir, "x", "sub", "b.txt")); got != "two" {
		t.Fatalf("b.txt holds %q", got)
	}
}

// A copy that waits for the user to say whether to replace a file has
// not ended, and the program hears it once it has.
func TestACopyWaitingOnAQuestionHasNotEnded(t *testing.T) {
	h, e, _ := refusingHarness(t, nil, nil, "a.txt", "web/a.txt")
	web := filepath.Join(h.dir, "web")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt")}, dest: web}, "Copying a.txt")
	})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask, ok := h.a.ops.dialogs[0].state.(ClashAsk)
	if !ok {
		t.Fatalf("the dialog asks %+v", h.a.ops.dialogs[0].state)
	}
	h.frames(10)
	if got := e.all(); len(got) > 0 {
		t.Fatalf("the copy ended while it asked: %+v", got)
	}
	h.answer(ClashAnswered{Op: ask.Op, Choice: ChoiceReplace})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	if end := e.all()[0]; end.Err != nil || end.Done != 1 {
		t.Fatalf("the copy ended as %+v", end)
	}
}

// A delete tells the program how many items went, and one stopped at a
// question ends as stopped.
func TestADeleteAndAStoppedCopyTellTheProgram(t *testing.T) {
	h, e, _ := refusingHarness(t, nil, nil, "a.txt", "b.txt", "web/a.txt")
	h.a.post(func() {
		h.a.startOp(job{kind: OpDelete, srcs: []string{filepath.Join(h.dir, "b.txt")}}, "Deleting b.txt")
	})
	h.until("the delete ends", func() bool { return len(e.all()) == 1 })
	if end := e.all()[0]; end.Kind != OpDelete || end.Err != nil || end.Done != 1 {
		t.Fatalf("the delete ended as %+v", end)
	}
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt")}, dest: filepath.Join(h.dir, "web")}, "Copying a.txt")
	})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask := h.a.ops.dialogs[0].state.(ClashAsk)
	h.answer(ClashAnswered{Op: ask.Op, Stop: true})
	h.until("the copy ends", func() bool { return len(e.all()) == 2 })
	if end := e.all()[1]; !errors.Is(end.Err, context.Canceled) || end.Failed != 0 {
		t.Fatalf("the stopped copy ended as %+v", end)
	}
}
