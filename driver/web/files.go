//go:build js && wasm

package web

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/internal/instream"
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
	// Files let go anywhere else on the page, as on the note recovery.go
	// shows or on what the page puts round the canvas, would have the browser
	// open them in its place, and the program gone. The canvas has its own
	// first, and these are for the rest.
	outside := func(e js.Value) {
		if !carriesFiles(e) || e.Get("target").Equal(c) {
			return
		}
		e.Call("preventDefault")
		e.Get("dataTransfer").Set("dropEffect", "none")
	}
	win := js.Global()
	d.on(win, "dragover", false, outside)
	d.on(win, "drop", false, outside)
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

	// A browser without the cancel event says nothing of a picker closed
	// with no choice, but the page has the keyboard back: once it has
	// left and come back, and no choice follows in a while, the user
	// cancelled.
	away, back := make(chan struct{}, 1), make(chan struct{}, 1)
	blur := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case away <- struct{}{}:
		default:
		}
		return nil
	})
	focus := js.FuncOf(func(js.Value, []js.Value) any {
		select {
		case back <- struct{}{}:
		default:
		}
		return nil
	})
	defer blur.Release()
	defer focus.Release()
	win := js.Global()
	win.Call("addEventListener", "blur", blur)
	win.Call("addEventListener", "focus", focus)
	defer win.Call("removeEventListener", "blur", blur)
	defer win.Call("removeEventListener", "focus", focus)

	if in.Get("showPicker").Type() == js.TypeFunction {
		if _, err := call(in, "showPicker"); err != nil {
			var be *browserError
			switch {
			case errors.As(err, &be) && be.name == "NotAllowedError":
				return nil, driver.ErrNoGesture
			case errors.As(err, &be) && be.name == "SecurityError":
				// A page in a frame of another site's, which may show no picker.
				return nil, fmt.Errorf("%w: %w", driver.ErrNoDialog, err)
			}
			return nil, err
		}
	} else {
		if !activeGesture() {
			return nil, driver.ErrNoGesture
		}
		in.Call("click")
	}
	left := false
	var grace <-chan time.Time
	var graceTimer *time.Timer
	defer func() {
		if graceTimer != nil {
			graceTimer.Stop()
		}
	}()
	for {
		select {
		case streams := <-got:
			return streams, nil
		case <-away:
			left = true
		case <-back:
			if left && grace == nil {
				graceTimer = time.NewTimer(pickerGrace)
				grace = graceTimer.C
			}
		case <-grace:
			return nil, nil
		case <-w.quit:
			return nil, errClosed
		}
	}
}

// pickerGrace is how long a choice may take to arrive once the page has the
// keyboard back from the picker, before the picker counts as cancelled. A
// browser hands over the files chosen a moment after the picker closes.
var pickerGrace = 3 * time.Second

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
