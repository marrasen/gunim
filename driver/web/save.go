//go:build js && wasm

package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/filecopy"
	"github.com/marrasen/gunim/internal/outstream"
)

// This file is files saved from the program to the user's computer: where the
// user chooses, in a browser with a dialog for saving; as a download written to
// the disk as it comes, through gunim's save worker; and failing both, as a
// download kept in the browser until it is whole.

// SaveStream implements [driver.StreamSaver], in the first of three ways the
// browser allows:
//
//   - Where the browser has a dialog for saving, as Chromium's
//     showSaveFilePicker, and shows it, the user chooses a place, and the
//     file is written there as the bytes come. It is nil where the user
//     cancelled.
//   - Otherwise, where the page names gunim's save worker (see
//     [github.com/marrasen/gunim/driver/web/saveworker]) and the browser runs
//     it, the file is a download, written to the disk as the bytes come. A
//     Write waits until the download takes more, for as long as the browser
//     holds the download: while it asks the user where to save it, or
//     whether the site may download, or forever where the user never
//     answers. A program that gives up calls Remove, which ends the wait at
//     once.
//   - Otherwise the file is a download kept in the browser until Close, which
//     Firefox and Safari keep in memory: [driver.HeldInMemory] says so of it.
//
// A download's Close returns [driver.ErrMayNotBeKept], as the browser tells
// nothing of how the download went. A browser shows the dialog only as the
// user clicks or types, so a program asks in the click.
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
	sw, scope, err := saveWorker()
	if err == nil {
		var f *streamed
		if f, err = newStreamed(sw, scope, name); err == nil {
			return f, nil
		}
	}
	log.Printf("gunim: web: no save worker for %s, so the browser holds it whole until it is downloaded: %v", name, err)
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
//
// Chromium makes the file as the user chooses it, empty, and empties one that
// was there (the dialog has asked the user before it lets one be chosen): what
// was there is gone before the first byte is written. So the file is Made, and
// Remove takes it away, through the handle's remove where the browser has it,
// as Chromium does from version 110. Where it has none, Remove leaves the file
// empty and returns [driver.ErrNotSaved].
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
		Abandon: f.abandon, Remove: f.remove})
	return f
}

// Name implements [driver.SavedFile].
func (f *picked) Name() string { return f.name }

// Place implements [driver.SavedFile]: a browser tells no place.
func (f *picked) Place() string { return "" }

// Made implements [driver.SavedFile]. See [picked].
func (f *picked) Made() bool { return true }

// InMemory implements [driver.MemorySaved]: the browser writes the file to
// the disk as it comes.
func (f *picked) InMemory() bool { return false }

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

// abandon lets go of what was written, which the browser keeps beside the
// file and never moves into it.
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

// remove takes the file away, where the browser can.
func (f *picked) remove() error {
	if f.handle.Get("remove").Type() != js.TypeFunction {
		return fmt.Errorf("%w: the browser cannot take %s away", driver.ErrNotSaved, f.name)
	}
	p, err := call(f.handle, "remove")
	if err == nil {
		_, err = await(p, 30*time.Second)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", driver.ErrNotSaved, err)
	}
	return nil
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

// blobKeep is how long a download's link to its Blob is kept once clicked.
// Nothing tells when the browser has read it. Firefox fails a download whose
// link goes at once, and FileSaver.js keeps it 40 s for that. Whether Safari
// reads it before or after it asks the user whether the site may download is
// not known here, and a Blob is memory in Firefox and Safari, so it is not
// kept longer for that.
var blobKeep = 40 * time.Second

// download is a file collected as Blobs in the browser, and handed to the
// browser's downloads once whole. Chrome keeps large Blobs on its disk;
// Firefox and Safari keep them in memory.
//
// The Blobs are merged as a binary counter counts: levels[i] holds 2^i
// chunks, or none, so no Blob is nested more than about log2 of the chunks
// deep, and each chunk is merged that many times at most.
type download struct {
	*outstream.Writer
	name string

	mu     sync.Mutex
	levels []js.Value
	gone   bool
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
	return f
}

// Name implements [driver.SavedFile].
func (f *download) Name() string { return f.name }

// Place implements [driver.SavedFile]: a browser tells no place.
func (f *download) Place() string { return "" }

// Made implements [driver.SavedFile].
func (f *download) Made() bool { return true }

// InMemory implements [driver.MemorySaved]: the browser may keep the whole
// file in memory until it is downloaded.
func (f *download) InMemory() bool { return true }

// add keeps a chunk.
func (f *download) add(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return errClosed
	}
	blob := js.Global().Get("Blob")
	carry := blob.New([]any{bytesOf(b)})
	for i := 0; ; i++ {
		if i == len(f.levels) {
			f.levels = append(f.levels, carry)
			return nil
		}
		if f.levels[i].IsUndefined() {
			f.levels[i] = carry
			return nil
		}
		// The level holds the bytes before the carry's.
		carry = blob.New([]any{f.levels[i], carry})
		f.levels[i] = js.Undefined()
	}
}

// finish hands the file to the browser's downloads.
func (f *download) finish(int64) error {
	f.mu.Lock()
	levels, gone := f.levels, f.gone
	f.levels, f.gone = nil, true
	f.mu.Unlock()
	if gone {
		return errClosed
	}
	// The highest level holds the first bytes.
	var parts []any
	for i := len(levels) - 1; i >= 0; i-- {
		if !levels[i].IsUndefined() {
			parts = append(parts, levels[i])
		}
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
	time.AfterFunc(blobKeep, func() { js.Global().Get("URL").Call("revokeObjectURL", url) })
	if err != nil {
		return err
	}
	return driver.ErrMayNotBeKept
}

func (f *download) drop() {
	f.mu.Lock()
	f.levels, f.gone = nil, true
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

// workerWait is how long the browser may take to start the save worker, and
// a download through it.
var workerWait = 10 * time.Second

// saveWorker returns the save worker the page names, running, and the scope
// it serves, or why there is none. It asks the browser each time, which keeps
// the registration: so a worker updated since, or one that failed to start
// before, is found as it is now. It registers the worker where there is none.
var saveWorker = func() (js.Value, string, error) {
	doc := js.Global().Get("document")
	meta := doc.Call("querySelector", `meta[name="gunim-save-worker"]`)
	if !meta.Truthy() {
		return js.Undefined(), "", errors.New("the page names none")
	}
	sw := js.Global().Get("navigator").Get("serviceWorker")
	if !sw.Truthy() {
		return js.Undefined(), "", errors.New("the browser runs none here")
	}
	urls := js.Global().Get("URL")
	script := urls.New(meta.Call("getAttribute", "content").String(), doc.Get("baseURI")).Get("href").String()
	scope := urls.New("gunim-save/", script).Get("href").String()
	running := func(reg js.Value) js.Value {
		if !reg.Truthy() {
			return js.Undefined()
		}
		if a := reg.Get("active"); a.Truthy() && a.Get("state").String() == "activated" {
			return a
		}
		return js.Undefined()
	}
	if p, err := call(sw, "getRegistration", scope); err == nil {
		if reg, err := await(p, workerWait); err == nil && running(reg).Truthy() &&
			reg.Get("active").Get("scriptURL").String() == script {
			return running(reg), scope, nil
		}
	}
	p, err := call(sw, "register", script, map[string]any{"scope": scope})
	if err != nil {
		return js.Undefined(), "", err
	}
	reg, err := await(p, workerWait)
	if err != nil {
		return js.Undefined(), "", err
	}
	for give := time.Now().Add(workerWait); ; time.Sleep(20 * time.Millisecond) {
		if a := running(reg); a.Truthy() {
			return a, scope, nil
		}
		if time.Now().After(give) {
			return js.Undefined(), "", errors.New("it did not start")
		}
	}
}

// openDownload has the browser fetch address as a download, in a frame of
// its own, and returns what lets the frame go.
var openDownload = func(address string) func() {
	doc := js.Global().Get("document")
	f := doc.Call("createElement", "iframe")
	f.Set("hidden", true)
	f.Get("style").Set("display", "none")
	f.Call("setAttribute", "aria-hidden", "true")
	f.Set("src", address)
	doc.Get("body").Call("appendChild", f)
	return func() { f.Call("remove") }
}

// errDownloadCancelled is the user, or the browser, ending a download that
// went through the save worker.
var errDownloadCancelled = errors.New("web: the download was cancelled")

// streamed is a download the save worker writes to the disk as it comes. The
// worker asks for each chunk as the download takes the one before, and Write
// waits for that.
type streamed struct {
	*outstream.Writer
	name, id string
	sw, port js.Value
	heard    js.Func
	letGo    func()

	ready, started, pulls, blocked chan struct{}
	cancelled, gone                chan struct{}
	cancelOnce, goneOnce           sync.Once
	endOnce                        sync.Once
}

// errFrameBlocked is a page whose Content-Security-Policy keeps out the
// frame a download through the save worker is asked for in.
var errFrameBlocked = errors.New("web: the page's Content-Security-Policy keeps the download's frame out; it needs frame-src 'self'")

// newStreamed starts a download of name through the save worker sw, which
// serves scope, and returns once the browser has asked the worker for it.
func newStreamed(sw js.Value, scope, name string) (*streamed, error) {
	var b [16]byte
	_, _ = rand.Read(b[:])
	f := &streamed{name: name, id: hex.EncodeToString(b[:]), sw: sw,
		ready: make(chan struct{}, 1), started: make(chan struct{}, 1), pulls: make(chan struct{}, 1),
		blocked: make(chan struct{}, 1), cancelled: make(chan struct{}), gone: make(chan struct{})}
	f.heard = js.FuncOf(func(_ js.Value, args []js.Value) any {
		m := args[0].Get("data")
		note := func(ch chan struct{}) {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		switch {
		case m.Get("ready").Truthy():
			note(f.ready)
		case m.Get("started").Truthy():
			note(f.started)
		case m.Get("pull").Truthy():
			note(f.pulls)
		case m.Get("cancelled").Truthy():
			f.cancelOnce.Do(func() { close(f.cancelled) })
		}
		return nil
	})
	ch := js.Global().Get("MessageChannel").New()
	f.port = ch.Get("port1")
	f.port.Set("onmessage", f.heard)
	if _, err := call(sw, "postMessage", map[string]any{"gunimSave": f.id, "name": name}, []any{ch.Get("port2")}); err != nil {
		f.end()
		return nil, err
	}
	if err := f.wait(f.ready, "take it"); err != nil {
		f.end()
		return nil, err
	}
	address := scope + f.id + "/" + js.Global().Call("encodeURIComponent", name).String()
	// A policy that keeps the frame out says so at once, where waiting for
	// the download would take workerWait.
	violation := js.FuncOf(func(_ js.Value, args []js.Value) any {
		e := args[0]
		if e.Get("disposition").String() == "enforce" && strings.HasPrefix(e.Get("blockedURI").String(), scope) {
			select {
			case f.blocked <- struct{}{}:
			default:
			}
		}
		return nil
	})
	doc := js.Global().Get("document")
	doc.Call("addEventListener", "securitypolicyviolation", violation)
	f.letGo = openDownload(address)
	err := f.wait(f.started, "start the download")
	doc.Call("removeEventListener", "securitypolicyviolation", violation)
	violation.Release()
	if err != nil {
		f.send(map[string]any{"abort": "never started"}, nil)
		f.end()
		return nil, err
	}
	f.Writer = outstream.New(outstream.Target{
		Open:    func() (io.Writer, error) { return streamWriter{f}, nil },
		Finish:  f.finish,
		Commit:  func() error { return nil },
		Abandon: f.abandon,
		Remove:  func() error { return nil },
	})
	go f.keepAlive()
	return f, nil
}

// wait waits for the worker to say ch, for workerWait at most.
func (f *streamed) wait(ch chan struct{}, what string) error {
	t := time.NewTimer(workerWait)
	defer t.Stop()
	select {
	case <-ch:
		return nil
	case <-f.blocked:
		return errFrameBlocked
	case <-t.C:
		return fmt.Errorf("the save worker did not %s", what)
	}
}

// send hands the worker a message about the file.
func (f *streamed) send(m map[string]any, transfer []any) {
	m["gunimSave"] = f.id
	if _, err := call(f.sw, "postMessage", m, transfer); err != nil {
		log.Printf("gunim: web: the save worker: %v", err)
	}
}

// keepAlive tells the worker now and then that the file goes on, as a
// browser stops a worker that hears nothing for a while, and the download
// with it, while the download waits for the program.
func (f *streamed) keepAlive() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			f.send(map[string]any{"ping": true}, nil)
		case <-f.gone:
			return
		}
	}
}

// next waits until the download asks for more.
func (f *streamed) next() error {
	select {
	case <-f.pulls:
		return nil
	case <-f.cancelled:
		return errDownloadCancelled
	case <-f.gone:
		return errClosed
	}
}

// streamWriter is a streamed download's writer, for outstream.
type streamWriter struct{ f *streamed }

// Write implements [io.Writer]: a chunk, once the download asks for it.
func (w streamWriter) Write(b []byte) (int, error) {
	if err := w.f.next(); err != nil {
		return 0, err
	}
	buf := bytesOf(b).Get("buffer")
	w.f.send(map[string]any{"chunk": buf}, []any{buf})
	return len(b), nil
}

func (f *streamed) finish(int64) error {
	if err := f.next(); err != nil {
		return err
	}
	f.send(map[string]any{"end": true}, nil)
	f.end()
	return driver.ErrMayNotBeKept
}

func (f *streamed) abandon() {
	f.send(map[string]any{"abort": "the program gave the file up"}, nil)
	f.end()
}

// end lets go of what the file holds, and of the frame the download was
// asked for in.
func (f *streamed) end() {
	f.endOnce.Do(func() {
		f.goneOnce.Do(func() { close(f.gone) })
		f.port.Set("onmessage", js.Null())
		f.port.Call("close")
		f.heard.Release()
		// The browser's downloads have the file from the frame on: the
		// frame goes, whatever the download does.
		if f.letGo != nil {
			f.letGo()
		}
	})
}

// Name implements [driver.SavedFile].
func (f *streamed) Name() string { return f.name }

// Place implements [driver.SavedFile]: a browser tells no place.
func (f *streamed) Place() string { return "" }

// Made implements [driver.SavedFile].
func (f *streamed) Made() bool { return true }

// InMemory implements [driver.MemorySaved]: the browser writes the download
// to the disk as it comes.
func (f *streamed) InMemory() bool { return false }
