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
	"runtime"
	"strings"
	"sync"

	"github.com/marrasen/gunim/driver"
)

// mark is in the name of a file on the device's own disk that is not whole.
const mark = ".gunim-part-"

// part returns a name for a file while it is not whole. It is short, as
// the file's own may be as long as a name can be.
func part() string { return fmt.Sprintf("%s%016x", mark, rand.Uint64()) }

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
	gone := false
	return Target{
		Open: func() (io.Writer, error) {
			file, err := fi.create(part)
			if err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			if gone {
				// Remove has come meanwhile, and found nothing.
				return nil, errors.Join(errEnded, file.Close(), fi.remove(part))
			}
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
			return err
		},
		Commit: func() error {
			if check != nil {
				if err := check(); err != nil {
					return err
				}
			}
			return fi.rename(part, name)
		},
		Abandon: func() {
			mu.Lock()
			defer mu.Unlock()
			gone = true
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
// whole, and the new file then has its permissions. A link at p stays, and
// the file it leads to is written.
func Save(p string) driver.SavedFile {
	name := filepath.Base(p)
	p, lost := follow(p)
	perm, made := fs.FileMode(0o666), true
	if st, err := os.Stat(p); err == nil {
		perm, made = st.Mode().Perm(), false
	}
	place := filepath.Join(filepath.Dir(p), part())
	fi := files{
		create: func(name string) (*os.File, error) {
			if lost != nil {
				return nil, lost
			}
			if made {
				return os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
			}
			f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return nil, err
			}
			if err := f.Chmod(perm); err != nil && runtime.GOOS != "windows" {
				return nil, errors.Join(err, f.Close(), os.Remove(name))
			}
			return f, nil
		},
		rename: os.Rename, remove: os.Remove,
	}
	return &disk{Writer: New(fi.target(place, p, nil)), name: name, place: place, made: made}
}

// follow returns where the links at p lead, to a file or to where one may
// come.
func follow(p string) (string, error) {
	for range 40 {
		to, err := os.Readlink(p)
		if err != nil {
			return p, nil //nolint:nilerr // It is no link.
		}
		if !filepath.IsAbs(to) {
			// The folder may be a link too, which ".." does not go back by.
			dir, err := filepath.EvalSymlinks(filepath.Dir(p))
			if err != nil {
				return p, err
			}
			to = filepath.Join(dir, to)
		}
		p = to
	}
	return p, &fs.PathError{Op: "open", Path: p, Err: errors.New("too many links")}
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
	place := filepath.FromSlash(path.Join(path.Dir(p), part()))
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
	commit, remove := t.Commit, t.Remove
	t.Commit = func() error {
		err := commit()
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
