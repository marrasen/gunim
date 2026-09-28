package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// paintRow paints the palette's row for item i on its own, and returns its ops.
func paintRow(c *paletteCard, i int) []paint.Op {
	r := newPaletteRow(c, c.p.keyOf(i))
	var p paint.Painter
	r.Paint(&p, gunim.Frame{}, geom.Sz(500, PaletteRowHeight.Default()), gunim.Children{})
	return p.Ops()
}

// titleX returns where the first text a row draws starts.
func titleX(t *testing.T, ops []paint.Op) float32 {
	t.Helper()
	for _, op := range ops {
		if o, ok := op.(*paint.TextOp); ok {
			return o.Transform.C
		}
	}
	t.Fatal("the row drew no text")
	return 0
}

func TestPaletteTitlesLineUpAfterAColumnForIcons(t *testing.T) {
	w, o, run := newPaletteStage(t)
	o.p.Items[1].Icon = icon.Columns2
	o.p.Items[2].Icon = icon.Rows2
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(20)
	c := o.p.card
	pad, room := MenuRowPadding.Default(), IconSize.Default()+IconGap.Default()
	with, without := paintRow(c, 1), paintRow(c, 0)
	var masks []*paint.MaskOp
	for _, op := range with {
		if m, ok := op.(*paint.MaskOp); ok {
			masks = append(masks, m)
		}
	}
	if len(masks) != 1 || strokeOf(t, masks[0]).Icon != icon.Columns2 || masks[0].Rect.Min.X != pad {
		t.Fatalf("the row with an icon drew %d masks, want its icon at the row's padding", len(masks))
	}
	if a, b := titleX(t, with), titleX(t, without); a != pad+room || b != pad+room {
		t.Fatalf("the titles start at %v and %v, want both at %v", a, b, pad+room)
	}
	// Narrowed to an item with no icon, the others' icons still keep the column.
	w.Input(input.TextInput{Text: "paste"})
	run(20)
	if f := c.found; len(f) != 1 || f[0].Index != 3 {
		t.Fatalf("typing paste found %v", f)
	}
	if x := titleX(t, paintRow(c, 3)); x != pad+room {
		t.Fatalf("narrowed, the title starts at %v, want %v", x, pad+room)
	}
	o.p.Items[1].Icon, o.p.Items[2].Icon = nil, nil
	if x := titleX(t, paintRow(c, 3)); x != pad {
		t.Fatalf("with no icons the title starts at %v, want at the padding", x)
	}
}
