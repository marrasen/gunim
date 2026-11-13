package glfw

import (
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

// onRealClipboard skips a test that replaces the system clipboard, unless it runs in CI or GUNIM_TEST_CLIPBOARD is
// set. In CI a test that cannot run fails, so a skip never passes for a run. It returns what ends the test's GLFW.
func onRealClipboard(t *testing.T) func() {
	t.Helper()
	ci := os.Getenv("CI") != ""
	if !ci && os.Getenv("GUNIM_TEST_CLIPBOARD") == "" {
		t.Skip("replaces the clipboard; set GUNIM_TEST_CLIPBOARD=1 to run it here")
	}
	runtime.LockOSThread()
	if err := Init(); err != nil {
		runtime.UnlockOSThread()
		if ci {
			t.Fatalf("Init: %v", err)
		}
		t.Skip(err)
	}
	return func() {
		_ = Terminate()
		runtime.UnlockOSThread()
	}
}

// openWithin opens the clipboard, trying for a while, as the history service and rdpclip open it too right after a
// change.
func openWithin(t *testing.T) {
	t.Helper()
	for range 100 {
		if r, _, _ := procOpenClipboard.Call(uintptr(_glfw.platformWindow.helperWindowHandle)); r != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the clipboard stayed held by another program for a second")
}

// onClipboard reports which of the sensitive marks and the text are on the clipboard, and each mark's size.
func onClipboard(t *testing.T) (marks [4]bool, sizes [4]uintptr, values [4]uint32, text bool) {
	t.Helper()
	openWithin(t)
	defer closeClipboard()
	for i, f := range sensitiveFormats {
		id, err := registerFormat(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if r, _, _ := procIsClipboardFormatAvailable.Call(id); r == 0 {
			continue
		}
		marks[i] = true
		h, _, _ := procGetClipboardData.Call(id)
		if h == 0 {
			t.Fatalf("the mark %s is there but has no data", f.name)
		}
		sizes[i], _, _ = procGlobalSize.Call(h)
		if p, _, _ := procGlobalLock.Call(h); p != 0 {
			values[i] = *(*uint32)(unsafe.Pointer(p)) //nolint:govet // memory GlobalLock lent
			_, _, _ = procGlobalUnlock.Call(h)
		}
	}
	r, _, _ := procIsClipboardFormatAvailable.Call(_CF_UNICODETEXT)
	return marks, sizes, values, r != 0
}

func TestSensitiveTextGoesOnWithEveryMark(t *testing.T) {
	defer onRealClipboard(t)()
	if err := SetClipboardStringSensitive("hunter2"); err != nil {
		t.Fatal(err)
	}
	marks, sizes, values, text := onClipboard(t)
	if marks != [4]bool{true, true, true, true} || !text {
		t.Fatalf("the marks are there %v, the text %v; want all four and the text", marks, text)
	}
	if sizes != [4]uintptr{4, 4, 4, 4} || values != [4]uint32{} {
		t.Fatalf("the marks are %v bytes, holding %v; want each a DWORD of 0", sizes, values)
	}
	if err := platformSetClipboardString("a path"); err != nil {
		t.Fatal(err)
	}
	if marks, _, _, _ := onClipboard(t); marks != [4]bool{} {
		t.Fatalf("plain text carries the marks %v", marks)
	}
}

// Where a mark cannot be set, the text is not left on the clipboard without it, nor are the other marks.
func TestSensitiveTextWhoseMarkFailsIsNotLeftOnTheClipboard(t *testing.T) {
	defer onRealClipboard(t)()
	for _, f := range sensitiveFormats {
		markFails = func(name string) bool { return name == f.name }
		err := SetClipboardStringSensitive("hunter2")
		markFails = nil
		if err == nil {
			t.Fatalf("with the mark %s failing, the copy said nothing", f.name)
		}
		if marks, _, _, text := onClipboard(t); text || marks != [4]bool{} {
			t.Fatalf("with the mark %s failing, the clipboard holds the text %v and the marks %v; want nothing",
				f.name, text, marks)
		}
	}
}

// The clear goes by ownership for the sensitive text: a copy made since, even of the same text, survives.
func TestTheSensitiveTextIsClearedOnlyWhileTheClipboardIsStillOurs(t *testing.T) {
	defer onRealClipboard(t)()
	if err := SetClipboardStringSensitive("hunter2"); err != nil {
		t.Fatal(err)
	}
	if cleared, err := ClearClipboardStringIf("hunter2"); !cleared || err != nil {
		t.Fatalf("clearing the sensitive text still there cleared %v, %v", cleared, err)
	}
	if _, _, _, text := onClipboard(t); text {
		t.Fatal("after clearing, the clipboard still holds text")
	}

	// The user copies the same password from elsewhere: as another program would, this leaves the record alone.
	if err := SetClipboardStringSensitive("hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := putClipboardText("hunter2", false); err != nil {
		t.Fatal(err)
	}
	if cleared, err := ClearClipboardStringIf("hunter2"); cleared || err != nil {
		t.Fatalf("clearing once the user copied the same text cleared %v, %v; want it left", cleared, err)
	}
	if s, err := platformGetClipboardString(); s != "hunter2" || err != nil {
		t.Fatalf("the user's copy reads %q, %v", s, err)
	}

	// Text put there plainly is cleared by comparing, and an empty clipboard has nothing to clear.
	if err := platformSetClipboardString("a path"); err != nil {
		t.Fatal(err)
	}
	if cleared, err := ClearClipboardStringIf("other"); cleared || err != nil {
		t.Fatalf("clearing other text cleared %v, %v", cleared, err)
	}
	if cleared, err := ClearClipboardStringIf("a path"); !cleared || err != nil {
		t.Fatalf("clearing the plain text there cleared %v, %v", cleared, err)
	}
	if cleared, err := ClearClipboardStringIf("a path"); cleared || err != nil {
		t.Fatalf("clearing an empty clipboard cleared %v, %v", cleared, err)
	}
}
