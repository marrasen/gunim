package driver

import "errors"

// ChooseOptions describes a dialog for choosing files or folders to open.
type ChooseOptions struct {
	Title string
	// Folders chooses folders instead of files.
	Folders bool
	// Multiple lets more than one be chosen.
	Multiple bool
	// Filters narrow the files shown, the first chosen to begin with.
	// None shows every file.
	Filters []FileFilter
}

// FileFilter is one choice of files to show, such as "Logs" for
// "*.log" and "*.jsonl".
type FileFilter struct {
	Name     string
	Patterns []string
}

// A FileChooser is a [Window] that can show the system's dialog for
// choosing files or folders to open, owned by the window. ChooseFiles
// blocks until the dialog closes, and returns nothing when it was
// cancelled. It may be called from any goroutine but the main one.
type FileChooser interface {
	ChooseFiles(o ChooseOptions) ([]string, error)
}

// ErrNoChooser is returned where the platform has no file dialog gunim
// can show.
var ErrNoChooser = errors.New("driver: no file dialog on this platform")
