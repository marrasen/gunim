package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
	failOpen         string
	// mkdirMode, where set, is the mode Mkdir makes every folder with,
	// as a server that ignores the mode asked for does.
	mkdirMode fs.FileMode
}

func (r refusingFS) Mkdir(path string, perm fs.FileMode) error {
	if r.mkdirMode != 0 {
		if err := r.FS.Mkdir(path, r.mkdirMode); err != nil {
			return err
		}
		return os.Chmod(path, r.mkdirMode)
	}
	return r.FS.Mkdir(path, perm)
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
		// The answer is of the user's file, not the part file it was.
		if msg := w.Err.Error(); strings.Contains(msg, ".part") || !strings.Contains(msg, w.Path) {
			t.Fatalf("the answer for %s says %q", w.Path, msg)
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
	if len(ws) != 1 || !strings.Contains(ws[0].Body, errOpUnsupported.Error()) || strings.Contains(ws[0].Body, ".part") {
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

// copyRefused gives the items named in modes their modes, and copies
// those at the top of the window's folder into its folder web, on a file
// system that refuses every Chmod and makes every folder open to all to
// read. It returns how the copy ended and the
// warnings shown.
func copyRefused(t *testing.T, modes map[string]fs.FileMode, spec ...string) (OpEnd, []Notice) {
	t.Helper()
	e := &endings{}
	h := newHarnessWith(t, func(o *Options) {
		o.FS = refusingFS{FS: LocalFS(), modeErr: errOpUnsupported, mkdirMode: 0o755}
		o.Ended = e.ended
	}, append(spec, "web/")...)
	got := notices(h)
	srcs := make([]string, 0, len(modes))
	for name, mode := range modes {
		p := filepath.Join(h.dir, name)
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(name, "/") {
			srcs = append(srcs, p)
		}
	}
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: srcs, dest: filepath.Join(h.dir, "web")}, "Copying")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	h.until("the warning shows", func() bool { return len(warnings(got())) > 0 })
	return e.all()[0], warnings(got())
}

// An item others could not read, whose permissions the file system
// refused, is marked private, and its warning says more than for one
// others could read. A folder is marked so too.
func TestARefusedItemOthersCouldNotReadIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no permissions for others")
	}
	// What a warning says, but for the path, which is in a folder of
	// each test's own.
	said := func(end OpEnd, n []Notice) string { return strings.ReplaceAll(n[0].Body, end.Warnings[0].Path, "") }
	end, n := copyRefused(t, map[string]fs.FileMode{"a.txt": 0o644}, "a.txt")
	if ws := end.Warnings; len(ws) != 1 || ws[0].Private {
		t.Fatalf("the warnings are %+v, want a.txt not marked private", ws)
	}
	open := said(end, n)
	end, n = copyRefused(t, map[string]fs.FileMode{"a.txt": 0o640}, "a.txt")
	if ws := end.Warnings; len(ws) != 1 || !ws[0].Private {
		t.Fatalf("the warnings are %+v, want a.txt marked private", ws)
	}
	if closed := said(end, n); !strings.HasPrefix(closed, open) || closed == open {
		t.Fatalf("the warning of a file others could not read says %q, and of one they could %q", closed, open)
	}
	end, _ = copyRefused(t, map[string]fs.FileMode{"src": 0o750, "src/a.txt": 0o644}, "src/a.txt")
	for _, w := range end.Warnings {
		if want := filepath.Base(w.Path) == "src"; w.Private != want {
			t.Fatalf("the warning of %s is %+v", w.Path, w)
		}
	}
	if len(end.Warnings) != 2 {
		t.Fatalf("the warnings are %+v, want the folder's and the file's", end.Warnings)
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

// refusingFS fails to open the file named failOpen, where it is set.
func (r refusingFS) Open(path string) (io.ReadSeekCloser, error) {
	if r.failOpen != "" && filepath.Base(path) == r.failOpen {
		return nil, &fs.PathError{Op: "open", Path: path, Err: errors.New("unreadable")}
	}
	return r.FS.Open(path)
}

// A move across volumes that fails takes away what it copied, and warns
// of none of it. What it moved stays where it was.
func TestAFailedMoveAcrossWarnsOfNothingItTookAway(t *testing.T) {
	e := &endings{}
	h := newHarnessWith(t, func(o *Options) {
		o.FS = refusingFS{FS: LocalFS(), modeErr: errOpUnsupported, failOpen: "b.txt"}
		o.Ended = e.ended
	}, "src/a.txt", "src/b.txt", "web/")
	h.a.post(func() {
		h.a.ops.across = true
		h.a.startOp(job{kind: OpMove, srcs: []string{filepath.Join(h.dir, "src")}, dest: filepath.Join(h.dir, "web")}, "Moving src")
	})
	h.until("the move ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err == nil || len(end.Warnings) != 0 || end.Written != 0 {
		t.Fatalf("the move ended as %+v, want a failure with no warnings or files written", end)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "web", "src")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the copy is left: %v", err)
	}
	if got := contents(t, filepath.Join(h.dir, "src", "a.txt")); got != "src/a.txt" {
		t.Fatalf("a.txt holds %q", got)
	}
}

// A window that closes while a copy waits on a question tells the
// program once, with what the copy wrote before.
func TestClosingTheWindowEndsItsOperationsOnce(t *testing.T) {
	h, e, _ := refusingHarness(t, errOpUnsupported, nil, "a.txt", "b.txt", "web/b.txt")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt"), filepath.Join(h.dir, "b.txt")},
			dest: filepath.Join(h.dir, "web")}, "Copying")
	})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	// As the serve loop does once the window's context ends.
	h.close()
	h.a.stopAll()
	deadline := time.Now().Add(untilLimit)
	for len(e.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	got := e.all()
	if len(got) != 1 {
		t.Fatalf("the program heard %d ends, want one", len(got))
	}
	if end := got[0]; !errors.Is(end.Err, ErrWindowClosed) || len(end.Warnings) != 1 ||
		filepath.Base(end.Warnings[0].Path) != "a.txt" {
		t.Fatalf("the copy ended as %+v", end)
	}
}

// A copy stopped at a question still warns of what it wrote before.
func TestAStoppedCopyWarnsOfWhatItWrote(t *testing.T) {
	h, e, got := refusingHarness(t, errOpUnsupported, nil, "a.txt", "b.txt", "web/b.txt")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt"), filepath.Join(h.dir, "b.txt")},
			dest: filepath.Join(h.dir, "web")}, "Copying")
	})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask := h.a.ops.dialogs[0].state.(ClashAsk)
	h.answer(ClashAnswered{Op: ask.Op, Stop: true})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	h.until("the warning shows", func() bool {
		for _, n := range warnings(got()) {
			if strings.Contains(n.Body, errOpUnsupported.Error()) {
				return true
			}
		}
		return false
	})
}

// An upload to a file system that refuses its permissions keeps the
// upload, and warns in place of saying it went well.
func TestAnUploadWherePermissionsAreRefusedWarns(t *testing.T) {
	u := newUploadHarnessOn(t, refusingFS{FS: bareFS{LocalFS()}, modeErr: errOpUnsupported}, false)
	u.edit("draft two")
	u.step()
	u.step()
	u.until("the change is offered", func() bool { return len(u.offers()) == 1 })
	u.answerOffer(0, false)
	u.until("the upload warns", func() bool {
		for _, n := range u.notices() {
			if n.Kind == "warning" && strings.Contains(n.Body, errOpUnsupported.Error()) && !strings.Contains(n.Body, ".part") {
				return true
			}
		}
		return false
	})
	if got := contents(t, u.remote); got != "draft two" {
		t.Fatalf("the remote holds %q", got)
	}
	if u.told("Uploaded report.txt") {
		for _, n := range u.notices() {
			if n.Kind == "success" {
				t.Fatalf("the upload also says it went well: %+v", n)
			}
		}
	}
}

// A copy of a folder that fails partway counts the files it wrote and
// left, and its warning says that many.
func TestAFailedFolderCopyCountsTheFilesItWrote(t *testing.T) {
	e := &endings{}
	h := newHarnessWith(t, func(o *Options) {
		o.FS = refusingFS{FS: LocalFS(), modeErr: errOpUnsupported, failOpen: "b.txt"}
		o.Ended = e.ended
	}, "src/a.txt", "src/b.txt", "web/")
	got := notices(h)
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "src")}, dest: filepath.Join(h.dir, "web")}, "Copying src")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	end := e.all()[0]
	if end.Err == nil || end.Done != 0 || end.Written != 1 || len(end.Warnings) != 1 {
		t.Fatalf("the copy ended as %+v, want one file written and warned of", end)
	}
	h.until("the warning shows", func() bool {
		for _, n := range warnings(got()) {
			if strings.Contains(n.Body, errOpUnsupported.Error()) {
				return strings.Contains(n.Title, "1") && !strings.Contains(n.Title, "0")
			}
		}
		return false
	})
}

// An operation that finished just as the window closed, before the
// window took its end, ended well.
func TestAnOperationFinishedAsTheWindowClosedEndedWell(t *testing.T) {
	h, e, _ := refusingHarness(t, nil, nil, "a.txt", "web/")
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "a.txt")}, dest: filepath.Join(h.dir, "web")}, "Copying a.txt")
	})
	// The copy starts, but its end is left waiting for the window.
	select {
	case fn := <-h.a.done:
		fn()
	case <-time.After(untilLimit):
		t.Fatal("the copy did not start")
	}
	deadline := time.Now().Add(untilLimit)
	for len(h.a.done) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	h.close()
	h.a.stopAll()
	for len(e.all()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := e.all()
	if len(got) != 1 || got[0].Err != nil || got[0].Done != 1 {
		t.Fatalf("the program heard %+v, want the copy ended well", got)
	}
}
