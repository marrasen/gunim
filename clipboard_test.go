package gunim

import (
	"errors"
	"testing"
)

// ReadClipboard tells an empty clipboard from one that could not be
// read; Clipboard reads both as nothing.
func TestAClipboardThatCannotBeReadSaysSo(t *testing.T) {
	w := newTestWindow()
	if s, err := w.ui.ReadClipboard(); s != "" || err != nil {
		t.Fatalf("empty, the clipboard reads %q, %v", s, err)
	}
	w.Offscreen().SetClipboardError(errors.New("no display"))
	if _, err := w.ui.ReadClipboard(); err == nil {
		t.Fatal("a clipboard that could not be read said nothing")
	}
	if s := w.ui.Clipboard(); s != "" {
		t.Fatalf("Clipboard read %q", s)
	}
}
