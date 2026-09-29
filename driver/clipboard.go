package driver

// ImageClipboard is a window that reads pictures from the system clipboard, as well as text.
type ImageClipboard interface {
	// ClipboardImage returns the picture on the clipboard as PNG, or nil when it holds none. It may wait on the
	// display server.
	ClipboardImage() ([]byte, error)
}
