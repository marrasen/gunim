//go:build js && wasm

package web

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// bigSize is the size of the big files the tests read and write: big beside
// the most the program's memory may grow by as they go, heapRoom, and well
// under what the browser can hold for one test.
const (
	bigSize  = 160 << 20
	heapRoom = 40 << 20
)

// patternUnit is a run of the pattern the big files hold, byte i being i%251,
// that repeats without a seam.
const patternUnit = 251 * 4096

// patternAt fills b with the pattern from offset off.
func patternAt(b []byte, off int64) {
	for i := range b {
		b[i] = byte((off + int64(i)) % 251)
	}
}

// patternChecker counts the bytes written to it and checks each against the
// pattern.
type patternChecker struct {
	n   int64
	bad error
}

func (c *patternChecker) Write(b []byte) (int, error) {
	for i, x := range b {
		if want := byte((c.n + int64(i)) % 251); x != want && c.bad == nil {
			c.bad = fmt.Errorf("byte %d is %d, want %d", c.n+int64(i), x, want)
		}
	}
	c.n += int64(len(b))
	return len(b), nil
}

// growth runs f and returns the more of how much the program's memory grew
// meanwhile, and how much f allocated in all. WebAssembly memory only grows,
// so the first is all the memory f needed at once, beyond what earlier tests
// left; the second counts what f let go of too, so a file held whole at any
// moment shows in it however much room was there before.
func growth(f func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return max(after.Sys-before.Sys, after.TotalAlloc-before.TotalAlloc)
}

// newFile makes a browser File holding text.
func newFile(name, mime, text string) js.Value {
	return js.Global().Get("File").New([]any{text}, name, map[string]any{"type": mime})
}

// bigPatternFile makes a browser File of bigSize bytes of the pattern, held by
// the browser.
func bigPatternFile(name string) js.Value {
	unit := make([]byte, patternUnit)
	patternAt(unit, 0)
	arr := bytesOf(unit)
	parts := make([]any, 0, bigSize/patternUnit+1)
	for n := 0; n < bigSize; n += patternUnit {
		parts = append(parts, arr)
	}
	return js.Global().Get("File").New(parts, name)
}

// dragEvent sends a drag event of kind carrying dt to the canvas at p, in the
// canvas's space, and reports whether the page took it from the browser.
func dragEvent(d *Driver, kind string, dt js.Value, p geom.Point, shift bool) (taken bool) {
	r := d.canvas.Call("getBoundingClientRect")
	e := js.Global().Get("DragEvent").New(kind, map[string]any{
		"bubbles": true, "cancelable": true, "dataTransfer": dt, "shiftKey": shift,
		"clientX": r.Get("left").Float() + float64(p.X), "clientY": r.Get("top").Float() + float64(p.Y),
	})
	d.canvas.Call("dispatchEvent", e)
	return e.Get("defaultPrevented").Bool()
}

// filesOf makes a DataTransfer of files.
func filesOf(files ...js.Value) js.Value {
	dt := js.Global().Get("DataTransfer").New()
	for _, f := range files {
		dt.Get("items").Call("add", f)
	}
	return dt
}

func TestFilesDraggedInFromTheComputer(t *testing.T) {
	d, w := testWindow(t)
	drain(w)
	dt := filesOf(newFile("notes.txt", "text/plain", "hello"), newFile("b.log", "", "a log"))

	// Over the window, the drag is the window's as a desktop's is, with no
	// names yet; it is told again only when it moves.
	if !dragEvent(d, "dragenter", dt, geom.Pt(30, 40), false) || !dragEvent(d, "dragover", dt, geom.Pt(30, 40), false) {
		t.Fatal("the page left the drag of files to the browser, which would open them in its place")
	}
	dragEvent(d, "dragover", dt, geom.Pt(50, 60), true)
	got := drain(w)
	want := []any{driver.FilesOver{Pos: geom.Pt(30, 40)}, driver.FilesOver{Pos: geom.Pt(50, 60), Mods: input.ModShift}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("dragging over brought %v, want %v", got, want)
	}
	dragEvent(d, "dragleave", dt, geom.Pt(50, 60), false)
	if got := drain(w); len(got) != 1 || got[0] != (driver.FilesLeft{}) {
		t.Fatalf("leaving brought %v, want FilesLeft", got)
	}

	// The drop brings each file to read, by its name, size and type.
	dragEvent(d, "dragover", dt, geom.Pt(30, 40), false)
	if !dragEvent(d, "drop", dt, geom.Pt(30, 40), false) {
		t.Fatal("the browser kept the drop")
	}
	got = drain(w)
	if len(got) != 2 {
		t.Fatalf("a move and a drop brought %v", got)
	}
	drop, ok := got[1].(input.Drop)
	files, isFiles := drop.Data.(input.Files)
	if !ok || !isFiles || drop.Pos != geom.Pt(30, 40) || len(files.Streams) != 2 || files.Paths != nil {
		t.Fatalf("the drop brought %#v", got[1])
	}
	for i, want := range []struct{ name, mime, text string }{{"notes.txt", "text/plain", "hello"}, {"b.log", "", "a log"}} {
		s := files.Streams[i]
		b, err := io.ReadAll(s)
		if err != nil || string(b) != want.text || s.Name != want.name || s.Type != want.mime || s.Size != int64(len(want.text)) {
			t.Fatalf("file %d is %q %q %d, read %q, %v; want %+v", i, s.Name, s.Type, s.Size, b, err, want)
		}
		_ = s.Close()
	}

	// A drag of text is the browser's.
	text := js.Global().Get("DataTransfer").New()
	text.Call("setData", "text/plain", "words")
	if dragEvent(d, "dragover", text, geom.Pt(30, 40), false) || dragEvent(d, "drop", text, geom.Pt(30, 40), false) {
		t.Fatal("the page took a drag of text")
	}
	if got := drain(w); len(got) != 0 {
		t.Fatalf("a drag of text brought %v", got)
	}
}

func TestFilesDroppedOffEveryWindowAreLetGo(t *testing.T) {
	d, w := testWindow(t)
	setCanvasSize(t, d, "320px", "240px")
	drain(w)
	w.d.mu.Lock()
	w.fills = false
	w.pos, w.size = geom.Pt(0, 0), geom.Sz(100, 100)
	w.d.mu.Unlock()
	t.Cleanup(func() {
		w.d.mu.Lock()
		w.fills = true
		w.d.mu.Unlock()
	})
	dt := filesOf(newFile("a.txt", "", "a"))
	dragEvent(d, "dragover", dt, geom.Pt(50, 50), false)
	dragEvent(d, "dragover", dt, geom.Pt(200, 200), false)
	dragEvent(d, "drop", dt, geom.Pt(200, 200), false)
	got := drain(w)
	if len(got) != 2 || got[1] != (driver.FilesLeft{}) {
		t.Fatalf("files moved off the window and dropped brought %v, want FilesOver and FilesLeft", got)
	}
}

func TestABigFileIsReadAPieceAtATime(t *testing.T) {
	f := bigPatternFile("big.bin")
	s := blobStream(f)
	defer func() { _ = s.Close() }()
	if s.Size < bigSize {
		t.Fatalf("the file has %d bytes, want %d or more", s.Size, bigSize)
	}
	var c patternChecker
	var err error
	grew := growth(func() { _, err = io.Copy(&c, s) })
	if err != nil || c.bad != nil || c.n != s.Size {
		t.Fatalf("read %d of %d bytes, %v, %v", c.n, s.Size, err, c.bad)
	}
	if grew > heapRoom {
		t.Fatalf("reading %d MB grew the program's memory by %d MB", c.n>>20, grew>>20)
	}
	t.Logf("read %d MB; the program's memory grew, or it allocated, %d MB", c.n>>20, grew>>20)
}

// fakeBlob is a Blob whose slices after the first fail, as a file changed
// since it was chosen does.
const fakeBlob = `({size: 3 << 20, name: "gone.bin", type: "", slice(a, b) {
	return {arrayBuffer: () => a == 0 ? Promise.resolve(new ArrayBuffer(b - a))
		: Promise.reject(new DOMException("the file changed", "NotReadableError"))};
}})`

func TestAReadThatFailsMidwaySaysSo(t *testing.T) {
	s := blobStream(js.Global().Call("eval", fakeBlob))
	n, err := io.Copy(io.Discard, s)
	if !errors.Is(err, input.ErrNotWhole) || !strings.Contains(err.Error(), "NotReadableError") || n != sliceSize {
		t.Fatalf("read %d bytes, %v; want the first slice and ErrNotWhole with the browser's words", n, err)
	}
	_ = s.Close()

	// A file shorter than it said is not whole either.
	short := js.Global().Call("eval", `({size: 10, name: "short", slice: () => ({arrayBuffer: () => Promise.resolve(new ArrayBuffer(0))})})`)
	if _, err := io.ReadAll(blobStream(short)); !errors.Is(err, input.ErrNotWhole) {
		t.Fatalf("a short file read with %v", err)
	}

	// A closed stream reads no more.
	s = blobStream(newFile("a.txt", "", "abc"))
	_ = s.Close()
	if _, err := s.Read(make([]byte, 4)); err == nil {
		t.Fatal("a closed stream read")
	}
}

func TestPickerFilters(t *testing.T) {
	for _, c := range []struct {
		filters []driver.FileFilter
		want    string
	}{
		{nil, ""},
		{[]driver.FileFilter{{Name: "Logs", Patterns: []string{"*.log", "*.jsonl"}}, {Name: "Text", Patterns: []string{"*.txt"}}}, ".log,.jsonl,.txt"},
		{[]driver.FileFilter{{Name: "Logs", Patterns: []string{"*.log"}}, {Name: "All", Patterns: []string{"*"}}}, ""},
		{[]driver.FileFilter{{Name: "Odd", Patterns: []string{"report-*.txt"}}}, ""},
	} {
		if got := acceptOf(c.filters); got != c.want {
			t.Errorf("acceptOf(%v) = %q, want %q", c.filters, got, c.want)
		}
	}
}

// helper returns the test runner's window.webtest, which does what only a
// user can, or skips where there is none, as on a page opened by hand.
func helper(t *testing.T) js.Value {
	h := js.Global().Get("webtest")
	if !h.Truthy() {
		t.Skip("no test runner to click or choose files")
	}
	return h
}

// ask asks the runner for what, and waits for its answer.
func ask(t *testing.T, what string, params any) {
	t.Helper()
	v, err := await(helper(t).Call("ask", what, params), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if e := v.Get("error"); e.Truthy() {
		t.Fatalf("the runner could not %s: %s", what, e.String())
	}
}

// click has the runner click the page beside the canvas, as a user would,
// which lets the page show a picker for a few seconds.
func click(t *testing.T) { ask(t, "click", map[string]any{"x": 700, "y": 100}) }

func TestThePickerGivesTheFilesChosen(t *testing.T) {
	helper(t)
	_, w := testWindow(t)

	// With no click lately, the browser shows no picker, and the program
	// hears why.
	ua := js.Global().Get("navigator").Get("userActivation")
	for give := time.Now().Add(10 * time.Second); ua.Get("isActive").Bool(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(give) {
			t.Fatal("the page's last click never lapsed")
		}
	}
	if s, err := w.ChooseStreams(driver.ChooseOptions{}); s != nil || !errors.Is(err, driver.ErrNoGesture) {
		t.Fatalf("a picker with no click gave %v, %v; want ErrNoGesture", s, err)
	}

	ask(t, "choose", []any{map[string]any{"name": "a.txt", "size": 5, "text": "hello"}, map[string]any{"name": "big.bin", "size": bigSize, "text": "start"}})
	click(t)
	got, err := w.ChooseStreams(driver.ChooseOptions{Multiple: true})
	if err != nil || len(got) != 2 {
		t.Fatalf("chose %v, %v", got, err)
	}
	defer func() {
		for _, s := range got {
			_ = s.Close()
		}
	}()
	if b, err := io.ReadAll(got[0]); err != nil || string(b) != "hello" || got[0].Name != "a.txt" || got[0].Size != 5 {
		t.Fatalf("the first file is %q of %d, read %q, %v", got[0].Name, got[0].Size, b, err)
	}
	big := got[1]
	head := make([]byte, 5)
	var n int64
	grew := growth(func() {
		if _, err = io.ReadFull(big, head); err != nil {
			return
		}
		n, err = io.Copy(io.Discard, big)
	})
	if err != nil || string(head) != "start" || n+5 != bigSize || big.Size != bigSize || big.Name != "big.bin" {
		t.Fatalf("the big file %q of %d read %q and %d bytes, %v", big.Name, big.Size, head, n, err)
	}
	if grew > heapRoom {
		t.Fatalf("reading %d MB grew the program's memory by %d MB", bigSize>>20, grew>>20)
	}
	t.Logf("read %d MB; the program's memory grew, or it allocated, %d MB", bigSize>>20, grew>>20)

	// The user closing the picker is no error, and chooses nothing.
	click(t)
	done := make(chan error, 1)
	var cancelled []*input.Stream
	go func() {
		var err error
		cancelled, err = w.ChooseStreams(driver.ChooseOptions{})
		done <- err
	}()
	doc := js.Global().Get("document")
	waitFor(t, "the picker", func() bool { return doc.Call("querySelector", "input[type=file]").Truthy() })
	doc.Call("querySelector", "input[type=file]").Call("dispatchEvent", js.Global().Get("Event").New("cancel"))
	if err := <-done; err != nil || cancelled != nil {
		t.Fatalf("a cancelled picker gave %v, %v", cancelled, err)
	}
	if doc.Call("querySelector", "input[type=file]").Truthy() {
		t.Fatal("the picker's input stayed on the page")
	}
}

// stubbed replaces a property of the page's window for the test.
func stubbed(t *testing.T, name string, v any) {
	obj := js.Global().Get("Object")
	prior := obj.Call("getOwnPropertyDescriptor", js.Global(), name)
	obj.Call("defineProperty", js.Global(), name, map[string]any{"configurable": true, "writable": true, "value": v})
	t.Cleanup(func() {
		if prior.IsUndefined() {
			js.Global().Get("Reflect").Call("deleteProperty", js.Global(), name)
		} else {
			obj.Call("defineProperty", js.Global(), name, prior)
		}
	})
}

// writePattern writes n bytes of the pattern to f, a chunk at a time.
func writePattern(f io.Writer, n int64) error {
	buf := make([]byte, 1<<20)
	for off := int64(0); off < n; {
		k := min(int64(len(buf)), n-off)
		patternAt(buf[:k], off)
		if _, err := f.Write(buf[:k]); err != nil {
			return err
		}
		off += k
	}
	return nil
}

// opfs returns the page's own private folder, where a test's stand-in for the
// save dialog keeps the file chosen.
func opfs(t *testing.T) js.Value {
	root, err := await(js.Global().Get("navigator").Get("storage").Call("getDirectory"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSaveStreamWritesToThePlaceChosen(t *testing.T) {
	_, w := testWindow(t)
	root := opfs(t)
	t.Cleanup(func() { ignore(root.Call("removeEntry", "saved.bin")) })
	// The dialog, as the user would answer it, choosing saved.bin.
	stubbed(t, "showSaveFilePicker", js.Global().Call("eval", `(opts => {
		window.savedWith = opts;
		return navigator.storage.getDirectory().then(d => d.getFileHandle("saved.bin", {create: true}));
	})`))

	f, err := w.SaveStream(driver.SaveOptions{Name: "../out.bin", Filters: []driver.FileFilter{{Name: "Data", Patterns: []string{"*.bin"}}}})
	if err != nil || f == nil {
		t.Fatalf("saving gave %v, %v", f, err)
	}
	opts := js.Global().Get("savedWith")
	if opts.Get("suggestedName").String() != "out.bin" || opts.Get("types").Index(0).Get("accept").Get("application/octet-stream").Index(0).String() != ".bin" {
		t.Fatalf("the dialog was asked with %s", js.Global().Get("JSON").Call("stringify", opts).String())
	}
	if f.Name() != "saved.bin" || f.Made() || f.Place() != "" {
		t.Fatalf("the file is %q, made %v, at %q", f.Name(), f.Made(), f.Place())
	}
	grew := growth(func() {
		if err = writePattern(f, bigSize); err == nil {
			err = f.Close()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if grew > heapRoom {
		t.Fatalf("writing %d MB grew the program's memory by %d MB", bigSize>>20, grew>>20)
	}
	t.Logf("wrote %d MB; the program's memory grew, or it allocated, %d MB", bigSize>>20, grew>>20)
	handle, err := await(root.Call("getFileHandle", "saved.bin"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	file, err := await(handle.Call("getFile"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var c patternChecker
	if _, err := io.Copy(&c, blobStream(file)); err != nil || c.bad != nil || c.n != bigSize {
		t.Fatalf("the file saved holds %d bytes, %v, %v", c.n, err, c.bad)
	}

	// Removed before it is whole, the file is as it was before: the browser
	// writes beside it.
	f, err = w.SaveStream(driver.SaveOptions{Name: "out.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove(); !errors.Is(err, driver.ErrNotMade) {
		t.Fatalf("Remove = %v, want ErrNotMade", err)
	}
	file, _ = await(handle.Call("getFile"), 10*time.Second)
	if size := int64(file.Get("size").Float()); size != bigSize {
		t.Fatalf("after Remove the file holds %d bytes, want the %d from before", size, bigSize)
	}
}

func TestSaveStreamTellsCancelFromFailure(t *testing.T) {
	_, w := testWindow(t)

	stubbed(t, "showSaveFilePicker", js.Global().Call("eval", `(() => Promise.reject(new DOMException("closed", "AbortError")))`))
	if f, err := w.SaveStream(driver.SaveOptions{Name: "a.txt"}); f != nil || err != nil {
		t.Fatalf("a cancelled dialog gave %v, %v; want nothing", f, err)
	}

	// A dialog the browser won't show, as with no click, has the file download.
	stubbed(t, "showSaveFilePicker", js.Global().Call("eval", `(() => Promise.reject(new DOMException("no gesture", "SecurityError")))`))
	if f, err := w.SaveStream(driver.SaveOptions{Name: "a.txt"}); err != nil || f == nil {
		t.Fatalf("a refused dialog gave %v, %v", f, err)
	} else if _, ok := f.(*download); !ok {
		t.Fatalf("a refused dialog gave a %T, want a download", f)
	} else {
		_ = f.Remove()
	}

	// Kinds of file the browser won't take are left out.
	stubbed(t, "showSaveFilePicker", js.Global().Call("eval", `(opts => opts.types
		? Promise.reject(new TypeError("bad extension"))
		: Promise.resolve({name: "a.txt", createWritable: () => Promise.resolve({
			n: 0,
			write() { return this.n++ ? Promise.reject(new DOMException("the disk is full", "QuotaExceededError")) : Promise.resolve(); },
			close: () => Promise.resolve(), abort: () => Promise.resolve(),
		})}))`))
	f, err := w.SaveStream(driver.SaveOptions{Name: "a.txt", Filters: []driver.FileFilter{{Name: "Odd", Patterns: []string{"*.x y"}}, {Name: "Text", Patterns: []string{"*.t"}}}})
	if err != nil || f == nil {
		t.Fatalf("a dialog that refused the kinds gave %v, %v", f, err)
	}
	// A write that fails midway says so, and so does every one after.
	err = writePattern(f, 4<<20)
	if err == nil || !strings.Contains(err.Error(), "QuotaExceededError") {
		t.Fatalf("a failing write gave %v", err)
	}
	if _, err := f.Write([]byte("more")); err == nil {
		t.Fatal("a write after a failure went through")
	}
	if err := f.Close(); err == nil {
		t.Fatal("a file whose write failed closed whole")
	}
}

func TestSaveStreamDownloadsWhereThereIsNoDialog(t *testing.T) {
	_, w := testWindow(t)
	stubbed(t, "showSaveFilePicker", js.Undefined())
	// The anchor's click, which would start the browser's download, keeps what
	// it would download.
	anchor := js.Global().Get("HTMLAnchorElement").Get("prototype")
	prior := anchor.Get("click")
	anchor.Set("click", js.Global().Call("eval", `(function() { window.downloaded = {href: this.href, name: this.download, on: this.isConnected}; })`))
	t.Cleanup(func() { anchor.Set("click", prior) })
	js.Global().Set("downloaded", js.Null())

	f, err := w.SaveStream(driver.SaveOptions{Name: "copy.bin"})
	if err != nil || f == nil || f.Name() != "copy.bin" || !f.Made() || f.Place() != "" {
		t.Fatalf("saving gave %v, %v", f, err)
	}
	grew := growth(func() {
		if err = writePattern(f, bigSize); err == nil {
			err = f.Close()
		}
	})
	if !errors.Is(err, driver.ErrMayNotBeKept) {
		t.Fatalf("Close = %v, want ErrMayNotBeKept: the browser tells nothing of a download", err)
	}
	if grew > heapRoom {
		t.Fatalf("writing %d MB grew the program's memory by %d MB", bigSize>>20, grew>>20)
	}
	t.Logf("wrote %d MB; the program's memory grew, or it allocated, %d MB", bigSize>>20, grew>>20)
	got := js.Global().Get("downloaded")
	if !got.Truthy() || got.Get("name").String() != "copy.bin" || !got.Get("on").Bool() {
		t.Fatal("nothing was handed to the browser's downloads")
	}
	res, err := await(js.Global().Call("fetch", got.Get("href")), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := await(res.Call("blob"), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var c patternChecker
	if _, err := io.Copy(&c, blobStream(blob)); err != nil || c.bad != nil || c.n != bigSize {
		t.Fatalf("the download holds %d bytes, %v, %v", c.n, err, c.bad)
	}

	// One removed before it is whole downloads nothing.
	js.Global().Set("downloaded", js.Null())
	f, _ = w.SaveStream(driver.SaveOptions{Name: "gone.bin"})
	if _, err := f.Write(bytes.Repeat([]byte("x"), 100)); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err == nil {
		t.Fatal("a removed file closed")
	}
	if js.Global().Get("downloaded").Truthy() {
		t.Fatal("a removed file downloaded")
	}
}
