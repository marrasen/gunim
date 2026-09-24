//go:build linux || windows || darwin

package desktop

import (
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

const (
	// atlasSize is the glyph atlas's width and height in pixels. At
	// 1024 it holds a few thousand glyphs at interface sizes.
	atlasSize = 1024
	// subpixel is how many horizontal shifts of a glyph the atlas
	// keeps, so a glyph at any fractional x lands within a quarter
	// pixel of where it belongs.
	subpixel = 4
)

// glyphTexture is a renderer's copy of the shared atlas: the glyphs it
// has drawn, in a single-channel texture, from the atlas's epoch.
type glyphTexture struct {
	tex   uint32
	epoch int
	have  map[glyphKey]bool
}

func (r *renderer) initGlyphs() {
	g := r.gl
	r.glyphs.have = map[glyphKey]bool{}
	r.glyphs.tex = g.CreateTexture()
	g.ActiveTexture(gl.TEXTURE0)
	g.BindTexture(gl.TEXTURE_2D, r.glyphs.tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexImage2D(gl.TEXTURE_2D, 0, glR8, atlasSize, atlasSize, glRed, gl.UNSIGNED_BYTE, make([]byte, atlasSize*atlasSize))
}

// text queues a shaped run's glyphs.
//
// A run under a plain translation snaps each glyph to the pixel grid,
// to the nearest quarter pixel across and a whole pixel down, so text
// at rest is sharp. Under a scale or rotation, such as a dialog growing
// into place, the glyphs keep their resting size in the atlas and the
// quads carry the transform; they soften while moving and sharpen when
// the transform settles back to a translation.
func (r *renderer) text(op *paint.TextOp) {
	if len(op.Glyphs) == 0 || op.Size <= 0 || op.Color.A == 0 {
		return
	}
	r.uses(0)
	t := op.Transform
	plain := t.A == 1 && t.B == 0 && t.D == 0 && t.E == 1
	sizePx := op.Size * r.scale
	l := look{kind: kindGlyph, color0: rgba(op.Color)}
	// Corners arrive in device pixels, placed by the glyph's own
	// transform: the op's without its translation.
	shape := paint.Transform{A: t.A, B: t.B, D: t.D, E: t.E}

	var (
		face   *text.Face
		faceID uint32
	)
	for _, gly := range op.Glyphs {
		if face == nil || gly.Face != faceID {
			f, ok := text.Lookup(gly.Face)
			if !ok {
				continue
			}
			face, faceID = f, gly.Face
		}
		o := t.Apply(gly.At)
		ox, oy := o.X*r.scale, o.Y*r.scale
		var shift uint8
		if plain {
			fx := float32(math.Floor(float64(ox)))
			s := int(math.Round(float64(ox-fx) * subpixel))
			if s == subpixel {
				fx, s = fx+1, 0
			}
			ox, oy, shift = fx, float32(math.Round(float64(oy))), uint8(s)
		}
		slot, ok := r.glyph(glyphKeyFor(faceID, gly.ID, sizePx, shift), face, sizePx)
		if !ok {
			continue
		}
		r.uses(0)
		x0, y0 := float32(slot.off.X), float32(slot.off.Y)
		q := geom.Rect{Min: geom.Pt(x0, y0), Max: geom.Pt(x0+float32(slot.w), y0+float32(slot.h))}
		uv := geom.Rect{
			Min: geom.Pt(float32(slot.x)/atlasSize, float32(slot.y)/atlasSize),
			Max: geom.Pt(float32(slot.x+slot.w)/atlasSize, float32(slot.y+slot.h)/atlasSize),
		}
		shape.C, shape.F = ox, oy
		r.quad(corners(q, uv), shape, 1, &l)
	}
}

// glyph returns where a glyph sits in the atlas, copying it into this
// renderer's texture on first use. It reports false for a glyph with
// nothing to draw.
func (r *renderer) glyph(key glyphKey, face *text.Face, sizePx float32) (glyphSlot, bool) {
	slot, epoch, ok := r.shared.glyph(key, face, sizePx)
	a := &r.glyphs
	g := r.gl
	if epoch != a.epoch {
		// The atlas started again, here or in another window. What is
		// queued was placed in the old one, so it draws first.
		r.flush()
		clear(a.have)
		a.epoch = epoch
		g.ActiveTexture(gl.TEXTURE0)
		g.BindTexture(gl.TEXTURE_2D, a.tex)
		g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, atlasSize, atlasSize, glRed, gl.UNSIGNED_BYTE, make([]byte, atlasSize*atlasSize))
	}
	if !ok {
		return glyphSlot{}, false
	}
	if !a.have[key] {
		// Queued glyphs are drawn later, from the texture as it will be
		// then, which holds them all: a new glyph only fills an empty
		// place.
		g.ActiveTexture(gl.TEXTURE0)
		g.BindTexture(gl.TEXTURE_2D, a.tex)
		g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(slot.x), int32(slot.y), int32(slot.w), int32(slot.h), glRed, gl.UNSIGNED_BYTE, slot.pix)
		a.have[key] = true
	}
	return slot.glyphSlot, true
}
