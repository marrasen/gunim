//go:build js && wasm

package web

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/instream"
	"github.com/marrasen/gunim/internal/outstream"
)

// This file is files to and from the user's computer: dropped on the canvas,
// chosen in the browser's file picker, and saved where the user says or as a
// download. A browser gives no paths, so every file is a stream, read from the
// browser's Blob a slice at a time and written a chunk at a time, and none is
// ever whole in the program's memory.

var (
	_ driver.StreamChooser = (*Window)(nil)
	_ driver.StreamSaver   = (*Window)(nil)
)

// listenFiles hears files dragged over the canvas from the computer, and
// dropped on it. They reach the window under them as files from a desktop's
// file manager do, with no names until the drop, which a browser keeps back
// until then.
func (d *Driver) listenFiles(c js.Value) {
	over := func(e js.Value) {
		if !carriesFiles(e) {
			return
		}
		// Taken, or the browser opens the file in place of the page.
		e.Call("preventDefault")
		e.Get("dataTransfer").Set("dropEffect", "copy")
		p, mods := d.at(e), modsOf(e)
		d.mu.Lock()
		w := d.hitLocked(p)
		left := d.filesOver
		var local = p
		if w != nil {
			local = p.Sub(w.rectLocked().Min)
		}
		// A browser says the drag is still there several times a second, where
		// it moved or not.
		same := w == left && local == d.filesAt && mods == d.filesMods
		d.filesOver, d.filesAt, d.filesMods = w, local, mods
		d.mu.Unlock()
		if same {
			return
		}
		if left != nil && left != w {
			left.in.Push(driver.FilesLeft{})
		}
		if w != nil {
			w.in.Push(driver.FilesOver{Pos: local, Mods: mods})
		}
	}
	d.on(c, "dragenter", false, over)
	d.on(c, "dragover", false, over)
	d.on(c, "dragleave", false, func(js.Value) {
		d.mu.Lock()
		w := d.filesOver
		d.filesOver = nil
		d.mu.Unlock()
		if w != nil {
			w.in.Push(driver.FilesLeft{})
		}
	})
	d.on(c, "drop", false, func(e js.Value) {
		if !carriesFiles(e) {
			return
		}
		e.Call("preventDefault")
		// The files can be read only while the browser hands the drop over.
		streams := droppedStreams(e.Get("dataTransfer"))
		p, mods, now := d.at(e), modsOf(e), stamp(e)
		d.mu.Lock()
		w := d.hitLocked(p)
		left := d.filesOver
		d.filesOver = nil
		var local = p
		if w != nil {
			local = p.Sub(w.rectLocked().Min)
		}
		d.mu.Unlock()
		if left != nil && left != w {
			left.in.Push(driver.FilesLeft{})
		}
		if w == nil {
			for _, s := range streams {
				_ = s.Close()
			}
			return
		}
		w.in.Push(input.Drop{Pos: local, Data: input.Files{Streams: streams}, Mods: mods, Time: now})
	})
}

// carriesFiles reports whether a drag event carries files from the computer,
// rather than text or a link.
func carriesFiles(e js.Value) bool {
	dt := e.Get("dataTransfer")
	if !dt.Truthy() {
		return false
	}
	types := dt.Get("types")
	for i := range types.Length() {
		if types.Index(i).String() == "Files" {
			return true
		}
	}
	return false
}

// droppedStreams returns the files of a drop, in order. A folder is there too,
// failing at its first Read.
func droppedStreams(dt js.Value) []*input.Stream {
	var streams []*input.Stream
	items := dt.Get("items")
	if !items.Truthy() {
		files := dt.Get("files")
		for i := range files.Length() {
			streams = append(streams, blobStream(files.Index(i)))
		}
		return streams
	}
	for i := range items.Length() {
		it := items.Index(i)
		if it.Get("kind").String() != "file" {
			continue
		}
		f := it.Call("getAsFile")
		if !f.Truthy() {
			continue
		}
		if it.Get("webkitGetAsEntry").Type() == js.TypeFunction {
			if entry, err := call(it, "webkitGetAsEntry"); err == nil && entry.Truthy() && entry.Get("isDirectory").Truthy() {
				streams = append(streams, instream.Failing(nameOf(f), input.ErrFolder))
				continue
			}
		}
		streams = append(streams, blobStream(f))
	}
	return streams
}

// nameOf returns a File's name, safe as one name in a folder.
func nameOf(f js.Value) string {
	name := ""
	if n := f.Get("name"); n.Type() == js.TypeString {
		name = n.String()
	}
	return filecopy.SafeName(name)
}

// blobStream returns a File, or any Blob, as a stream.
func blobStream(b js.Value) *input.Stream {
	size := int64(b.Get("size").Float())
	mime := ""
	if t := b.Get("type"); t.Type() == js.TypeString {
		mime = t.String()
	}
	return input.NewStream(nameOf(b), instream.Type(mime), size, &blobReader{blob: b, size: size})
}

// sliceSize is how much of a Blob is asked of the browser at a time. The
// slice is the browser's, outside the program's memory, and is copied in as
// Read asks for it.
const sliceSize = 1 << 20

// blobReader reads a Blob a slice at a time. One goroutine reads, as for any
// reader, and Close may come from another.
type blobReader struct {
	blob js.Value
	size int64

	// slice is the browser's bytes from off on, of which at has been read.
	slice   js.Value
	at, len int
	off     int64
	err     error
	closed  atomic.Bool
}

// Read implements [io.Reader]. It returns [io.EOF] only at the end of the
// whole file, and an error that is [input.ErrNotWhole], with the browser's
// words, for one it could not read to its end, as a file changed or taken
// away since it was chosen.
func (r *blobReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if r.err != nil {
		return 0, r.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.at == r.len {
		if r.off >= r.size {
			r.err = io.EOF
			return 0, r.err
		}
		end := min(r.off+sliceSize, r.size)
		buf, err := r.readSlice(r.off, end)
		if r.closed.Load() {
			return 0, io.ErrClosedPipe
		}
		if err != nil {
			r.err = fmt.Errorf("%w: %w", input.ErrNotWhole, err)
			return 0, r.err
		}
		n := buf.Get("byteLength").Int()
		if n == 0 {
			r.err = input.ErrNotWhole
			return 0, r.err
		}
		r.slice, r.at, r.len = js.Global().Get("Uint8Array").New(buf), 0, n
		r.off += int64(n)
	}
	k := min(len(p), r.len-r.at)
	js.CopyBytesToGo(p[:k], r.slice.Call("subarray", r.at, r.at+k))
	r.at += k
	if r.at == r.len {
		r.slice = js.Undefined()
	}
	return k, nil
}

// readSlice asks the browser for the bytes from start to end, and waits.
func (r *blobReader) readSlice(start, end int64) (js.Value, error) {
	part, err := call(r.blob, "slice", float64(start), float64(end))
	if err != nil {
		return js.Undefined(), err
	}
	p, err := call(part, "arrayBuffer")
	if err != nil {
		return js.Undefined(), err
	}
	return await(p, 0)
}

// Close implements [io.Closer]. A read under way ends as its slice comes.
func (r *blobReader) Close() error {
	r.closed.Store(true)
	return nil
}

// ChooseStreams implements [driver.StreamChooser] with the browser's file
// picker, an input of type file. It returns no streams where the user
// cancelled, in a browser that says so, as all of today's do. A browser shows
// the picker only as the user clicks or types, and ChooseStreams returns
// [driver.ErrNoGesture] where it can tell that the user has done neither for a
// while. Filters become the picker's accept, where each pattern is an
// extension, and o.Title and o.Folder are the browser's to choose.
func (w *Window) ChooseStreams(o driver.ChooseOptions) ([]*input.Stream, error) {
	doc := js.Global().Get("document")
	in := doc.Call("createElement", "input")
	in.Set("type", "file")
	in.Set("multiple", o.Multiple)
	if accept := acceptOf(o.Filters); accept != "" {
		in.Set("accept", accept)
	}
	for k, v := range map[string]string{
		"position": "fixed", "left": "0", "top": "0", "width": "1px", "height": "1px",
		"opacity": "0", "pointerEvents": "none",
	} {
		in.Get("style").Set(k, v)
	}
	in.Call("setAttribute", "aria-hidden", "true")
	in.Call("setAttribute", "tabindex", "-1")
	doc.Get("body").Call("appendChild", in)
	defer in.Call("remove")

	got := make(chan []*input.Stream, 1)
	chosen := js.FuncOf(func(js.Value, []js.Value) any {
		files := in.Get("files")
		streams := make([]*input.Stream, 0, files.Length())
		for i := range files.Length() {
			streams = append(streams, blobStream(files.Index(i)))
		}
		select {
		case got <- streams:
		default:
		}
		return nil
	})
	cancelled := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case got <- nil:
		default:
		}
		return nil
	})
	defer chosen.Release()
	defer cancelled.Release()
	in.Call("addEventListener", "change", chosen)
	in.Call("addEventListener", "cancel", cancelled)

	if in.Get("showPicker").Type() == js.TypeFunction {
		if _, err := call(in, "showPicker"); err != nil {
			var be *browserError
			if errors.As(err, &be) && (be.name == "NotAllowedError" || be.name == "SecurityError") {
				return nil, driver.ErrNoGesture
			}
			return nil, err
		}
	} else {
		if !activeGesture() {
			return nil, driver.ErrNoGesture
		}
		in.Call("click")
	}
	select {
	case streams := <-got:
		return streams, nil
	case <-w.quit:
		return nil, errClosed
	}
}

// activeGesture reports whether the user has just clicked or typed, as a
// browser asks before it shows a picker, or true where the browser does not
// say.
func activeGesture() bool {
	ua := js.Global().Get("navigator").Get("userActivation")
	if !ua.Truthy() {
		return true
	}
	return ua.Get("isActive").Truthy()
}

// extensionsOf returns the extensions filters name, with their dots, or false
// where a pattern is more than an extension, or takes every file: the
// browser's picker can show only by extension, and shows every file then.
func extensionsOf(f driver.FileFilter) ([]string, bool) {
	var exts []string
	for _, p := range f.Patterns {
		ext, ok := strings.CutPrefix(p, "*.")
		if !ok || ext == "" || ext == "*" || strings.ContainsAny(ext, "*?[]{}/\\,; ") {
			return nil, false
		}
		exts = append(exts, "."+ext)
	}
	return exts, len(exts) > 0
}

// acceptOf returns the accept attribute of a picker that shows the files of
// all filters, or "" for every file.
func acceptOf(filters []driver.FileFilter) string {
	var all []string
	for _, f := range filters {
		exts, ok := extensionsOf(f)
		if !ok {
			return ""
		}
		all = append(all, exts...)
	}
	return strings.Join(all, ",")
}

// SaveStream implements [driver.StreamSaver]. Where the browser has a
// dialog for saving a file, as Chromium's showSaveFilePicker, the user
// chooses a place, and the file is written there as the bytes come; it is
// nil where the user cancelled. A browser shows that dialog only as the
// user clicks or types, as it has none at all elsewhere: the file then
// becomes a download, collected in the browser's own storage, outside the
// program's memory, and handed to the browser's downloads as Close is
// called. Close then returns [driver.ErrMayNotBeKept], as the browser tells
// nothing of how the download went.
//
// A file the dialog chose is not Made, as nothing tells whether it was
// there: Remove leaves it as it was before, whatever was written, as the
// browser writes beside it until Close.
func (w *Window) SaveStream(o driver.SaveOptions) (driver.SavedFile, error) {
	name := filecopy.SafeName(o.Name)
	if js.Global().Get("showSaveFilePicker").Type() == js.TypeFunction {
		handle, err := pickPlace(name, o.Filters)
		switch {
		case err == nil:
			return newPicked(handle, name), nil
		case errors.Is(err, errCancelled):
			return nil, nil
		}
		// No gesture, or a page the browser keeps from the disk, as one in
		// a frame: the download does.
		log.Printf("gunim: web: no dialog for saving %s, so it downloads: %v", name, err)
	}
	return newDownload(name), nil
}

// SaveFolder implements [driver.StreamSaver]. A browser gives a page no
// folder to write in, so it returns [driver.ErrNoChooser].
func (w *Window) SaveFolder(driver.ChooseOptions) (driver.SavedFolder, error) {
	return nil, driver.ErrNoChooser
}

// errCancelled is the user closing the browser's dialog.
var errCancelled = errors.New("web: the dialog was cancelled")

// pickPlace shows the browser's dialog for saving a file, and returns the
// handle of the file chosen.
func pickPlace(name string, filters []driver.FileFilter) (js.Value, error) {
	opts := map[string]any{"suggestedName": name}
	var types []any
	for _, f := range filters {
		if exts, ok := extensionsOf(f); ok {
			ext := make([]any, len(exts))
			for i, e := range exts {
				ext[i] = e
			}
			types = append(types, map[string]any{"description": f.Name, "accept": map[string]any{"application/octet-stream": ext}})
		}
	}
	ask := func() (js.Value, error) {
		p, err := call(js.Global(), "showSaveFilePicker", opts)
		if err != nil {
			return js.Undefined(), err
		}
		return await(p, 0)
	}
	if len(types) > 0 {
		opts["types"] = types
	}
	handle, err := ask()
	var be *browserError
	if errors.As(err, &be) && be.name == "TypeError" && len(types) > 0 {
		// Kinds of file the browser won't take: any file will do.
		delete(opts, "types")
		handle, err = ask()
	}
	if errors.As(err, &be) && be.name == "AbortError" {
		return js.Undefined(), errCancelled
	}
	return handle, err
}

// picked is a file the user chose a place for in the browser's dialog,
// written through the browser's writable stream, which keeps the bytes
// beside the file until it is closed.
type picked struct {
	*outstream.Writer
	name   string
	handle js.Value

	mu       sync.Mutex
	writable js.Value
}

func newPicked(handle js.Value, name string) *picked {
	if n := handle.Get("name"); n.Type() == js.TypeString && n.String() != "" {
		name = n.String()
	}
	f := &picked{name: name, handle: handle}
	f.Writer = outstream.New(outstream.Target{Open: f.open, Finish: f.finish, Commit: func() error { return nil },
		Abandon: f.abandon, Remove: func() error { return driver.ErrNotMade }})
	return f
}

// Name implements [driver.SavedFile].
func (f *picked) Name() string { return f.name }

// Place implements [driver.SavedFile]: a browser tells no place.
func (f *picked) Place() string { return "" }

// Made implements [driver.SavedFile].
func (f *picked) Made() bool { return false }

func (f *picked) open() (io.Writer, error) {
	p, err := call(f.handle, "createWritable")
	if err != nil {
		return nil, err
	}
	wr, err := await(p, 0)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.writable = wr
	f.mu.Unlock()
	return jsWriter{wr}, nil
}

func (f *picked) finish(int64) error {
	f.mu.Lock()
	wr := f.writable
	f.mu.Unlock()
	p, err := call(wr, "close")
	if err != nil {
		return err
	}
	_, err = await(p, 0)
	return err
}

func (f *picked) abandon() {
	f.mu.Lock()
	wr := f.writable
	f.writable = js.Undefined()
	f.mu.Unlock()
	if wr.Truthy() {
		if p, err := call(wr, "abort"); err == nil {
			ignore(p)
		}
	}
}

// jsWriter writes to a browser's writable stream, a chunk at a time, waiting
// for each.
type jsWriter struct{ wr js.Value }

// Write implements [io.Writer].
func (j jsWriter) Write(b []byte) (int, error) {
	p, err := call(j.wr, "write", bytesOf(b))
	if err != nil {
		return 0, err
	}
	if _, err := await(p, 0); err != nil {
		return 0, err
	}
	return len(b), nil
}

// bytesOf copies b out to the browser.
func bytesOf(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

// ignore keeps a promise whose end no one waits for from being reported as
// rejected and unhandled.
func ignore(p js.Value) {
	if p.Truthy() && p.Get("catch").Type() == js.TypeFunction {
		p.Call("catch", ignored)
	}
}

var ignored = js.FuncOf(func(js.Value, []js.Value) any { return nil })

// mergeParts is how many Blobs a download holds before it makes them one,
// which the browser does without copying.
const mergeParts = 64

// revokeAfter is how long a download's bytes are kept once handed to the
// browser, which reads them as it saves.
var revokeAfter = time.Minute

// download is a file collected as Blobs, in the browser's storage, which may
// keep them on its disk, and handed to the browser's downloads once whole.
type download struct {
	*outstream.Writer
	name string

	mu    sync.Mutex
	parts js.Value
}

func newDownload(name string) *download {
	f := &download{name: name}
	f.Writer = outstream.New(outstream.Target{
		Open:    func() (io.Writer, error) { return downloadWriter{f}, nil },
		Finish:  f.finish,
		Commit:  func() error { return nil },
		Abandon: f.drop,
		Remove:  func() error { return nil },
	})
	f.parts = js.Global().Get("Array").New()
	return f
}

// Name implements [driver.SavedFile].
func (f *download) Name() string { return f.name }

// Place implements [driver.SavedFile]: a browser tells no place.
func (f *download) Place() string { return "" }

// Made implements [driver.SavedFile].
func (f *download) Made() bool { return true }

// add keeps a chunk. It is outstream's writer's Write.
func (f *download) add(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.parts.Truthy() {
		return errClosed
	}
	blob := js.Global().Get("Blob")
	f.parts.Call("push", blob.New([]any{bytesOf(b)}))
	if f.parts.Length() >= mergeParts {
		f.parts = js.Global().Get("Array").New(blob.New(f.parts))
	}
	return nil
}

// finish hands the file to the browser's downloads.
func (f *download) finish(int64) error {
	f.mu.Lock()
	parts := f.parts
	f.parts = js.Undefined()
	f.mu.Unlock()
	if !parts.Truthy() {
		return errClosed
	}
	blob := js.Global().Get("Blob").New(parts, map[string]any{"type": "application/octet-stream"})
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	doc := js.Global().Get("document")
	a := doc.Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", f.name)
	a.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", a)
	_, err := call(a, "click")
	a.Call("remove")
	time.AfterFunc(revokeAfter, func() { js.Global().Get("URL").Call("revokeObjectURL", url) })
	if err != nil {
		return err
	}
	return driver.ErrMayNotBeKept
}

func (f *download) drop() {
	f.mu.Lock()
	f.parts = js.Undefined()
	f.mu.Unlock()
}

// downloadWriter is a download's writer, for outstream.
type downloadWriter struct{ f *download }

// Write implements [io.Writer].
func (d downloadWriter) Write(b []byte) (int, error) {
	if err := d.f.add(b); err != nil {
		return 0, err
	}
	return len(b), nil
}
