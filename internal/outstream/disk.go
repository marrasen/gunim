package outstream

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/marrasen/gunim/driver"
)

// mark is in the name of a file on the device's own disk that is not whole.
const mark = ".gunim-part-"

// part returns a name for base while it is not whole.
func part(base string) string { return fmt.Sprintf(".%s%s%08x", base, mark, rand.Uint32()) }

// disk is a file on the device's own disk, as it is written.
type disk struct {
	*Writer
	name, place string
	made        bool
}

// Name implements [driver.SavedFile], as Place and Made do.
func (d *disk) Name() string { return d.name }

// Place implements [driver.SavedFile].
func (d *disk) Place() string { return d.place }

// Made implements [driver.SavedFile].
func (d *disk) Made() bool { return d.made }

// files is the disk as a Target needs it, by a whole path or in a folder.
type files struct {
	create func(name string) (*os.File, error)
	rename func(from, to string) error
	remove func(name string) error
}

// target writes part, which is renamed to name once it is whole, where
// check, if any, has nothing against it by then.
func (fi files) target(part, name string, check func() error) Target {
	var mu sync.Mutex
	var f *os.File
	return Target{
		Open: func() (io.Writer, error) {
			file, err := fi.create(part)
			if err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			f = file
			return file, nil
		},
		Finish: func(n int64) error {
			err := f.Sync()
			if st, serr := f.Stat(); err == nil {
				err = serr
				if err == nil && st.Size() != n {
					err = driver.ErrNotSaved
				}
			}
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err == nil && check != nil {
				err = check()
			}
			if err != nil {
				return err
			}
			return fi.rename(part, name)
		},
		Abandon: func() {
			mu.Lock()
			defer mu.Unlock()
			if f != nil {
				_ = f.Close()
			}
		},
		Remove: func() error {
			if err := fi.remove(part); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		},
	}
}

// Save returns the file at p, to write. What is there stays until it is
// whole.
func Save(p string) driver.SavedFile {
	_, err := os.Lstat(p)
	place := filepath.Join(filepath.Dir(p), part(filepath.Base(p)))
	fi := files{
		create: func(name string) (*os.File, error) {
			return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		},
		rename: os.Rename, remove: os.Remove,
	}
	return &disk{Writer: New(fi.target(place, p, nil)), name: filepath.Base(p), place: place, made: errors.Is(err, fs.ErrNotExist)}
}

// RemoveSaved takes away what [driver.SavedFile.Place] names, from an
// earlier run, and no file of another kind.
func RemoveSaved(place string) error {
	if !filepath.IsAbs(place) || !strings.Contains(filepath.Base(place), mark) {
		return driver.ErrNotMade
	}
	if err := os.Remove(place); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// folder is a folder on the device's own disk.
type folder struct {
	root *os.Root
	dir  string

	mu sync.Mutex
	// open counts the files that have not ended, and closed says Close has
	// come: the folder goes with the last of them.
	open   int
	closed bool
}

// use counts a file that begins, by 1, or ends, by -1, or Close, by 0.
func (fo *folder) use(by int) {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	fo.open += by
	fo.closed = fo.closed || by == 0
	if fo.closed && fo.open == 0 {
		_ = fo.root.Close()
	}
}

// Folder returns the folder at dir, to write in.
func Folder(dir string) (driver.SavedFolder, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &folder{root: r, dir: dir}, nil
}

// CheckPath returns an error for a path that names no file in a folder.
func CheckPath(op, p string) error {
	if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, "\x00\\") {
		return &fs.PathError{Op: op, Path: p, Err: fs.ErrInvalid}
	}
	return nil
}

// Create implements [driver.SavedFolder].
func (fo *folder) Create(p string) (driver.SavedFile, error) {
	if err := CheckPath("create", p); err != nil {
		return nil, err
	}
	// There is a file of the name, or a folder, or a link.
	check := func() error {
		if _, err := fo.root.Lstat(filepath.FromSlash(p)); err == nil {
			return &fs.PathError{Op: "create", Path: p, Err: fs.ErrExist}
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	if err := fo.root.MkdirAll(filepath.FromSlash(path.Dir(p)), 0o777); err != nil {
		return nil, err
	}
	place := filepath.FromSlash(path.Join(path.Dir(p), part(path.Base(p))))
	fi := files{
		create: func(name string) (*os.File, error) {
			return fo.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		},
		rename: fo.root.Rename, remove: fo.root.Remove,
	}
	fo.use(1)
	var once sync.Once
	ended := func() { once.Do(func() { fo.use(-1) }) }
	t := fi.target(place, filepath.FromSlash(p), check)
	finish, remove := t.Finish, t.Remove
	t.Finish = func(n int64) error {
		err := finish(n)
		if err == nil {
			ended()
		}
		return err
	}
	t.Remove = func() error {
		err := remove()
		ended()
		return err
	}
	return &disk{Writer: New(t), name: path.Base(p), place: filepath.Join(fo.dir, place), made: true}, nil
}

// Mkdir implements [driver.SavedFolder].
func (fo *folder) Mkdir(p string) error {
	if err := CheckPath("mkdir", p); err != nil {
		return err
	}
	return fo.root.MkdirAll(filepath.FromSlash(p), 0o777)
}

// Close implements [driver.SavedFolder].
func (fo *folder) Close() error {
	fo.use(0)
	return nil
}
