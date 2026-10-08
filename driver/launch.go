package driver

import "errors"

// A Launcher is a [Window] that hands files to the rest of the desktop.
// Open opens the file or folder at path with the program the system
// keeps for it, and Reveal shows it in the system's file manager. Both
// may be called from any goroutine but the main one.
type Launcher interface {
	Open(path string) error
	Reveal(path string) error
}

// ErrNoLauncher is returned where gunim cannot hand a file to the
// system's programs.
var ErrNoLauncher = errors.New("driver: no way to open files with the system's programs on this platform")

// A LinkOpener is a [Window] that opens web pages in the system's
// browser. OpenLink opens the page at url, an http or https address, and
// may be called from any goroutine but the main one.
type LinkOpener interface {
	OpenLink(url string) error
}

// ErrNoLinkOpener is returned where gunim cannot open a web page in the
// system's browser.
var ErrNoLinkOpener = errors.New("driver: no way to open web pages in the system's browser on this platform")
