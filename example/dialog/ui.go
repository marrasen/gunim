package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// registerViews is the window half.
//
// Views run in the window's process and own everything about how the
// interface looks and feels: the widget tree, the springs, the focus
// ring, how a row arrives and leaves. The application sends state and
// hears intents, and the wiring in between is all here.
func registerViews(w *gunim.Window) {
	w.RegisterTheme(darkTheme())
	w.RegisterTheme(lightTheme())

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
	gunim.RegisterPatch(w, "joblist", func(l *widget.List, p JobProgress, u *gunim.UI) {
		if r, ok := widget.RowOf[*jobRow](l, widget.Key(p.ID)); ok {
			r.progress.Animate(p.Progress, widget.Quick.Get(u.Theme()))
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
//
// Its colours follow the theme through a status change. The row
// animates how far it has moved from its last status to its current
// one, and blends the theme's colours for the two each frame, so a theme
// switch in the middle of a status change lands smoothly too.
type jobRow struct {
	anim.Group

	job Job
	// progress trails the job's real progress, so a patch lands as a
	// bar sliding rather than jumping.
	progress *anim.Float
	// was is the status the row is moving away from, and change runs
	// from 0 to 1 as it arrives at job.Status.
	was    JobStatus
	change *anim.Float
	action *widget.Button
	// title is the job's name, shaped for its size.
	title     text.Run
	titleText string
}

func newJobRow(j Job) *jobRow {
	r := &jobRow{
		job:      j,
		was:      j.Status,
		progress: anim.NewFloat(j.Progress),
		change:   anim.NewFloat(1),
	}
	r.action = widget.NewButton("Run")
	r.action.On = intentFor(j)
	r.Add(r.progress, r.change)
	return r
}

// Set takes fresh data for a row that is already on screen. Everything
// it changes is a target, so the row moves to the new values rather
// than snapping to them.
func (r *jobRow) Set(j Job, u *gunim.UI) {
	if j.Status != r.job.Status {
		r.was = r.job.Status
		r.change.Jump(0)
		r.change.Animate(1, widget.Settle.Get(u.Theme()))
	}
	r.job = j
	r.progress.Animate(j.Progress, widget.Quick.Get(u.Theme()))
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

// tint returns the theme's colour for a status.
func tint(s JobStatus, th *theme.Live) color.NRGBA {
	switch s {
	case JobRunning:
		return RowRunning.Get(th)
	case JobDone:
		return RowDone.Get(th)
	case JobPending:
		return RowIdle.Get(th)
	}
	return RowIdle.Get(th)
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

// Layout implements [gunim.Node].
func (r *jobRow) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	h := RowHeight.Get(f.Theme)
	kid := kids.At(0)
	size := kid.Layout(gunim.Loose(geom.Sz(c.Max.W, h)))
	kid.Place(geom.Pt(c.Max.W-size.W-8, (h-size.H)/2))
	return geom.Sz(c.Max.W, h)
}

// Paint implements [gunim.Node].
func (r *jobRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	full := geom.Rect{Max: box.Point()}
	fill := anim.Mix(anim.ColorCodec, tint(r.was, th), tint(r.job.Status, th), r.change.Value())
	p.RRect(full, RowRadius.Get(th), paint.Solid(fill))

	// The bar reads the spring, so it is wherever the animation has got
	// to rather than wherever the last patch said.
	if t := r.progress.Value(); t > 0 {
		bar := geom.Rc(0, box.H-3, box.W*t, 3)
		p.RRect(bar, 1.5, paint.Solid(widget.Accent.Get(th)))
	}

	if size := RowTitleSize.Get(th); r.titleText != r.job.Title || r.title.Size != size {
		r.title, r.titleText = text.Default().Shape(r.job.Title, size), r.job.Title
	}
	r.title.Paint(p, geom.Pt(14, (box.H-r.title.Height())/2), widget.Ink.Get(th))
	kids.At(0).Paint(p)
}
