package filemanager

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// xdgTrash is a trash laid out as the freedesktop.org trash specification
// has it: the items in files, and a .trashinfo record of each in info.
type xdgTrash struct {
	dir string
}

// Trash implements [Trasher].
func (t xdgTrash) Trash(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("moving %s to the trash: %w", path, err)
	}
	if _, err := os.Lstat(abs); err != nil {
		return "", fmt.Errorf("moving %s to the trash: %w", abs, err)
	}
	files, info := filepath.Join(t.dir, "files"), filepath.Join(t.dir, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", fmt.Errorf("making the trash: %w", err)
		}
	}
	base := filepath.Base(abs)
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = numbered(base, n)
		}
		dest, ok, err := t.claim(abs, name)
		if err != nil {
			return "", err
		}
		if ok {
			return dest, nil
		}
	}
}

// claim moves abs into the trash as name, and reports false when name is
// taken there.
func (t xdgTrash) claim(abs, name string) (dest string, ok bool, err error) {
	record := filepath.Join(t.dir, "info", name+".trashinfo")
	dest = filepath.Join(t.dir, "files", name)
	// The record is made first and alone, which claims the name.
	f, err := os.OpenFile(record, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("writing the trash record for %s: %w", abs, err)
	}
	_, taken := os.Lstat(dest)
	if taken == nil {
		if err := errors.Join(f.Close(), os.Remove(record)); err != nil {
			return "", false, fmt.Errorf("writing the trash record for %s: %w", abs, err)
		}
		return "", false, nil
	}
	if !errors.Is(taken, fs.ErrNotExist) {
		return "", false, errors.Join(fmt.Errorf("looking in the trash: %w", taken), f.Close(), os.Remove(record))
	}
	body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		(&url.URL{Path: filepath.ToSlash(abs)}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	_, werr := f.WriteString(body)
	if err := errors.Join(werr, f.Close()); err != nil {
		return "", false, errors.Join(fmt.Errorf("writing the trash record for %s: %w", abs, err), os.Remove(record))
	}
	if err := os.Rename(abs, dest); err != nil {
		if isCrossDevice(err) {
			err = fmt.Errorf("%s is on another volume than the trash, and trashing across volumes is not supported yet", abs)
		} else {
			err = fmt.Errorf("moving %s to the trash: %w", abs, err)
		}
		return "", false, errors.Join(err, os.Remove(record))
	}
	return dest, true, nil
}

// Restore implements [Trasher]: it checks the item's .trashinfo record
// says it came from original, moves it to to, and removes the record.
func (t xdgTrash) Restore(original, trashed string, _ time.Time, to string) error {
	if trashed == "" {
		return errNoRestore
	}
	record := filepath.Join(t.dir, "info", filepath.Base(trashed)+".trashinfo")
	from, err := t.origin(record)
	if err != nil {
		return err
	}
	if !SystemPaths.Same(from, original) {
		return fmt.Errorf("the trash record %s says the item came from %s, not %s", record, from, original)
	}
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s exists again; move it away to restore the one in the trash", to)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("restoring %s: %w", to, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("restoring %s: %w", to, err)
	}
	if err := os.Rename(trashed, to); err != nil {
		return fmt.Errorf("restoring %s: %w", to, err)
	}
	if err := os.Remove(record); err != nil {
		return fmt.Errorf("restored %s, but removing its trash record failed: %w", to, err)
	}
	return nil
}

// origin reads where the item of the .trashinfo record at path came from.
func (t xdgTrash) origin(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the trash record: %w", err)
	}
	for line := range strings.Lines(string(b)) {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "Path=")
		if !ok {
			continue
		}
		p, err := url.PathUnescape(v)
		if err != nil {
			return "", fmt.Errorf("reading the trash record %s: %w", path, err)
		}
		p = filepath.FromSlash(p)
		if !filepath.IsAbs(p) {
			// A trash on another volume keeps paths from the volume's top.
			p = filepath.Join(filepath.Dir(t.dir), p)
		}
		return p, nil
	}
	return "", fmt.Errorf("the trash record %s says nothing of where the item came from", path)
}

// Describe implements [Trasher].
func (xdgTrash) Describe(trashed string) string {
	return "in the trash, " + describe(LocalFS(), trashed)
}

// numbered returns name with " (n)" put before its extension.
func numbered(name string, n int) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem, ext = name, ""
	}
	return fmt.Sprintf("%s (%d)%s", stem, n, ext)
}
