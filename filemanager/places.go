package filemanager

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// space is how much room a volume has, in bytes.
type space struct {
	free, total uint64
}

// userPlace is one of the user's own folders, found by the system.
type userPlace struct {
	name, kind, path string
}

// LocalPlaces returns the places of the computer's own file system: the
// user's home and the folders in it the system knows, and the volumes with
// their space. A volume whose space cannot be read carries the error.
// It is what a window on the computer's own file system offers when its
// options give no places, and a start for a caller that adds its own.
func LocalPlaces() ([]Place, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("finding your home folder: %w", err)
	}
	folders, err := userFolders(home)
	if err != nil {
		return nil, err
	}
	out := []Place{{Name: "Home", Path: home, Kind: "home"}}
	for _, f := range folders {
		_, serr := os.Stat(f.path)
		if errors.Is(serr, fs.ErrNotExist) {
			continue
		}
		p := Place{Name: f.name, Path: f.path, Kind: f.kind}
		if serr != nil {
			p.Err = serr.Error()
		}
		out = append(out, p)
	}
	vols, err := volumes()
	if err != nil {
		return nil, err
	}
	for _, v := range vols {
		s, err := volumeSpace(v.Path)
		if err != nil {
			// The row names the volume, so the cause alone is said.
			v.Err = rootCause(err).Error()
		} else {
			v.Free, v.Total = s.free, s.total
		}
		out = append(out, v)
	}
	return out, nil
}

// rootCause returns the innermost error err wraps.
func rootCause(err error) error {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return err
		}
		err = inner
	}
}
