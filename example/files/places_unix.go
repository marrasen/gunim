//go:build linux || darwin || freebsd || openbsd || netbsd

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// userFolders returns the user's desktop, documents, downloads and
// pictures, from user-dirs.dirs where there is one.
func userFolders(home string) ([]userPlace, error) {
	dirs := map[string]string{
		"DESKTOP":   filepath.Join(home, "Desktop"),
		"DOCUMENTS": filepath.Join(home, "Documents"),
		"DOWNLOAD":  filepath.Join(home, "Downloads"),
		"PICTURES":  filepath.Join(home, "Pictures"),
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	if err := readUserDirs(filepath.Join(config, "user-dirs.dirs"), home, dirs); err != nil {
		return nil, err
	}
	return []userPlace{
		{"Desktop", "desktop", dirs["DESKTOP"]},
		{"Documents", "documents", dirs["DOCUMENTS"]},
		{"Downloads", "downloads", dirs["DOWNLOAD"]},
		{"Pictures", "pictures", dirs["PICTURES"]},
	}, nil
}

// readUserDirs reads the XDG_*_DIR lines of the file at path into dirs.
// A missing file changes nothing.
func readUserDirs(path, home string, dirs map[string]string) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || !strings.HasPrefix(key, "XDG_") || !strings.HasSuffix(key, "_DIR") {
			continue
		}
		value = strings.Trim(value, `"`)
		value = strings.Replace(value, "$HOME", home, 1)
		dirs[strings.TrimSuffix(strings.TrimPrefix(key, "XDG_"), "_DIR")] = value
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	return nil
}

// volumes returns the root and what is mounted under /media and
// /run/media for the user.
func volumes() ([]Place, error) {
	out := []Place{{Name: "Computer", Path: "/", Kind: "drive"}}
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("finding who you are: %w", err)
	}
	for _, base := range []string{"/media/" + u.Username, "/run/media/" + u.Username} {
		es, err := os.ReadDir(base)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", base, err)
		}
		for _, e := range es {
			if e.IsDir() {
				out = append(out, Place{Name: e.Name(), Path: filepath.Join(base, e.Name()), Kind: "drive"})
			}
		}
	}
	return out, nil
}

// volumeSpace returns the room on the volume that holds path.
func volumeSpace(path string) (space, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return space{}, fmt.Errorf("reading the free space of %s: %w", path, err)
	}
	bs := uint64(st.Bsize) //nolint:unconvert // Bsize differs in type between systems.
	return space{free: st.Bavail * bs, total: st.Blocks * bs}, nil
}
