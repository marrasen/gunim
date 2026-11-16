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
	// Private says only its owner could read the original. Without its
	// permissions, others may be able to read the item.
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

// ownerOnly reports whether only the owner may read an item of mode.
func ownerOnly(mode fs.FileMode) bool {
	perm := mode.Perm()
	return perm != 0 && perm&0o077 == 0
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
	for i := range r.rec.warnings {
		k := &r.rec.warnings[i]
		if k.Path != w.Path {
			continue
		}
		k.Mode, k.Time, k.Private = k.Mode || w.Mode, k.Time || w.Time, k.Private || w.Private
		if k.Err == nil {
			k.Err = w.Err
		}
		return
	}
	r.rec.warnings = append(r.rec.warnings, w)
}

// opEnd is how job j ended, having done rec, with err.
func (a *app) opEnd(j job, rec record, err error) OpEnd {
	e := OpEnd{Kind: j.kind, FS: a.fs.ID(), Done: len(rec.steps) + rec.replaced, Warnings: rec.warnings, Err: err}
	switch j.kind {
	case OpCopy, OpMove, OpNewFolder, OpZip:
		e.Into = j.dest
	case OpExtract:
		e.Into = a.ps.Join(j.dest, j.name)
	case OpRename:
		if len(j.srcs) > 0 {
			e.Into = a.ps.Dir(j.srcs[0])
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
	return e
}

// warnNotice is the one notice for the warnings ws of an operation that
// wrote into the folder into, with the file system's answer as its body.
func (a *app) warnNotice(into string, ws []Warning) Notice {
	mode, time, private := false, false, false
	for _, w := range ws {
		mode, time, private = mode || w.Mode, time || w.Time, private || w.Private
	}
	what, their := plural(len(ws), "item"), "their"
	if len(ws) == 1 {
		what, their = a.ps.Base(ws[0].Path), "its"
	}
	var lost string
	switch {
	case mode && time:
		lost = "set " + their + " permissions or the time " + pronoun(len(ws)) + " last changed"
	case mode:
		lost = "set " + their + " permissions"
	default:
		lost = "set the time " + pronoun(len(ws)) + " last changed"
	}
	n := Notice{Title: "Wrote " + what + " to " + a.ps.placeName(into) + ", but couldn't " + lost, Kind: "warning"}
	for _, w := range ws {
		if w.Err != nil {
			n.Body = w.Err.Error()
			break
		}
	}
	if private {
		n.Body = strings.TrimSpace(n.Body + " Only the owner could read the original, and others may now be able to read the copy.")
	}
	return n
}

// pronoun is "it" for one item, and "they" for more.
func pronoun(n int) string {
	if n == 1 {
		return "it"
	}
	return "they"
}
