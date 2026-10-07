package widget

import (
	"fmt"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// ringReach is how far past its box a widget may draw: its focus ring's
// reach.
const ringReach = 4

// spills returns a line for each op in ops drawn past box, by more than
// reach. A text op counts by its glyphs' origins. Ops inside a clipping
// layer that lies within box are left out.
func spills(ops []paint.Op, box geom.Size, reach float32) []string {
	within := geom.Rect{Min: geom.Pt(-reach, -reach), Max: geom.Pt(box.W+reach, box.H+reach)}
	outside := func(r geom.Rect) bool {
		return r.Min.X < within.Min.X || r.Min.Y < within.Min.Y || r.Max.X > within.Max.X || r.Max.Y > within.Max.Y
	}
	moved := func(t paint.Transform, r geom.Rect) geom.Rect {
		return geom.Rect{Min: t.Apply(r.Min), Max: t.Apply(r.Max)}
	}
	var out []string
	clipped := 0
	for _, op := range ops {
		switch op := op.(type) {
		case *paint.LayerOp:
			if clipped > 0 || op.Opts.Clip && !outside(moved(op.Transform, op.Opts.Bounds)) {
				clipped++
			}
		case *paint.LayerEndOp:
			if clipped > 0 {
				clipped--
			}
		}
		if clipped > 0 {
			continue
		}
		switch op := op.(type) {
		case *paint.RRectOp:
			r := moved(op.Transform, op.Rect)
			half := op.Stroke.Width / 2
			stroke := geom.Rect{Min: r.Min.Sub(geom.Pt(half, half)), Max: r.Max.Add(geom.Pt(half, half))}
			if outside(r) && op.Fill != (paint.Fill{}) || outside(stroke) && op.Stroke.Width > 0 {
				out = append(out, fmt.Sprintf("a rect at %v", r))
			}
		case *paint.MaskOp:
			if r := moved(op.Transform, op.Rect); outside(r) {
				out = append(out, fmt.Sprintf("a mask at %v", r))
			}
		case *paint.ImageOp:
			if r := moved(op.Transform, op.Rect); outside(r) {
				out = append(out, fmt.Sprintf("an image at %v", r))
			}
		case *paint.TextOp:
			for _, g := range op.Glyphs {
				if at := op.Transform.Apply(g.At); at.X < within.Min.X || at.X > within.Max.X {
					out = append(out, fmt.Sprintf("a glyph at %v", at))
					break
				}
			}
		}
	}
	return out
}
