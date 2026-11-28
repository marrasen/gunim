package filemanager

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/zipcrypt"
)

// Files to and from the user's computer, for a window in a browser,
// which has none of the computer's folders: Upload puts files the user
// picks, or drops on the window, into a folder, and Download saves items
// where the user says, a folder or several items as one zip. Each is an
// operation of the window, with its card, and reads and writes a piece
// at a time, so no file is ever whole in memory.

// UploadDropped says files of the user's computer were dropped on the
// folder Into: Token names them, as the view put them aside.
type UploadDropped struct {
	Into  string
	Token int
}

func init() { gunim.RegisterType[UploadDropped]("files.uploaddropped") }

// inMemoryMost is the largest download a browser that holds it in memory
// until it is whole is given.
const inMemoryMost = 512 << 20

// dropped holds the files of drops, as the view puts them aside, until
// the window takes them: what crosses to the window is a number, as a
// file in a browser can cross nothing else. Those not taken within
// droppedFor are let go of.
var dropped struct {
	mu   sync.Mutex
	next int
	held map[int][]*input.Stream
}

const droppedFor = time.Minute

// holdDropped puts streams aside, and returns the number they are taken
// by.
func holdDropped(streams []*input.Stream) int {
	dropped.mu.Lock()
	defer dropped.mu.Unlock()
	if dropped.held == nil {
		dropped.held = map[int][]*input.Stream{}
	}
	dropped.next++
	n := dropped.next
	dropped.held[n] = streams
	time.AfterFunc(droppedFor, func() { closeStreams(takeDropped(n)) })
	return n
}

// takeDropped takes the streams put aside as n, or none where they were
// taken or let go of.
func takeDropped(n int) []*input.Stream {
	dropped.mu.Lock()
	defer dropped.mu.Unlock()
	s := dropped.held[n]
	delete(dropped.held, n)
	return s
}

// closeStreams closes streams not to be read.
func closeStreams(streams []*input.Stream) {
	for _, s := range streams {
		_ = s.Close()
	}
}

// handleExchange takes the intents of uploads and downloads.
func (a *app) handleExchange(in gunim.Intent) bool {
	switch v := in.(type) {
	case UploadDropped:
		a.uploadInto(v.Into, takeDropped(v.Token))
	case Command:
		switch v.Name {
		case CmdUpload:
			if !a.readOnlyHere(v.Name) {
				a.chooseUploads()
			}
		case CmdDownload:
			a.download(a.selectedPaths())
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// chooseUploads asks the user for files, and uploads them into the
// folder showing. It asks at once, in the click, which a browser shows
// its picker in only.
func (a *app) chooseUploads() {
	into, c := a.nav.path, a.c.c
	if into == "" || !a.c.on {
		return
	}
	go func() {
		streams, err := c.ChooseStreams(a.ctx, driver.ChooseOptions{Title: "Upload Files", Multiple: true})
		a.post(func() {
			switch {
			case errors.Is(err, driver.ErrNoGesture):
				a.fail("The browser showed no files to choose. Click Upload files again.")
			case errors.Is(err, context.Canceled) || errors.Is(err, gunim.ErrWindowClosed):
				closeStreams(streams)
			case err != nil:
				closeStreams(streams)
				a.fail("Choosing files to upload: " + err.Error())
			default:
				a.uploadInto(into, streams)
			}
		})
	}()
}

// uploadInto writes streams into the folder into, as an operation of the
// window, asking about a name that is taken. Each is written to a part
// file beside it first, so one cut short never looks whole.
func (a *app) uploadInto(into string, streams []*input.Stream) {
	if len(streams) == 0 {
		return
	}
	switch {
	case a.shell.ReadOnly:
		a.refuseReadOnly()
	case a.aboveTop(into):
		a.refuseAbove(into)
	case a.archiveOf(into, true) != "":
		a.refuseArchive(a.archiveOf(into, true))
	default:
		a.startUploadInto(into, streams)
		return
	}
	closeStreams(streams)
}

// startUploadInto runs the upload of streams into the folder into.
func (a *app) startUploadInto(into string, streams []*input.Stream) {
	names := make([]string, len(streams))
	for i, s := range streams {
		names[i] = s.Name
	}
	what, where := whatIn(a.ps, names), a.placeName(into)
	id, ctx, _ := a.newOp("Uploading "+what+" to "+where, OpCopy)
	r := a.ops.running[id]
	r.pause = &pauseGate{}
	r.from, r.to = "This computer", where
	ask := func(ctx context.Context, c clash) (answer, error) { return a.askClash(ctx, id, c) }
	e := env{fs: a.fs, ask: ask, pause: r.pause, report: func(p progress) { a.post(func() { a.progressed(id, p) }) }}
	a.ops.wg.Go(func() {
		run := &runner{ctx: ctx, env: e, rec: record{kind: OpCopy}}
		landed, folders, bad, err := run.uploadAll(streams, into)
		run.tell(true)
		stopped := ctx.Err() != nil
		a.post(func() { a.uploadEnded(id, into, what, landed, folders, bad, err, stopped) })
	})
}

// uploadEnded takes the end of upload id into the folder into, which
// put the files landed there and left out skipped, those that were no
// files, as folders are.
func (a *app) uploadEnded(id int, into, what string, landed, folders, bad []string, err error, stopped bool) {
	r, ok := a.ops.running[id]
	if !ok {
		return
	}
	delete(a.ops.running, id)
	r.cancel()
	a.dropDialogs(id)
	n := Notice{Title: "Uploaded " + what + " to " + a.placeName(into), Kind: "success"}
	var left []string
	if len(folders) > 0 {
		left = append(left, "Left out "+whatIn(a.ps, folders)+": only files can be uploaded, not folders.")
	}
	if len(bad) > 0 {
		left = append(left, "Left out "+strings.Join(bad, ", ")+".")
	}
	skipped := len(left) > 0
	if skipped {
		n.Body, n.Kind = strings.Join(left, " "), "warning"
	}
	switch {
	case stopped || errors.Is(err, context.Canceled) || errors.Is(err, errStopped):
		a.endCard(r, false, Notice{})
		a.patch(Notice{Title: "Stopped: " + r.title, Body: plural(len(landed), "file") + " uploaded before it stopped.",
			Kind: "warning"})
	case err != nil:
		a.endCard(r, false, Notice{})
		a.showError(ErrorBox{Title: "The upload stopped", Body: err.Error()})
	case !a.endCard(r, !skipped, n) || skipped:
		a.patch(n)
	}
	if a.ps.Same(into, a.nav.path) {
		a.nav.pick = nil
		for _, p := range landed {
			a.nav.pick = append(a.nav.pick, a.ps.Base(p))
		}
	}
	a.relist()
	a.touched(job{dest: into})
}

// uploadAll writes each of streams into the folder dest, and returns the
// paths written, the names of those that were folders, and those left out
// for their names, with why. It closes every stream.
func (r *runner) uploadAll(streams []*input.Stream, dest string) (landed, folders, badNames []string, err error) {
	defer closeStreams(streams)
	r.p.itemsTotal = len(streams)
	for _, s := range streams {
		if s.Size > 0 {
			r.p.bytesTotal += s.Size
		}
	}
	ps := r.env.fs.Paths()
	for _, s := range streams {
		if err := r.ctx.Err(); err != nil {
			return landed, folders, badNames, err
		}
		var to string
		if nerr := checkName(ps, s.Name); nerr != nil {
			badNames = append(badNames, s.Name+" ("+nerr.Error()+")")
		} else {
			to, err = r.uploadOne(s, dest)
		}
		switch {
		case errors.Is(err, input.ErrFolder):
			folders, err = append(folders, s.Name), nil
		case err != nil:
			return landed, folders, badNames, err
		case to != "":
			landed = append(landed, to)
		}
		_ = s.Close()
		r.p.items++
		r.tell(false)
	}
	return landed, folders, badNames, nil
}

// uploadOne writes s into the folder dest, and returns where it went, or
// "" where the user skipped it. Its first bytes are read before anything
// is asked or made, so a folder dropped is found to be one first, and
// fails with input.ErrFolder.
func (r *runner) uploadOne(s *input.Stream, dest string) (string, error) {
	fsys := r.env.fs
	ps := fsys.Paths()
	to := ps.Join(dest, s.Name)
	r.p.current = to
	r.tell(false)
	first := make([]byte, 64<<10)
	n, rerr := io.ReadFull(s, first)
	switch {
	case errors.Is(rerr, io.EOF), errors.Is(rerr, io.ErrUnexpectedEOF):
		// All of it is in hand.
	case errors.Is(rerr, input.ErrFolder):
		return "", input.ErrFolder
	case rerr != nil:
		return "", fmt.Errorf("reading %s: %w", s.Name, rerr)
	}
	from := io.MultiReader(bytes.NewReader(first[:n]), s)
	if rerr != nil {
		from = bytes.NewReader(first[:n])
	}
	replace := false
	if old, err := fsys.Lstat(to); err == nil {
		ans, err := r.decide(clash{dst: to, sameKind: !old.IsDir(), from: ItemFacts(max(s.Size, 0), false, time.Time{})})
		if err != nil {
			return "", err
		}
		switch ans.choice {
		case choiceSkip:
			return "", nil
		case choiceKeepBoth:
			if to, err = freeName(fsys, to); err != nil {
				return "", err
			}
		default:
			replace = true
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("writing %s: %w", ps.Show(to), err)
	}
	out, part, err := createTemp(fsys, dest, ".files-*.part")
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", ps.Show(to), err)
	}
	// What failed takes its part file with it, where the file system
	// still answers.
	if err := r.pourAs(out, from, s.Name, ps.Show(to), true); err != nil {
		return "", errors.Join(err, out.Close(), fsys.Remove(part))
	}
	if err := out.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("writing %s: %w", ps.Show(to), err), fsys.Remove(part))
	}
	if !replace {
		// Made meanwhile, it is not written over.
		if _, err := fsys.Lstat(to); err == nil {
			return "", errors.Join(fmt.Errorf("%s already exists", ps.Show(to)), fsys.Remove(part))
		}
	}
	if err := fsys.Rename(part, to); err != nil {
		return "", errors.Join(fmt.Errorf("writing %s: %w", ps.Show(to), err), fsys.Remove(part))
	}
	return to, nil
}

// decide asks the user about a clash, unless an answer for all of them
// was given.
func (r *runner) decide(c clash) (answer, error) {
	if r.all != nil {
		return *r.all, nil
	}
	if r.env.ask == nil {
		return answer{choice: choiceKeepBoth}, nil
	}
	ans, err := r.env.ask(r.ctx, c)
	if err != nil {
		return answer{}, err
	}
	if ans.all {
		r.all = &ans
	}
	return ans, nil
}

// download saves the items at paths on the user's computer: one file as
// it is, and a folder, or several items, as one zip. It asks where at
// once, in the click, which a browser shows its dialog in only.
func (a *app) download(paths []string) {
	if len(paths) == 0 || !a.c.on {
		return
	}
	paths = slices.Clone(paths)
	es := a.selectedEntries()
	// A file goes as it is, an archive too, though the window walks into
	// it.
	one := len(paths) == 1 && len(es) == 1 && !es[0].Dir
	name := a.ps.Base(paths[0])
	if !one {
		name = zipName(a.ps, paths)
	}
	c := a.c.c
	go func() {
		f, err := c.SaveStream(a.ctx, driver.SaveOptions{Title: "Download", Name: name})
		a.post(func() {
			switch {
			case err == nil && f == nil:
			case errors.Is(err, context.Canceled) || errors.Is(err, gunim.ErrWindowClosed):
			case err != nil:
				a.fail("Downloading " + name + ": " + err.Error())
			default:
				a.startDownload(f, paths, one)
			}
		})
	}()
}

// startDownload writes the items at paths into f, which the user chose:
// the one file itself where one says so, and otherwise a zip of them.
func (a *app) startDownload(f driver.SavedFile, paths []string, one bool) {
	what, from := a.what(paths), a.placeName(a.ps.Dir(paths[0]))
	id, ctx, _ := a.newOp("Downloading "+what, OpCopy)
	r := a.ops.running[id]
	r.pause = &pauseGate{}
	r.from, r.to = from, "This computer"
	e := env{fs: a.fs, pause: r.pause, report: func(p progress) { a.post(func() { a.progressed(id, p) }) }}
	held := driver.HeldInMemory(f)
	a.ops.wg.Go(func() {
		run := &runner{ctx: ctx, env: e, rec: record{kind: OpCopy}}
		err := run.downloadTo(f, paths, one, held)
		run.tell(true)
		stopped := ctx.Err() != nil
		a.post(func() { a.downloadEnded(id, f.Name(), err, stopped) })
	})
}

// tooLarge is a download larger than a browser that holds it in memory
// is given: what it is, and how large.
type tooLarge struct{ what string }

func (t *tooLarge) Error() string { return t.what }

// downloadTo writes the items at paths into f, and ends f: closed once
// whole, and taken away where it is not. held says f is held in memory
// until it is whole, which only so much is given.
func (r *runner) downloadTo(f driver.SavedFile, paths []string, one, held bool) error {
	err := r.downloadAll(f, paths, one, held)
	if err != nil {
		return errors.Join(err, f.Remove())
	}
	if err := f.Close(); err != nil && !errors.Is(err, driver.ErrMayNotBeKept) {
		return err
	}
	return nil
}

func (r *runner) downloadAll(f io.Writer, paths []string, one, held bool) error {
	fsys := r.env.fs
	ps := fsys.Paths()
	if one {
		info, err := fsys.Stat(paths[0])
		if err != nil {
			return fmt.Errorf("reading %s: %w", ps.Show(paths[0]), err)
		}
		if held && info.Size() > inMemoryMost {
			return &tooLarge{ps.Base(paths[0]) + " is " + humanBytes(info.Size())}
		}
		r.p.itemsTotal, r.p.bytesTotal, r.p.current = 1, info.Size(), paths[0]
		in, err := fsys.Open(paths[0])
		if err != nil {
			return fmt.Errorf("reading %s: %w", ps.Show(paths[0]), err)
		}
		err = r.pourAs(f, in, ps.Show(paths[0]), "the download", true)
		if cerr := in.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("reading %s: %w", ps.Show(paths[0]), cerr)
		}
		r.p.items = 1
		return err
	}
	if err := r.planBytes(paths); err != nil {
		return err
	}
	if held && r.p.bytesTotal > inMemoryMost {
		return &tooLarge{whatIn(ps, paths) + " come to " + humanBytes(r.p.bytesTotal)}
	}
	zw := zipcrypt.NewWriter(zip.NewWriter(f), "")
	for _, src := range paths {
		if err := r.zipTree(zw, src, ""); err != nil {
			return errors.Join(err, zw.Close())
		}
	}
	return zw.Close()
}

// downloadEnded takes the end of download id, of the file the browser
// keeps as name.
func (a *app) downloadEnded(id int, name string, err error, stopped bool) {
	r, ok := a.ops.running[id]
	if !ok {
		return
	}
	delete(a.ops.running, id)
	r.cancel()
	a.dropDialogs(id)
	var large *tooLarge
	switch {
	case stopped || errors.Is(err, context.Canceled):
		a.endCard(r, false, Notice{})
		a.patch(Notice{Title: "Stopped: " + r.title, Kind: "warning"})
	case errors.As(err, &large):
		a.endCard(r, false, Notice{})
		a.showError(ErrorBox{Title: "Too large to download here",
			Body: large.what + ". This browser keeps a download in memory until it is whole, and takes at most " +
				humanBytes(inMemoryMost) + " so. Chrome and Edge save a download as it comes."})
	case err != nil:
		a.endCard(r, false, Notice{})
		a.showError(ErrorBox{Title: "The download stopped", Body: err.Error()})
	default:
		n := Notice{Title: "Downloaded " + name, Kind: "success"}
		if !a.endCard(r, true, n) {
			a.patch(n)
		}
	}
}
