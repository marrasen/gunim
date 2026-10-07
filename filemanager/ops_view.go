package filemanager

import (
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

func registerOps(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, s Ops, u *gunim.UI) { b.ops.set(s, u) })
	gunim.RegisterPatch(w, "browser", func(b *browser, t OpTick, u *gunim.UI) { b.ops.tick(t, u) })
	gunim.RegisterPatch(w, "browser", func(b *browser, n Notice, u *gunim.UI) {
		t := widget.Toast{Title: n.Title, Body: n.Body, Kind: toastKinds[n.Kind]}
		if n.Undo != 0 {
			t.Action, t.On = "Undo", UndoOp{ID: n.Undo}
		}
		t.Key, t.Check = n.Key, n.Check
		answer := func(i int) func(bool) gunim.Intent {
			return func(on bool) gunim.Intent { return NoticeAnswered{Key: n.Key, Button: i, Checked: on} }
		}
		for i, label := range n.Buttons {
			t.Buttons = append(t.Buttons, widget.ToastButton{Label: label, On: answer(i)})
		}
		if len(n.Buttons) > 0 {
			t.Dismiss = answer(-1)
		}
		b.toasts.Show(t, u)
	})
	gunim.RegisterPatch(w, "browser", func(b *browser, n NoticeGone, u *gunim.UI) { b.toasts.Close(n.Key, u) })
}

// toastKinds are the toasts' kinds, by a notice's kind.
var toastKinds = map[string]widget.ToastKind{
	"success": widget.ToastSuccess,
	"warning": widget.ToastWarning,
	"info":    widget.ToastInfo,
}

// opsPanel is the panel over the status bar that shows the operations
// running. It slides up when the first one starts and away when the last
// one ends, and each operation's row grows in and collapses out.
type opsPanel struct {
	fold *fold
	list *widget.List
}

func newOpsPanel() *opsPanel {
	p := &opsPanel{list: widget.NewList()}
	p.fold = newFold(&panelBox{child: p.list})
	return p
}

func (p *opsPanel) set(s Ops, u *gunim.UI) {
	widget.Sync(p.list, u, s.Ops,
		func(o OpView) widget.Key { return widget.Key(strconv.Itoa(o.ID)) },
		newOpRow, (*opRow).set)
	p.fold.set(len(s.Ops) > 0, u)
}

func (p *opsPanel) tick(t OpTick, u *gunim.UI) {
	if r, ok := widget.RowOf[*opRow](p.list, widget.Key(strconv.Itoa(t.ID))); ok {
		r.progress(t.Done, t.Unknown, t.Detail, u)
	}
}

// opRow is one running operation: what it does, a bar, how far it has
// got, and a button to stop it.
type opRow struct {
	title  *widget.Label
	detail *widget.Label
	bar    *widget.ProgressBar
	cancel *widget.Button
	graph  *widget.LiveGraph
	badge  *doneBadge
	// samples counts the speeds the graph has had.
	samples int
	col     *widget.Flex
}

func newOpRow(o OpView) *opRow {
	r := &opRow{title: widget.NewLabel(o.Title), detail: widget.NewLabel(o.Detail), bar: widget.NewProgressBar(),
		cancel: widget.NewButton("Cancel"), graph: newSpeedGraph()}
	r.badge = newDoneBadge(r.cancel)
	r.title.MaxLines = 1
	r.detail.Color, r.detail.Size, r.detail.MaxLines = Faint, SmallText, 1
	r.cancel.On = CancelOp{ID: o.ID}
	r.bar.Indeterminate = o.Unknown
	head := widget.Row(r.title, r.badge).Grow(r.title, 1)
	head.Cross = widget.CrossCenter
	r.col = widget.Column(head, &glowBar{bar: r.bar, badge: r.badge}, r.graph, r.detail)
	r.col.Cross = widget.CrossStretch
	r.col.Gap = smallGap
	return r
}

func (r *opRow) set(o OpView, u *gunim.UI) {
	r.title.Text = o.Title
	r.progress(o.Done, o.Unknown, o.Detail, u)
}

func (r *opRow) progress(done float32, unknown bool, detail string, u *gunim.UI) {
	r.bar.Indeterminate = unknown
	if !unknown {
		r.bar.SetValue(done, u)
	}
	r.detail.Text = detail
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (r *opRow) Children() []gunim.Node { return []gunim.Node{r.col} }

// Layout implements [gunim.Node].
func (r *opRow) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W-24, 0), Max: geom.Sz(c.Max.W-24, 0)})
	kid.Place(geom.Pt(12, 10))
	return geom.Sz(c.Max.W, s.H+20)
}

// Paint implements [gunim.Node].
func (r *opRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, widget.CardRadius.Get(f.Theme), paint.Solid(widget.CardFill.Get(f.Theme)))
	paintCheer(p, f.Theme, r.badge, box)
	kids.At(0).Paint(p)
}

// panelBox pads the progress panel and draws its edge.
type panelBox struct {
	child gunim.Node
}

// Children implements [gunim.Composite].
func (b *panelBox) Children() []gunim.Node { return []gunim.Node{b.child} }

// Layout implements [gunim.Node].
func (b *panelBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W-24, 0), Max: geom.Sz(c.Max.W-24, 0)})
	kid.Place(geom.Pt(12, 10))
	return geom.Sz(c.Max.W, s.H+20)
}

// Paint implements [gunim.Node].
func (b *panelBox) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(PaneFill.Get(f.Theme)))
	p.RRect(geom.Rc(0, 0, box.W, 1), 0, paint.Solid(widget.SplitLine.Get(f.Theme)))
	kids.At(0).Paint(p)
}
