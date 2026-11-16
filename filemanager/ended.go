package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// OpEnd is how one of a window's operations ended, as Options.Ended
// hears it. An operation that waits for an answer, such as whether to
// replace a file, has not ended.
type OpEnd struct {
	// Kind is what the operation did.
	Kind OpKind
	// FS is the ID of the file system it worked on, and Into the folder
	// it wrote into: where a copy or a move went, the folder an archive
	// was extracted into, or the folder a zip or a new folder was made
	// in. Into is empty for an operation that wrote nothing, such as a
	// delete.
	FS, Into string
	// Done is how many items it copied, moved, made, extracted, trashed
	// or deleted, as its notice counts them: a folder is one item, with
	// all it holds.
	Done int
	// Written is how many files it wrote whole and left in place, each
	// file in a folder counted, also where the operation stopped short.
	Written int
	// Failed is how many items it could not do: the one it stopped at,
	// since an operation stops at its first failure. It is zero for an
	// operation that went well, or that the user stopped.
	Failed int
	// Warnings are the items it wrote whole but could not finish as the
	// originals were: the file system refused to set their permissions,
	// or when they last changed. Each item is listed once.
	Warnings []Warning
	// Err is why the operation stopped short, and nil when it went well.
	// It matches context.Canceled where the user stopped it.
	Err error
}

// Warning is an item an operation wrote whole, whose permissions or time
// the file system refused to set. The item stays, without them.
type Warning struct {
	// Path is where the item is.
	Path string
	// Mode says the file system refused the item's permissions, and Time
	// the time it last changed.
	Mode, Time bool
	// Private says others, beyond the owner and the group, could not
	// read the original. Without its permissions, they may be able to
	// read the item.
	Private bool
	// Err is the file system's first refusal for the item.
	Err error
}

// refused reports whether err, from a Chmod or a Chtimes, is a file
// system refusing to keep permissions or times at all: "operation
// unsupported" or "permission denied".
func refused(err error) bool {
	return errors.Is(err, errors.ErrUnsupported) || errors.Is(err, fs.ErrPermission)
}

// closedToOthers reports whether others than its owner and its group could
// not read an item of mode, so that without it they may. A mode of 0
// says nothing.
func closedToOthers(mode fs.FileMode) bool {
	perm := mode.Perm()
	return perm != 0 && perm&0o004 == 0
}

// stamped takes what a Chmod, with mode set, or a Chtimes of an item
// returned. A refusal goes into w, and stamped returns nil, so the item
// stays. Any other failure it returns.
func stamped(w *Warning, err error, mode bool) error {
	if err == nil || !refused(err) {
		return err
	}
	if mode {
		w.Mode = true
	} else {
		w.Time = true
	}
	if w.Err == nil {
		w.Err = err
	}
	return nil
}

// warn keeps w, once the item it is about is in place at its path. A
// second warning for the same item joins the first.
func (r *runner) warn(w Warning) {
	if !w.Mode && !w.Time {
		return
	}
	if i, ok := r.warned[w.Path]; ok {
		k := &r.rec.warnings[i]
		k.Mode, k.Time, k.Private = k.Mode || w.Mode, k.Time || w.Time, k.Private || w.Private
		if k.Err == nil {
			k.Err = w.Err
		}
		return
	}
	if r.warned == nil {
		r.warned = map[string]int{}
	}
	r.warned[w.Path] = len(r.rec.warnings)
	r.rec.warnings = append(r.rec.warnings, w)
}

// unwarn forgets the warnings of the items at path, and under it, that
// the operation has taken away again.
func (r *runner) unwarn(path string) {
	fsys := r.env.fs
	ps := fsys.Paths()
	kept := r.rec.warnings[:0]
	for _, w := range r.rec.warnings {
		if ps.inside(w.Path, path) {
			if _, err := fsys.Lstat(w.Path); errors.Is(err, fs.ErrNotExist) {
				continue
			}
		}
		kept = append(kept, w)
	}
	clear(r.rec.warnings[len(kept):])
	r.rec.warnings = kept
	clear(r.warned)
	for i, w := range kept {
		r.warned[w.Path] = i
	}
}

// renamed is err, an error about the part file part, said of the file
// dst it became: the user never sees the part file, and it is gone.
func renamed(err error, part, dst string) error {
	if err == nil || part == dst {
		return err
	}
	return &renamedErr{msg: strings.ReplaceAll(err.Error(), part, dst), err: err}
}

// renamedErr is an error said of another path. It still matches what
// the error it wraps matches.
type renamedErr struct {
	msg string
	err error
}

func (e *renamedErr) Error() string { return e.msg }
func (e *renamedErr) Unwrap() error { return e.err }

// ErrWindowClosed is what OpEnd.Err matches when the window closed
// before the operation finished, and so stopped it.
var ErrWindowClosed = errors.New("the window closed")

// endOf is how job j ended on the file system of ID fsID, whose paths
// are written as ps, having done rec, with err. closed says the window
// closed before it finished.
func endOf(j job, fsID string, ps PathStyle, rec record, err error, closed bool) OpEnd {
	e := OpEnd{Kind: j.kind, FS: fsID, Done: doneCount(rec), Written: rec.written, Warnings: rec.warnings, Err: err}
	switch j.kind {
	case OpCopy, OpMove, OpNewFolder, OpZip:
		e.Into = j.dest
	case OpExtract:
		e.Into = ps.Join(j.dest, j.name)
	case OpRename:
		if len(j.srcs) > 0 {
			e.Into = ps.Dir(j.srcs[0])
		}
	case OpTrash, OpDelete, OpUndo:
		// They write nothing new, or put items back where they were.
	}
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
	case errors.Is(err, errStopped):
		// The user stopped it at a question.
		e.Err = fmt.Errorf("%w: %w", context.Canceled, err)
	default:
		e.Failed = 1
	}
	if closed {
		if e.Err == nil {
			e.Err = ErrWindowClosed
		} else {
			e.Err = fmt.Errorf("%w: %w", ErrWindowClosed, e.Err)
		}
	}
	return e
}

// doneCount is how many items rec says an operation did, as its notice
// counts them.
func doneCount(rec record) int { return len(rec.steps) + rec.replaced }

// warnNotice is the one notice for the warnings of operation j, which
// did rec and stopped short with err, or went well, with the file
// system's answer as its body. One that went well counts the items it
// did, as its notice would; one that stopped short counts the files it
// wrote, as the items it was given may not be done.
func (a *app) warnNotice(j job, rec record, err error) Notice {
	n := doneCount(rec)
	what := plural(n, "item")
	if n == 1 && len(rec.landed) == 1 {
		what = rec.landed[0]
	}
	if err != nil {
		n = rec.written
		what = plural(n, "file")
		if n == 0 {
			// Only folders it made, whose time was refused.
			n = len(rec.warnings)
			what = plural(n, "item")
		}
	}
	into := endOf(j, "", a.ps, rec, nil, false).Into
	if into == "" {
		into = a.ps.Dir(rec.warnings[0].Path)
	}
	w := warnBody(rec.warnings)
	w.Title = "Wrote " + what + " to " + a.ps.placeName(into) + ", but couldn't " + lost(n, rec.warnings, "set")
	if rec.replaced > 0 {
		w.Body += " " + plural(rec.replaced, "file") + " replaced, which undo cannot bring back."
	}
	return w
}

// warnBody is a warning notice with no title yet, whose body is the file
// system's first answer of ws, and says when others may now read what
// only its owner could.
func warnBody(ws []Warning) Notice {
	n := Notice{Kind: "warning"}
	private := false
	for _, w := range ws {
		if w.Err != nil && n.Body == "" {
			n.Body = upperFirst(w.Err.Error()) + "."
		}
		private = private || w.Private
	}
	if private {
		n.Body = strings.TrimSpace(n.Body + " Others couldn't read the original, and may be able to read it now.")
	}
	return n
}

// lost says what the file system refused for n items, as ws say, verb
// being what the window tried to do: "set their permissions", say.
func lost(n int, ws []Warning, verb string) string {
	mode, time := false, false
	for _, w := range ws {
		mode, time = mode || w.Mode, time || w.Time
	}
	their, they := "their", "they"
	if n == 1 {
		their, they = "its", "it"
	}
	switch {
	case mode && time:
		return verb + " " + their + " permissions or the time " + they + " last changed"
	case mode:
		return verb + " " + their + " permissions"
	}
	return verb + " the time " + they + " last changed"
}

// upperFirst is s with its first letter in capitals.
func upperFirst(s string) string {
	for i, c := range s {
		return strings.ToUpper(string(c)) + s[i+len(string(c)):]
	}
	return s
}
