//go:build linux

package desktop

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/marrasen/gunim/driver"
)

// systemFolder reads the folder from the user's user-dirs.dirs, where
// the desktop keeps the names of its folders, translated as the
// user's language has them, as Musik.
func systemFolder(f driver.UserFolder) string {
	if f != driver.FolderMusic {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	conf := os.Getenv("XDG_CONFIG_HOME")
	if conf == "" {
		conf = filepath.Join(home, ".config")
	}
	file, err := os.Open(filepath.Join(conf, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || key != "XDG_MUSIC_DIR" {
			continue
		}
		dir := strings.ReplaceAll(strings.Trim(value, `"`), "$HOME", home)
		if dir == home {
			// The desktop's way of saying there is none.
			return ""
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}
