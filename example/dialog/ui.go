package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

// registerViews is the window half.
//
// Views run in the window's process and own everything about how the
// interface looks and feels: the widget tree, the springs, the focus
// ring, how a row arrives and leaves. The application sends state and
// hears intents, and the wiring in between is all here.
func registerViews(w *gunim.Window) {
	gunim.RegisterView(w, "joblist",
		func(JobList) *widget.List { return widget.NewList() },
		func(l *widget.List, s JobList, u *gunim.UI) {
			// One line turns a fresh slice into animation. Rows that
			// arrived grow into place, rows that went collapse while
			// their neighbours close the gap, and the rest spring to
			// wherever they belong now.
			widget.Sync(l, u, s.Jobs,
				func(j Job) widget.Key { return widget.Key(j.ID) },
				newJobRow,
				(*jobRow).Set)
		})

	// A patch reaches one row and retargets one spring. The list stays
	// exactly as it is, so a job ticking from 20% to 30% costs a
	// retarget rather than a reconciliation.
	gunim.RegisterPatch(w, "joblist", func(l *widget.List, p JobProgress, _ *gunim.UI) {
		if r, ok := widget.RowOf[*jobRow](l, widget.Key(p.ID)); ok {
			r.progress.Animate(p.Progress, anim.Snappy)
		}
	})

	gunim.RegisterView(w, "confirm",
		func(s ConfirmState) *widget.Dialog {
			d := widget.NewDialog(s.Title)
			d.Accept = Confirmed{What: s.Title}
			d.Dismiss = Cancelled{}
			return d
		},
		func(d *widget.Dialog, s ConfirmState, _ *gunim.UI) {
			d.SetTitle(s.Title)
		})
}

// jobRow renders one job: a title, a progress bar and a button.
type jobRow struct {
	anim.Group

	job Job
	// progress trails the job's real progress, so a patch lands as a
	// bar sliding rather than jumping.
	progress *anim.Float
	tint     *anim.Color
	action   *widget.Button
	// title is the job's name, shaped when the job changes.
	title text.Run
}

var (
	rowIdle    = color.NRGBA{R: 0x22, G: 0x26, B: 0x30, A: 0xff}
	rowRunning = color.NRGBA{R: 0x1e, G: 0x33, B: 0x4a, A: 0xff}
	rowDone    = color.NRGBA{R: 0x1e, G: 0x3a, B: 0x2c, A: 0xff}
	rowBar     = color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff}
	rowText    = color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff}
)

func newJobRow(j Job) *jobRow {
	r := &jobRow{
		job:      j,
		progress: anim.NewFloat(j.Progress),
		tint:     anim.NewColor(tintFor(j.Status)),
		title:    text.Default().Shape(j.Title, 15),
	}
	r.action = widget.NewButton("Run")
	r.action.On = intentFor(j)
	r.Add(r.progress, r.tint)
	return r
}

// Set takes fresh data for a row that is already on screen. Everything
// it changes is a target, so the row moves to the new values rather
// than snapping to them.
func (r *jobRow) Set(j Job) {
	r.job = j
	r.title = text.Default().Shape(j.Title, 15)
	r.progress.Animate(j.Progress, anim.Snappy)
	r.tint.Animate(tintFor(j.Status), anim.Gentle)
	r.action.SetLabel(labelFor(j.Status))
	r.action.On = intentFor(j)
}

// intentFor is what the row's button asks for: to run a pending job,
// to delete a finished one, and nothing while it runs.
func intentFor(j Job) gunim.Intent {
	switch j.Status {
	case JobPending:
		return RunRequested{ID: j.ID}
	case JobDone:
		return DeleteRequested{ID: j.ID}
	case JobRunning:
	}
	return nil
}

func tintFor(s JobStatus) color.NRGBA {
	switch s {
	case JobRunning:
		return rowRunning
	case JobDone:
		return rowDone
	case JobPending:
		return rowIdle
	}
	return rowIdle
}

func labelFor(s JobStatus) string {
	switch s {
	case JobRunning:
		return "Running"
	case JobDone:
		return "Delete"
	case JobPending:
		return "Run"
	}
	return "Run"
}

// Children implements [gunim.Composite].
func (r *jobRow) Children() []gunim.Node { return []gunim.Node{r.action} }

const rowHeight = 44

// Layout implements [gunim.Node].
func (r *jobRow) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	size := kid.Layout(gunim.Loose(geom.Sz(c.Max.W, rowHeight)))
	kid.Place(geom.Pt(c.Max.W-size.W-8, (rowHeight-size.H)/2))
	return geom.Sz(c.Max.W, rowHeight)
}

// Paint implements [gunim.Node].
func (r *jobRow) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	full := geom.Rect{Max: box.Point()}
	p.RRect(full, 8, paint.Solid(r.tint.Value()))

	// The bar reads the spring, so it is wherever the animation has got
	// to rather than wherever the last patch said.
	if t := r.progress.Value(); t > 0 {
		bar := geom.Rc(0, box.H-3, box.W*t, 3)
		p.RRect(bar, 1.5, paint.Solid(rowBar))
	}

	r.title.Paint(p, geom.Pt(14, (box.H-r.title.Height())/2), rowText)
	kids.At(0).Paint(p)
}
