//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/internal/outstream"
)

// GunimSave's calls, as Native.save numbers them.
const (
	saveOpen = iota
	saveFinish
	saveAbandon
	saveDelete
	saveLetGo
	saveRoot
	saveFind
	saveMkdir
	saveCreate
	saveRename
	saveForget
)

// save does one of GunimSave's calls.
func save(what int, args ...string) string {
	var p runtime.Pinner
	defer p.Unpin()
	var s [5]*C.uint16_t
	var n [5]C.int
	for i, a := range args {
		u, up := utf16Of(a)
		if up != nil {
			p.Pin(up)
		}
		s[i], n[i] = up, C.int(len(u))
	}
	p.Pin(&s[0])
	var got C.int
	r := C.gunim_save(C.int(what), &s[0], &n[0], &got)
	if r == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(r))
	return goString(r, got)
}

// savedPlace is what the user chose, "" for nothing.
type savedPlace struct {
	place, name string
	made        bool
}

// The pickers of a place waiting on the user, by their request code, from
// the same count as the rest.
var savingTo = map[int]chan savedPlace{}

// pickPlace shows a picker, and waits for the place chosen.
func pickPlace(show func(code C.int)) savedPlace {
	asking.Lock()
	askCode++
	code := askCode
	ch := make(chan savedPlace, 1)
	savingTo[code] = ch
	asking.Unlock()
	show(C.int(code))
	return <-ch
}

//export goSaved
func goSaved(code C.int, place *C.uint16_t, np C.int, name *C.uint16_t, nn C.int, made C.uchar) {
	p := savedPlace{name: goString(name, nn), made: made != 0}
	if place != nil {
		p.place = goString(place, np)
	}
	asking.Lock()
	ch := savingTo[int(code)]
	delete(savingTo, int(code))
	asking.Unlock()
	if ch != nil {
		ch <- p
	}
}

// SaveStream implements [driver.StreamSaver] with the system's picker of a
// place for a file, which takes no title, folder or filters. The file is
// there under its own name, and empty, once the user has chosen.
func (w *Window) SaveStream(o driver.SaveOptions) (driver.SavedFile, error) {
	name := filecopy.SafeName(o.Name)
	u, up := utf16Of(name)
	p := pickPlace(func(code C.int) { C.gunim_create_document(code, up, C.int(len(u)), nil, 0) })
	runtime.KeepAlive(u)
	if p.place == "" {
		return nil, nil
	}
	if p.name == "" {
		p.name = name
	}
	letGo := func(string) {}
	if p.made {
		letGo = func(place string) { save(saveLetGo, place) }
	}
	return newDocument(p.place, p.name, "", p.made, letGo), nil
}

// SaveFolder implements [driver.StreamSaver] with the system's chooser of
// folders.
func (w *Window) SaveFolder(driver.ChooseOptions) (driver.SavedFolder, error) {
	p := pickPlace(func(code C.int) { C.gunim_save_folder(code) })
	if p.place == "" {
		return nil, nil
	}
	root := save(saveRoot, p.place)
	if root == "" {
		save(saveLetGo, p.place)
		return nil, driver.ErrNotSaved
	}
	return &tree{place: p.place, dirs: map[string]string{".": root}}, nil
}

// RemoveSaved implements [driver.Remover].
func (d *Driver) RemoveSaved(place string) error {
	if !strings.HasPrefix(place, "content://") {
		return driver.ErrNotMade
	}
	if save(saveDelete, place) == "" {
		return driver.ErrNotSaved
	}
	// The right to it goes with ForgetSaved: there may be more to take away
	// by it, as in a folder.
	return nil
}

// ForgetSaved implements [driver.Remover].
func (d *Driver) ForgetSaved(but []string) { save(saveForget, strings.Join(but, "\x00")) }

// document is a file that another application keeps.
type document struct {
	*outstream.Writer
	made  bool
	letGo func(place string)

	mu          sync.Mutex
	place, name string
	f           *os.File
	id          string
	gone        bool
	// maybe says that nothing tells whether the document is kept. The
	// right to it stays then, for Remove.
	maybe bool
}

// newDocument returns the document at place, to write. Once it is whole it
// is named as, where that is set, and letGo lets go of the right to it.
func newDocument(place, name, as string, made bool, letGo func(place string)) *document {
	d := &document{place: place, name: name, made: made, letGo: letGo}
	d.Writer = outstream.New(outstream.Target{
		Open: d.open, Finish: d.finish, Commit: func() error { return d.commit(as) }, Abandon: d.abandon, Remove: d.remove,
	})
	return d
}

// Name implements [driver.SavedFile], as Place and Made do.
func (d *document) Name() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.name
}

// Place implements [driver.SavedFile].
func (d *document) Place() string {
	if !d.made {
		return ""
	}
	return d.at()
}

// at returns the document's address.
func (d *document) at() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.place
}

// Made implements [driver.SavedFile].
func (d *document) Made() bool { return d.made }

func (d *document) open() (io.Writer, error) {
	fd, id, ok := strings.Cut(save(saveOpen, d.at()), " ")
	n, err := strconv.Atoi(fd)
	if !ok || err != nil {
		return nil, driver.ErrNotSaved
	}
	if waitsForSender(n) {
		_ = syscall.SetNonblock(n, true)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.f, d.id = os.NewFile(uintptr(n), "document"), id
	if d.gone {
		// Remove has come meanwhile.
		d.closeLocked()
		return nil, driver.ErrNotSaved
	}
	return quietWriter{d.f}, nil
}

// quietWriter is a file whose errors tell of no more than a failure, or a
// full disk.
type quietWriter struct{ f *os.File }

// Write implements [io.Writer].
func (q quietWriter) Write(p []byte) (int, error) {
	n, err := q.f.Write(p)
	return n, quietError(err)
}

func quietError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		return fmt.Errorf("%w: %w", driver.ErrNotSaved, syscall.ENOSPC)
	}
	return driver.ErrNotSaved
}

// finish makes sure that the n bytes written are kept.
func (d *document) finish(n int64) error {
	d.mu.Lock()
	f, id := d.f, d.id
	d.mu.Unlock()
	// A pipe has nothing to sync, and tells of no size.
	err := f.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.ENOTSUP) {
		err = nil
	}
	var st syscall.Stat_t
	disk := false
	if c, cerr := f.SyscallConn(); err == nil && cerr == nil {
		_ = c.Control(func(fd uintptr) {
			disk = syscall.Fstat(int(fd), &st) == nil && st.Mode&syscall.S_IFMT == syscall.S_IFREG
		})
	}
	if disk && st.Size != n {
		err = driver.ErrNotSaved
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return quietError(err)
	}
	piped := "1"
	if disk {
		piped = ""
	}
	switch save(saveFinish, id, piped) {
	case "1":
		return nil
	case "?":
		d.mu.Lock()
		d.maybe = true
		d.mu.Unlock()
		return driver.ErrMayNotBeKept
	}
	return driver.ErrNotSaved
}

// commit names the document as it is to be named, and lets go of the right
// to it.
func (d *document) commit(as string) error {
	if as != "" {
		got := fields(save(saveRename, d.at(), as))
		if len(got) != 2 {
			return driver.ErrNotSaved
		}
		d.mu.Lock()
		d.place, d.name = got[0], got[1]
		d.mu.Unlock()
	}
	d.mu.Lock()
	maybe := d.maybe
	d.mu.Unlock()
	if !maybe {
		d.letGo(d.at())
	}
	return nil
}

// closeLocked lets the document go as one that is not whole.
func (d *document) closeLocked() {
	f, id := d.f, d.id
	d.f = nil
	// Either may wait for who keeps it.
	go func() {
		_ = f.Close()
		save(saveAbandon, id)
	}()
}

func (d *document) abandon() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gone = true
	if d.f != nil {
		d.closeLocked()
	}
}

func (d *document) remove() error {
	place := d.at()
	if !d.made {
		d.letGo(place)
		return driver.ErrNotMade
	}
	if save(saveDelete, place) == "" {
		return driver.ErrNotSaved
	}
	d.letGo(place)
	return nil
}

// tree is a folder that another application keeps.
type tree struct {
	place string

	mu sync.Mutex
	// dirs are the ids of the folders in it that are known, by their path.
	dirs map[string]string
	// open counts the files open in it, and closed says Close has come:
	// the right to it goes with the last of them.
	open   int
	closed bool
}

// dir returns the id of the folder at p, which it makes where there is
// none. It runs with mu held.
func (t *tree) dir(p string) (string, error) {
	if id, ok := t.dirs[p]; ok {
		return id, nil
	}
	in, err := t.dir(path.Dir(p))
	if err != nil {
		return "", err
	}
	found := save(saveFind, t.place, in, path.Base(p))
	id, isDir := strings.CutPrefix(found, "d")
	switch {
	case isDir:
	case found == "":
		if id = save(saveMkdir, t.place, in, path.Base(p)); id == "" {
			return "", &fs.PathError{Op: "mkdir", Path: p, Err: driver.ErrNotSaved}
		}
	case found == "!":
		return "", &fs.PathError{Op: "mkdir", Path: p, Err: driver.ErrNotSaved}
	default:
		return "", &fs.PathError{Op: "mkdir", Path: p, Err: fs.ErrExist}
	}
	t.dirs[p] = id
	return id, nil
}

// Create implements [driver.SavedFolder].
func (t *tree) Create(p string) (driver.SavedFile, error) {
	if err := outstream.CheckPath("create", p); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	in, err := t.dir(path.Dir(p))
	if err != nil {
		return nil, err
	}
	name := path.Base(p)
	switch save(saveFind, t.place, in, name) {
	case "":
	case "!":
		return nil, &fs.PathError{Op: "create", Path: p, Err: driver.ErrNotSaved}
	default:
		return nil, &fs.PathError{Op: "create", Path: p, Err: fs.ErrExist}
	}
	got := fields(save(saveCreate, t.place, in, name, name+".part", ""))
	if len(got) != 2 {
		return nil, &fs.PathError{Op: "create", Path: p, Err: driver.ErrNotSaved}
	}
	as := name
	if got[1] == name {
		as = ""
	}
	t.open++
	var once sync.Once
	return newDocument(got[0], got[1], as, true, func(string) { once.Do(t.letGo) }), nil
}

// Mkdir implements [driver.SavedFolder].
func (t *tree) Mkdir(p string) error {
	if err := outstream.CheckPath("mkdir", p); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.dir(p)
	return err
}

// Close implements [driver.SavedFolder].
func (t *tree) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		if t.open == 0 {
			save(saveLetGo, t.place)
		}
	}
	return nil
}

// letGo is told of a file in it that has ended.
func (t *tree) letGo() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open--
	if t.closed && t.open == 0 {
		save(saveLetGo, t.place)
	}
}

var (
	_ driver.StreamSaver = (*Window)(nil)
	_ driver.Opener      = (*Window)(nil)
	_ driver.Remover     = (*Driver)(nil)
)
