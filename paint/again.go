package paint

import (
	"weak"

	"github.com/marrasen/gunim/geom"
)

// A Run is a stretch of one frame's commands, which a node that draws
// the same again next frame can hand on with [Painter.Again] rather
// than drawing it anew: a terminal of thousands of cells, still while
// something beside it moves.
//
// It names the commands by where they lie in the frame and by the first
// and the last of them, so a run from another painter, or from another
// frame than the one before, is told apart and refused.
type Run struct {
	// p is the painter that drew the run, held weakly so a kept run never keeps it alive.
	p           weak.Pointer[Painter]
	from, to    int
	first, last Op
	// at is the space in force as the run was taken, and frame the painter's frame then.
	at    Transform
	frame uint64
}

// RunFrom returns the commands recorded since mark as a run.
func (p *Painter) RunFrom(mark int) Run {
	if p.self.Value() != p {
		p.self = weak.Make(p)
	}
	r := Run{p: p.self, from: mark, to: len(p.ops), at: p.at(), frame: p.frame}
	if r.to > r.from {
		r.first, r.last = p.ops[r.from], p.ops[r.to-1]
	}
	return r
}

// before reports whether run is from this painter's frame before this one.
func (p *Painter) before(run Run) bool {
	if run.p.Value() != p || !p.hasPrev || run.frame+1 != p.frame || run.from < 0 || run.to > len(p.prev) || run.from > run.to {
		return false
	}
	return run.to == run.from || p.prev[run.from] == run.first && p.prev[run.to-1] == run.last
}

// held returns the run's commands and their bounds while its painter holds them: in the frame they were drawn in, or
// in the one after.
func (run Run) held() ([]Op, []geom.Rect, bool) {
	p := run.p.Value()
	if p == nil {
		return nil, nil, false
	}
	var ops []Op
	var bounds []geom.Rect
	switch {
	case run.frame == p.frame:
		ops, bounds = p.ops, p.bounds
	case run.frame+1 == p.frame && p.hasPrev:
		ops, bounds = p.prev, p.prevBounds
	default:
		return nil, nil, false
	}
	if run.from < 0 || run.to > len(ops) || run.from > run.to {
		return nil, nil, false
	}
	if run.to > run.from && (ops[run.from] != run.first || ops[run.to-1] != run.last) {
		return nil, nil, false
	}
	return ops[run.from:run.to], bounds[run.from:run.to], true
}

// carried is a stretch of this frame's commands copied from the frame
// before: n of them, at at, from from.
type carried struct{ at, from, n int }

// Again records run, from the frame before, into this frame unchanged,
// and reports whether it could. It can when run is from this painter's
// frame before this one and holds only shapes, text, images, masks,
// ribbons, areas and cells, and layers that neither blur nor tilt, each closed
// in the run that opens it; with false nothing is recorded, and the
// node draws as usual.
//
// The commands keep the transforms and blends they were drawn with, so
// the space in force should be the one they were drawn in, and the
// blend in force does not change them. [Painter.Damage] takes
// commands carried to the place they held as unchanged, without
// comparing them.
func (p *Painter) Again(run Run) bool {
	if !p.before(run) {
		return false
	}
	depth := 0
	for _, op := range p.prev[run.from:run.to] {
		switch op := op.(type) {
		case *RRectOp, *TextOp, *ImageOp, *MaskOp, *CellsOp, *RibbonOp, *AreaOp:
		case *LayerOp:
			// A blur spreads past its bounds, a backdrop reads what lies
			// under it, and a tilt changes the space input is taken in.
			if op.Opts.Blur > 0 || op.Opts.Backdrop > 0 || op.Opts.Tilt.tilted() {
				return false
			}
			depth++
		case *LayerEndOp:
			depth--
			if depth < 0 {
				return false
			}
		default:
			return false
		}
	}
	if depth != 0 {
		return false
	}
	at := len(p.ops)
	for _, op := range p.prev[run.from:run.to] {
		switch op := op.(type) {
		case *RRectOp:
			p.ops = append(p.ops, p.takeRRect(*op))
		case *TextOp:
			t := p.texts.take()
			*t = *op
			p.ops = append(p.ops, t)
		case *ImageOp:
			c := *op
			p.ops = append(p.ops, &c)
		case *MaskOp:
			p.ops = append(p.ops, p.takeMask(*op))
		case *CellsOp:
			c := *op
			p.ops = append(p.ops, &c)
		case *RibbonOp:
			// Its points are copied, as the frame after next reuses the
			// buffer they lie in.
			p.ops = append(p.ops, p.takeRibbon(op, op.Points))
		case *AreaOp:
			p.ops = append(p.ops, p.takeArea(op, op.Points))
		case *LayerOp:
			c := *op
			p.ops = append(p.ops, &c)
		case *LayerEndOp:
			p.ops = append(p.ops, &LayerEndOp{})
		}
	}
	// The bounds come as they were: a layer in the run already holds
	// all it held. The layers open round the run take in the run, once,
	// from the ops at its own depth, which hold the rest.
	p.bounds = append(p.bounds, p.prevBounds[run.from:run.to]...)
	p.shows = append(p.shows, p.prevShows[run.from:run.to]...)
	if len(p.open) > 0 {
		var b, sh geom.Rect
		depth := 0
		for i, op := range p.prev[run.from:run.to] {
			if depth == 0 {
				b, sh = b.Union(p.prevBounds[run.from+i]), sh.Union(p.prevShows[run.from+i])
			}
			switch op.(type) {
			case *LayerOp:
				depth++
			case *LayerEndOp:
				depth--
			}
		}
		for _, j := range p.open {
			p.bounds[j] = p.bounds[j].Union(b)
			p.shows[j] = p.shows[j].Union(sh)
		}
	}
	if n := run.to - run.from; n > 0 {
		p.carried = append(p.carried, carried{at: at, from: run.from, n: n})
	}
	return true
}

// SameAs reports whether the commands recorded since mark draw what run,
// from the frame before, drew, command for command, at the same bounds:
// for checking that a node which said it would draw the same did.
func (p *Painter) SameAs(run Run, mark int) bool {
	if !p.before(run) || mark < 0 || mark > len(p.ops) || len(p.ops)-mark != run.to-run.from {
		return false
	}
	for i := range run.to - run.from {
		if p.bounds[mark+i] != p.prevBounds[run.from+i] || !sameOp(p.ops[mark+i], p.prev[run.from+i]) {
			return false
		}
	}
	return true
}

// Floated returns how many paintings [Painter.Float] has put off so far
// in the frame, still to run.
func (p *Painter) Floated() int { return len(p.floats) }
