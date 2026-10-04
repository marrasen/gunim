package desktop

import (
	"golang.org/x/sys/windows"

	"github.com/marrasen/gunim/driver"
)

// systemFolder asks Windows for the known folder, which may lie
// elsewhere than the home, as in OneDrive.
func systemFolder(f driver.UserFolder) string {
	if f != driver.FolderMusic {
		return ""
	}
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Music, 0)
	if err != nil {
		return ""
	}
	return dir
}
