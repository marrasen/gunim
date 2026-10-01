package filemanager

import (
	"fmt"
	"os"
	"path/filepath"
)

// systemTrash returns the user's home trash, in $XDG_DATA_HOME/Trash.
func systemTrash() (trasher, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("finding the trash: %w", err)
		}
		data = filepath.Join(home, ".local", "share")
	}
	return xdgTrash{dir: filepath.Join(data, "Trash")}, nil
}
