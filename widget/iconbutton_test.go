package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

type refreshed struct{}

// painted returns the ops a node paints into box.
func painted(n gunim.Node, box geom.Size) []paint.Op {
	var p paint.Painter
	n.Paint(&p, gunim.Frame{Scale: 1}, box, gunim.Children{})
	return p.Ops()
}

// masksIn returns the mask ops in ops.
func masksIn(ops []paint.Op) []*paint.MaskOp {
	var out []*paint.MaskOp
	for _, op := range ops {
		if m, ok := op.(*paint.MaskOp); ok {
			out = append(out, m)
		}
	}
	return out
}

func TestAButtonShowsItsIconBeforeItsLabel(t *testing.T) {
	plain, withIcon := NewButton("Copy"), NewButton("Copy")
	withIcon.Icon = icon.Copy
	f := gunim.Frame{Scale: 1}
	a := plain.Layout(gunim.Constraints{Max: geom.Sz(400, 100)}, f, gunim.Children{})
	b := withIcon.Layout(gunim.Constraints{Max: geom.Sz(400, 100)}, f, gunim.Children{})
	if want := a.W + IconSize.Default() + IconGap.Default(); b.W != want {
		t.Fatalf("with an icon the button is %v wide, want %v", b.W, want)
	}
	ops := painted(withIcon, b)
	ms := masksIn(ops)
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.Copy {
		t.Fatalf("the button drew %d masks, want its icon", len(ms))
	}
	var textX float32 = -1
	for _, op := range ops {
		if tx, ok := op.(*paint.TextOp); ok && len(tx.Glyphs) > 0 {
			textX = tx.Transform.Apply(tx.Glyphs[0].At).X
		}
	}
	if ms[0].Rect.Max.X > textX || ms[0].Rect.Min.X < ButtonPadding.Default()-0.5 {
		t.Fatalf("the icon spans %v and the label starts at %v, want the icon first, inside the padding", ms[0].Rect, textX)
	}
	iconOnly := NewButton("")
	iconOnly.Icon = icon.Copy
	if s := iconOnly.Layout(gunim.Constraints{Max: geom.Sz(400, 100)}, f, gunim.Children{}); s.W != s.H {
		t.Fatalf("a button with an icon and no label is %v, want it square", s)
	}
}

func TestAnIconButtonIsSquareNamedByItsTooltipAndClickable(t *testing.T) {
	b := NewIconButton(icon.RefreshCw, "Refresh")
	b.On = refreshed{}
	w, run := stage(t, &frame{child: Row(b), size: geom.Sz(200, 100)})
	h := ButtonHeight.Default()
	ms := maskOps(w.Offscreen())
	if len(ms) != 1 {
		t.Fatalf("the icon button drew %d masks, want its icon", len(ms))
	}
	if r := ms[0].Rect; r != geom.Rc((h-16)/2, (h-16)/2, 16, 16) {
		t.Fatalf("the icon sits at %v, want it centred in the %v square", r, h)
	}
	if info := b.Access(); info.Role != access.RoleButton || info.Name != "Refresh" {
		t.Fatalf("the icon button reads as %v %q", info.Role, info.Name)
	}
	click(w, h/2, h/2)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (refreshed{}) {
		t.Fatalf("a click sent %v, want the button's intent", got)
	}
	if b.tip.popup != nil {
		t.Fatal("the tooltip is up after a click")
	}
}

// fillAlpha returns the alpha of the first rounded rectangle an icon button painted.
func fillAlpha(w *gunim.Window) uint8 {
	for _, op := range w.Offscreen().Ops() {
		if r, ok := op.(*paint.RRectOp); ok {
			return r.Fill.Solid.A
		}
	}
	return 0
}

func TestAnIconButtonFillsUnderThePointerAndShowsItsTooltip(t *testing.T) {
	b := NewIconButton(icon.X, "Close")
	w, run := stage(t, &frame{child: Row(b), size: geom.Sz(200, 100)})
	if a := fillAlpha(w); a != 0 {
		t.Fatalf("at rest the icon button's fill has alpha %d, want it clear", a)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(10, 10), Time: time.Now()})
	run(60)
	if a := fillAlpha(w); a < 0xf0 {
		t.Fatalf("under the pointer the fill has alpha %d, want the hover fill", a)
	}
	if b.tip.popup == nil {
		t.Fatal("the tooltip did not show after its delay")
	}
	w.Input(input.PointerLeave{Time: time.Now()})
	run(60)
	if b.tip.popup != nil || fillAlpha(w) != 0 {
		t.Fatal("the tooltip or the fill stayed after the pointer left")
	}
}

func TestALinkShowsItsIconBeforeItsText(t *testing.T) {
	l := NewLink("Open folder")
	l.Icon = icon.FolderOpen
	w, _ := stage(t, &frame{child: Row(l), size: geom.Sz(400, 40)})
	ms := maskOps(w.Offscreen())
	if len(ms) != 1 || ms[0].Color != LinkInk.Default() {
		t.Fatalf("the link drew %d masks, want its icon in the link colour", len(ms))
	}
	size := TextSize.Default()
	if r := ms[0].Rect; r.Min.X != 0 || r.Size() != geom.Sz(size, size) {
		t.Fatalf("the icon is at %v, want it first, as tall as the text", r)
	}
	for _, op := range w.Offscreen().Ops() {
		if tx, ok := op.(*paint.TextOp); ok {
			if x := tx.Transform.Apply(tx.Glyphs[0].At).X; x < size+IconGap.Default() {
				t.Fatalf("the text starts at %v, over the icon", x)
			}
		}
	}
}

func TestMenuItemsShowTheirIcons(t *testing.T) {
	m := NewMenu("Copy", "Refresh", "Close")
	f := gunim.Frame{Scale: 1}
	plain := m.Layout(gunim.Constraints{Max: geom.Sz(400, 400)}, f, gunim.Children{})
	m.Icons = []*icon.Icon{icon.Copy, nil, icon.X}
	s := m.Layout(gunim.Constraints{Max: geom.Sz(400, 400)}, f, gunim.Children{})
	if want := plain.W + IconSize.Default() + IconGap.Default(); s.W != want {
		t.Fatalf("with icons the menu is %v wide, want %v", s.W, want)
	}
	ms := masksIn(painted(m, s))
	if len(ms) != 2 || strokeOf(t, ms[0]).Icon != icon.Copy || strokeOf(t, ms[1]).Icon != icon.X {
		t.Fatalf("the menu drew %d icons, want copy and close", len(ms))
	}
}
