package filemanager

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// offlineFile writes a file of text at name in a new folder and marks it offline, as Windows marks a file whose
// contents are elsewhere; the Recall attributes a cloud provider sets cannot be set by hand.
func offlineFile(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_OFFLINE); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAFileKeptOnlineIsPreviewedOnlyWhenAsked(t *testing.T) {
	path := offlineFile(t, "notes.txt", "hello from the cloud\n")
	e, err := statEntry(LocalFS(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Online || !rowOf(e).Online {
		t.Fatal("a file marked offline is not listed as kept online")
	}
	pv := itemPreview(context.Background(), LocalFS(), 1, path, e, false)
	if !pv.Online || pv.Text != "" {
		t.Fatalf("the preview of a file kept online says online %v and shows %q; want it left unread", pv.Online, pv.Text)
	}
	pv = itemPreview(context.Background(), LocalFS(), 2, path, e, true)
	if pv.Online || pv.Text != "hello from the cloud\n" {
		t.Fatalf("asked to download, the preview says online %v and shows %q; want the text", pv.Online, pv.Text)
	}
}

func TestAPictureKeptOnlineGetsNoThumbnailReadFromIt(t *testing.T) {
	// Not a picture at all, so a thumbnail read from it would fail
	path := offlineFile(t, "photo.png", "not a picture")
	img, _, err := makeThumb(LocalFS(), path, 128, true)
	if err != nil || img != nil {
		t.Fatalf("the thumbnail of a picture kept online is %v, %v; want none, with no reading", img, err)
	}
}

func TestAskingWindowsForACachedThumbnailDoesNotFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dot.png")
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	// A new file has no thumbnail cached, which is no failure
	if _, err := cachedShellThumb(path, 128); err != nil {
		t.Fatal(err)
	}
}
