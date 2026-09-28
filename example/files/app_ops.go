package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/marrasen/gunim"
)

// showAfter is how long an operation runs before the progress panel
// shows it, so a quick rename does not flash the panel.
const showAfter = 250 * time.Millisecond

// opsState is the running operations, what finished ones did, the
// app's own clipboard of paths, and the dialogs waiting to be shown.
type opsState struct {
	next    int
	running map[int]*opRun
	// cheering holds the operations that went well and show so a moment
	// in the panel.
	cheering map[int]*opRun
	// records holds what finished operations did, by their ID, and undo
	// their IDs in the order Ctrl+Z takes them back.
	records map[int]*finished
	undo    []int
	clip    []string
	cut     bool
	// limit holds copies to that many bytes a second, for the demo.
	limit float64
	// dialogs are the dialogs to show, the first showing.
	dialogs []*dialog
	shown   int
	tokens  int
	// wg counts the operations' goroutines.
	wg sync.WaitGroup
}

func (o *opsState) init() {
	o.running = map[int]*opRun{}
	o.cheering = map[int]*opRun{}
	o.records = map[int]*finished{}
}

// opRun is an operation running.
type opRun struct {
	id      int
	title   string
	kind    OpKind
	cancel  context.CancelFunc
	visible bool
	last    progress
	meter   speedometer
	// undoes is the finished operation an undo reverses.
	undoes int
}

// finished is what a finished operation did, and how to say it.
type finished struct {
	title string
	rec   record
}

// dialog is a dialog to show, and what to do with its answer.
type dialog struct {
	id     gunim.ID
	view   string
	state  any
	op     int
	answer func(in gunim.Intent)
}

// handleOps takes the intents about file operations and their dialogs.
func (a *app) handleOps(in gunim.Intent) bool {
	switch v := in.(type) {
	case CancelOp:
		if r, ok := a.ops.running[v.ID]; ok {
			r.cancel()
		}
	case UndoOp:
		a.undo(v.ID)
	case ClashAnswered, Confirmed, Prompted, DialogClosed:
		a.answered(in)
	case Command:
		return a.opsCommand(v.Name)
	default:
		return false
	}
	return true
}

func (a *app) opsCommand(name string) bool {
	here := a.nav.path
	switch name {
	case CmdCopy, CmdCut:
		paths := a.selectedPaths()
		if len(paths) == 0 {
			return true
		}
		a.ops.clip, a.ops.cut = paths, name == CmdCut
		a.clipChanged()
		verb := "copy"
		if a.ops.cut {
			verb = "move"
		}
		a.patch(Notice{Title: plural(len(paths), "item") + " ready to " + verb, Body: "Go to a folder and paste with Ctrl+V.",
			Kind: "info"})
	case CmdPaste:
		if len(a.ops.clip) == 0 || here == "" {
			return true
		}
		srcs := slices.Clone(a.ops.clip)
		if a.ops.cut {
			a.ops.clip, a.ops.cut = nil, false
			a.clipChanged()
			a.startOp(job{kind: OpMove, srcs: srcs, dest: here}, "Moving "+what(srcs)+" to "+placeName(here))
		} else {
			a.startOp(job{kind: OpCopy, srcs: srcs, dest: here}, "Copying "+what(srcs)+" to "+placeName(here))
		}
	case CmdTrash:
		if paths := a.selectedPaths(); len(paths) > 0 {
			a.startOp(job{kind: OpTrash, srcs: paths}, "Moving "+what(paths)+" to the trash")
		}
	case CmdDelete:
		paths := a.selectedPaths()
		if len(paths) == 0 {
			return true
		}
		a.confirm(Confirm{Title: "Delete " + what(paths) + " for good?",
			Body: "They will not go to the trash, and this cannot be undone.", OK: "Delete"},
			func() { a.startOp(job{kind: OpDelete, srcs: paths}, "Deleting "+what(paths)) })
	case CmdRename:
		es := a.selectedEntries()
		if len(es) != 1 {
			return true
		}
		src := filepath.Join(here, es[0].Name)
		stem := utf8.RuneCountInString(es[0].Name)
		if !es[0].Dir {
			stem = utf8.RuneCountInString(strings.TrimSuffix(es[0].Name, filepath.Ext(es[0].Name)))
		}
		a.prompt(Prompt{Title: "Rename", Text: es[0].Name, OK: "Rename", Stem: stem}, func(name string) {
			a.nav.pick = name
			a.startOp(job{kind: OpRename, srcs: []string{src}, name: name}, "Renaming "+es[0].Name)
		})
	case CmdNewFolder:
		if here == "" {
			return true
		}
		name, err := freeName(filepath.Join(here, "New folder"))
		if err != nil {
			a.fail("Looking for a free name: " + err.Error())
			return true
		}
		base := filepath.Base(name)
		a.prompt(Prompt{Title: "New folder", Text: base, OK: "Make", Stem: utf8.RuneCountInString(base)}, func(name string) {
			a.nav.pick = name
			a.startOp(job{kind: OpNewFolder, dest: here, name: name}, "Making "+name)
		})
	case CmdUndo:
		if n := len(a.ops.undo); n > 0 {
			a.undo(a.ops.undo[n-1])
		}
	default:
		return false
	}
	return true
}

// what says how many items paths are, or names the one.
func what(paths []string) string {
	if len(paths) == 1 {
		return filepath.Base(paths[0])
	}
	return plural(len(paths), "item")
}

// startOp runs j in the background, with its progress in the panel once
// it has run a moment.
func (a *app) startOp(j job, title string) {
	a.ops.next++
	id := a.ops.next
	ctx, cancel := context.WithCancel(a.ctx)
	r := &opRun{id: id, title: title, kind: j.kind, cancel: cancel}
	a.ops.running[id] = r
	time.AfterFunc(showAfter, func() {
		a.post(func() {
			if r, ok := a.ops.running[id]; ok {
				r.visible = true
				a.publishOps()
			}
		})
	})
	e := env{
		trash:  a.trash,
		ask:    func(ctx context.Context, c clash) (answer, error) { return a.askClash(ctx, id, c) },
		report: func(p progress) { a.post(func() { a.progressed(id, p) }) },
		limit:  a.ops.limit,
	}
	a.ops.wg.Go(func() {
		rec, err := runJob(ctx, j, e)
		a.post(func() { a.finish(id, j, rec, err) })
	})
}

// cheer keeps finished operation id in the panel while it shows it went
// well.
func (a *app) cheer(id int, r *opRun) {
	a.ops.cheering[id] = r
	time.AfterFunc(cheerFor, func() {
		a.post(func() {
			delete(a.ops.cheering, id)
			a.publishOps()
		})
	})
}

// progressed takes how far operation id has got.
func (a *app) progressed(id int, p progress) {
	r, ok := a.ops.running[id]
	if !ok {
		return
	}
	sampled := r.meter.add(time.Now(), p.bytes)
	r.last = p
	if r.visible {
		a.patch(r.tick())
		if sampled && p.bytesTotal > 0 {
			a.patch(OpSpeed{ID: id, Rate: r.meter.rate, Left: r.meter.left(p.bytes, p.bytesTotal), File: p.current})
		}
	}
}

// tick is how the panel shows r now.
func (r *opRun) tick() OpTick {
	p := r.last
	t := OpTick{ID: r.id}
	switch {
	case p.bytesTotal > 0:
		t.Done = float32(float64(p.bytes) / float64(p.bytesTotal))
		t.Detail = humanBytes(p.bytes) + " of " + humanBytes(p.bytesTotal)
		if speed := r.meter.smooth; speed > 0 {
			t.Detail += "  ·  " + humanBytes(int64(speed)) + "/s"
			if left := r.meter.left(p.bytes, p.bytesTotal); left >= 1 {
				t.Detail += fmt.Sprintf("  ·  %s left", (time.Duration(left) * time.Second).Round(time.Second))
			}
		}
	case p.itemsTotal > 0:
		t.Done = float32(p.items) / float32(p.itemsTotal)
		t.Detail = count(p.items) + " of " + plural(p.itemsTotal, "item")
	default:
		t.Unknown = true
		t.Detail = "Counting…"
	}
	if p.current != "" {
		t.Detail = filepath.Base(p.current) + "  ·  " + t.Detail
	}
	return t
}

func (a *app) publishOps() {
	var s Ops
	ids := make([]int, 0, len(a.ops.running))
	for id := range a.ops.running {
		ids = append(ids, id)
	}
	for id := range a.ops.cheering {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		r, ok := a.ops.running[id]
		if !ok {
			r = a.ops.cheering[id]
		}
		if !r.visible {
			continue
		}
		t := r.tick()
		if !ok {
			t = OpTick{ID: id, Done: 1, Detail: "Done"}
		}
		s.Ops = append(s.Ops, OpView{ID: id, Title: r.title, Done: t.Done, Unknown: t.Unknown, Detail: t.Detail})
	}
	a.patch(s)
}

// finish takes the end of operation id.
func (a *app) finish(id int, j job, rec record, err error) {
	r, ok := a.ops.running[id]
	if !ok {
		return
	}
	delete(a.ops.running, id)
	r.cancel()
	a.dropDialogs(id)
	if j.kind == OpUndo {
		a.undone(r.undoes, rec)
	}
	if r.visible {
		a.patch(OpDone{ID: id, OK: err == nil})
		if err == nil {
			r.title = doneTitle(j, rec)
			a.cheer(id, r)
		}
		a.publishOps()
	}
	done := plural(len(rec.steps), "item")
	keep := undoable(rec)
	if keep {
		a.ops.records[id] = &finished{title: r.title, rec: rec}
		a.ops.undo = append(a.ops.undo, id)
	}
	undo := 0
	if keep {
		undo = id
	}
	switch {
	case err == nil && j.kind == OpUndo:
		a.patch(Notice{Title: "Undone", Body: r.title, Kind: "success"})
	case err == nil:
		n := Notice{Title: doneTitle(j, rec), Undo: undo, Kind: "success"}
		switch {
		case rec.replaced > 0:
			n.Body = plural(rec.replaced, "file") + " replaced, which undo cannot bring back."
		case j.kind == OpTrash && !restorable(rec):
			// Undo would only say so; the toast says it now.
			n.Undo = 0
			n.Body = "Restore from the system's trash to bring them back."
		}
		a.patch(n)
	case errors.Is(err, context.Canceled):
		a.patch(Notice{Title: "Stopped: " + r.title, Body: done + " done before it stopped.", Undo: undo, Kind: "warning"})
	case errors.Is(err, errStopped):
		a.patch(Notice{Title: "Stopped: " + r.title, Body: done + " done before it stopped.", Undo: undo, Kind: "warning"})
	default:
		body := err.Error()
		if len(rec.steps) > 0 {
			body += "\n\n" + done + " done before this."
		}
		a.showError(ErrorBox{Title: failedTitle(j), Body: body})
		if keep {
			a.patch(Notice{Title: "Partly done: " + r.title, Undo: undo, Kind: "warning"})
		}
	}
	if j.kind == OpRename || j.kind == OpNewFolder {
		if err != nil {
			a.nav.pick = ""
		}
	}
	a.touched(j)
	a.relist()
}

// restorable reports whether the app can take what a trash record holds
// back out of the trash.
func restorable(rec record) bool {
	for _, s := range rec.steps {
		if s.to == "" {
			return false
		}
	}
	return true
}

// doneTitle says what a finished job did.
func doneTitle(j job, rec record) string {
	n := len(rec.steps) + rec.replaced
	switch j.kind {
	case OpCopy:
		return "Copied " + plural(n, "item") + " to " + placeName(j.dest)
	case OpMove:
		return "Moved " + plural(n, "item") + " to " + placeName(j.dest)
	case OpTrash:
		return "Moved " + plural(n, "item") + " to the trash"
	case OpDelete:
		return "Deleted " + plural(n, "item") + " for good"
	case OpRename:
		return "Renamed to " + j.name
	case OpNewFolder:
		return "Made " + j.name
	case OpUndo:
		return "Undone"
	}
	return "Done"
}

// failedTitle names a job that failed.
func failedTitle(j job) string {
	return map[OpKind]string{
		OpCopy: "The copy stopped", OpMove: "The move stopped", OpTrash: "Moving to the trash stopped",
		OpDelete: "Deleting stopped", OpRename: "The rename failed", OpNewFolder: "The folder was not made",
		OpUndo: "Undo stopped",
	}[j.kind]
}

// undo reverses finished operation id. Its record stays while the undo
// runs, off the list Ctrl+Z takes from.
func (a *app) undo(id int) {
	f, ok := a.ops.records[id]
	if !ok || !slices.Contains(a.ops.undo, id) {
		return
	}
	a.ops.undo = slices.DeleteFunc(a.ops.undo, func(v int) bool { return v == id })
	rec := f.rec
	a.startOp(job{kind: OpUndo, undo: &rec}, "Undoing: "+f.title)
	a.ops.running[a.ops.next].undoes = id
}

// undone takes the end of the undo of finished operation id: it drops
// the record, or keeps the steps the undo left, to undo later.
func (a *app) undone(id int, rec record) {
	f, ok := a.ops.records[id]
	if !ok {
		return
	}
	if len(rec.left) == 0 {
		delete(a.ops.records, id)
		return
	}
	f.rec.steps = rec.left
	a.ops.undo = append(a.ops.undo, id)
}

// askClash asks the user about clash c of operation op, and waits for the
// answer or the end of ctx.
func (a *app) askClash(ctx context.Context, op int, c clash) (answer, error) {
	reply := make(chan ClashAnswered, 1)
	a.post(func() {
		ask := ClashAsk{Op: op, Name: filepath.Base(c.dst), Where: placeName(filepath.Dir(c.dst)),
			New: c.from, Old: describe(c.dst), SameKind: c.sameKind, CanForAll: true}
		if ask.New == "" {
			ask.New = describe(c.src)
		}
		a.showDialog(&dialog{view: "clash", state: ask, op: op, answer: func(in gunim.Intent) {
			if v, ok := in.(ClashAnswered); ok {
				reply <- v
				return
			}
			reply <- ClashAnswered{Stop: true}
		}})
	})
	select {
	case v := <-reply:
		if v.Stop {
			return answer{}, errStopped
		}
		return answer{choice: choice(v.Choice), all: v.All}, nil
	case <-ctx.Done():
		return answer{}, ctx.Err()
	}
}

// describe says what is at path, for a clash dialog.
func describe(path string) string {
	e, err := statEntry(path)
	if err != nil {
		return "cannot be read: " + err.Error()
	}
	s := e.Type
	if !e.Dir {
		s += ", " + humanBytes(e.Size)
	}
	return s + ", modified " + e.Mod.Format("2006-01-02 15:04")
}

// confirm asks c, and runs yes once the user agrees.
func (a *app) confirm(c Confirm, yes func()) {
	a.ops.tokens++
	c.Token = a.ops.tokens
	a.showDialog(&dialog{view: "confirm", state: c, answer: func(in gunim.Intent) {
		if v, ok := in.(Confirmed); ok && v.OK {
			yes()
		}
	}})
}

// prompt asks for a name, and runs got with it once the user gives one.
func (a *app) prompt(p Prompt, got func(name string)) {
	a.ops.tokens++
	p.Token = a.ops.tokens
	a.showDialog(&dialog{view: "prompt", state: p, answer: func(in gunim.Intent) {
		if v, ok := in.(Prompted); ok && v.OK {
			got(v.Text)
		}
	}})
}

// showError says an operation failed, in a dialog.
func (a *app) showError(e ErrorBox) {
	a.showDialog(&dialog{view: "error", state: e, answer: func(gunim.Intent) {}})
}

// showDialog queues d, and shows it once the dialogs before it are
// answered.
func (a *app) showDialog(d *dialog) {
	a.ops.shown++
	d.id = gunim.ID(fmt.Sprintf("%s-%d", dialogID, a.ops.shown))
	a.ops.dialogs = append(a.ops.dialogs, d)
	if len(a.ops.dialogs) == 1 {
		a.mountDialog(d)
	}
}

func (a *app) mountDialog(d *dialog) {
	a.send(a.c.Mount(gunim.Root, d.id, d.view, d.state))
	a.send(a.c.Focus(d.id))
}

// answered hands a dialog's answer to the dialog showing, and shows the
// next. An answer meant for another dialog, as one that arrives after
// its operation ended, is dropped.
func (a *app) answered(in gunim.Intent) {
	if len(a.ops.dialogs) == 0 || !answers(in, a.ops.dialogs[0].state) {
		return
	}
	d := a.ops.dialogs[0]
	a.ops.dialogs = a.ops.dialogs[1:]
	d.answer(in)
	if len(a.ops.dialogs) > 0 {
		a.mountDialog(a.ops.dialogs[0])
		return
	}
	a.patch(FocusListing{})
}

// answers reports whether in is an answer to the dialog with state: of
// its kind, and with its operation or token.
func answers(in gunim.Intent, state any) bool {
	switch v := in.(type) {
	case ClashAnswered:
		s, ok := state.(ClashAsk)
		return ok && s.Op == v.Op
	case Confirmed:
		s, ok := state.(Confirm)
		return ok && s.Token == v.Token
	case Prompted:
		s, ok := state.(Prompt)
		return ok && s.Token == v.Token
	case PropsApplied:
		s, ok := state.(Props)
		return ok && s.Token == v.Token
	case DialogClosed:
		switch state.(type) {
		case ErrorBox, Props:
			return true
		}
	}
	return false
}

// dropDialogs takes away the dialogs of operation op, which has ended.
func (a *app) dropDialogs(op int) {
	if len(a.ops.dialogs) == 0 {
		return
	}
	first := a.ops.dialogs[0]
	a.ops.dialogs = slices.DeleteFunc(a.ops.dialogs, func(d *dialog) bool { return d.op == op && op != 0 })
	if first.op == op && op != 0 {
		a.send(a.c.Unmount(first.id))
		if len(a.ops.dialogs) > 0 {
			a.mountDialog(a.ops.dialogs[0])
		} else {
			a.patch(FocusListing{})
		}
	}
}
