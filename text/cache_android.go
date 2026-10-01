package text

import (
	"os"
	"path/filepath"
)

// fontCacheDir returns where the scan of the system's fonts keeps its
// index. On Android the font scanner needs one named; the activity
// points the user cache directory at the application's own.
func fontCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(dir, "fontscan")
}
