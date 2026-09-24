//go:build linux || windows || darwin

package desktop

import (
	"encoding/binary"
	"image"
	"math"

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
	// maxQuads is the most glyphs one draw call carries.
	maxQuads = 4096
)

// atlas packs glyph masks into one single-channel texture, in shelves.
// When it fills, it starts again from empty.
type atlas struct {
	tex        uint32
	x, y, rowH int
	slots      map[glyphKey]glyphSlot
}

type glyphKey struct {
	face, id uint32
	// size is the device size in 1/64 pixel.
	size  int32
	shift uint8
}

// glyphSlot is where a glyph sits in the atlas. A zero w marks a glyph
// with nothing to draw, such as a space.
type glyphSlot struct {
	x, y, w, h int
	off        image.Point
}

func (r *renderer) initText() {
	g := r.gl
	r.glyphs.slots = map[glyphKey]glyphSlot{}
	r.glyphs.tex = g.CreateTexture()
	g.BindTexture(gl.TEXTURE_2D, r.glyphs.tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexImage2D(gl.TEXTURE_2D, 0, glR8, atlasSize, atlasSize, glRed, gl.UNSIGNED_BYTE, make([]byte, atlasSize*atlasSize))

	r.textVAO = g.CreateVertexArray()
	g.BindVertexArray(r.textVAO)
	r.textVBO = g.CreateBuffer()
	g.BindBuffer(gl.ARRAY_BUFFER, r.textVBO)
	g.BufferInit(gl.ARRAY_BUFFER, maxQuads*4*16, gl.STREAM_DRAW)
	g.EnableVertexAttribArray(0)
	g.VertexAttribPointer(0, 2, gl.FLOAT, false, 16, 0)
	g.EnableVertexAttribArray(1)
	g.VertexAttribPointer(1, 2, gl.FLOAT, false, 16, 8)

	// Every batch uses the same two triangles per quad.
	idx := make([]byte, 0, maxQuads*6*2)
	for q := range maxQuads {
		b := uint16(q * 4)
		for _, i := range [6]uint16{b, b + 1, b + 2, b, b + 2, b + 3} {
			idx = binary.LittleEndian.AppendUint16(idx, i)
		}
	}
	r.textIBO = g.CreateBuffer()
	g.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.textIBO)
	g.BufferInit(gl.ELEMENT_ARRAY_BUFFER, len(idx), gl.STREAM_DRAW)
	g.BufferSubData(gl.ELEMENT_ARRAY_BUFFER, 0, idx)
}

// text draws a shaped run.
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
	t := op.Transform
	plain := t.A == 1 && t.B == 0 && t.D == 0 && t.E == 1
	sizePx := op.Size * r.scale
	r.quads = r.quads[:0]

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
		slot, ok := r.glyph(op, face, faceID, gly.ID, sizePx, shift)
		if !ok {
			continue
		}
		if len(r.quads)/16 == maxQuads {
			r.flushText(op)
		}
		x0, y0 := float32(slot.off.X), float32(slot.off.Y)
		x1, y1 := x0+float32(slot.w), y0+float32(slot.h)
		u0, v0 := float32(slot.x)/atlasSize, float32(slot.y)/atlasSize
		u1, v1 := float32(slot.x+slot.w)/atlasSize, float32(slot.y+slot.h)/atlasSize
		corner := func(cx, cy, u, v float32) {
			if plain {
				r.quads = append(r.quads, ox+cx, oy+cy, u, v)
				return
			}
			r.quads = append(r.quads, ox+t.A*cx+t.B*cy, oy+t.D*cx+t.E*cy, u, v)
		}
		corner(x0, y0, u0, v0)
		corner(x1, y0, u1, v0)
		corner(x1, y1, u1, v1)
		corner(x0, y1, u0, v1)
	}
	r.flushText(op)
	r.gl.BindVertexArray(r.vao)
}

// glyph returns where a glyph sits in the atlas, rasterizing and
// uploading it on first use. It reports false for a glyph with nothing
// to draw.
func (r *renderer) glyph(op *paint.TextOp, face *text.Face, faceID, id uint32, sizePx float32, shift uint8) (glyphSlot, bool) {
	a := &r.glyphs
	key := glyphKey{face: faceID, id: id, size: int32(math.Round(float64(sizePx) * 64)), shift: shift}
	if slot, ok := a.slots[key]; ok {
		return slot, slot.w > 0
	}
	m := face.Rasterize(id, sizePx, float32(shift)/subpixel)
	if m.W == 0 {
		a.slots[key] = glyphSlot{}
		return glyphSlot{}, false
	}
	if m.W+1 > atlasSize || m.H+1 > atlasSize {
		return glyphSlot{}, false
	}
	if a.x+m.W+1 > atlasSize {
		a.x, a.y, a.rowH = 0, a.y+a.rowH, 0
	}
	if a.y+m.H+1 > atlasSize {
		// Full: draw what this run has queued against the glyphs it was
		// built from, then start the atlas again.
		r.flushText(op)
		r.resetAtlas()
	}
	slot := glyphSlot{x: a.x, y: a.y, w: m.W, h: m.H, off: m.Offset}
	g := r.gl
	g.BindTexture(gl.TEXTURE_2D, a.tex)
	g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(slot.x), int32(slot.y), int32(m.W), int32(m.H), glRed, gl.UNSIGNED_BYTE, m.Pix)
	a.x += m.W + 1
	a.rowH = max(a.rowH, m.H+1)
	a.slots[key] = slot
	return slot, true
}

func (r *renderer) resetAtlas() {
	a := &r.glyphs
	clear(a.slots)
	a.x, a.y, a.rowH = 0, 0, 0
	g := r.gl
	g.BindTexture(gl.TEXTURE_2D, a.tex)
	g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, atlasSize, atlasSize, glRed, gl.UNSIGNED_BYTE, make([]byte, atlasSize*atlasSize))
}

// flushText draws the queued glyph quads.
func (r *renderer) flushText(op *paint.TextOp) {
	n := len(r.quads) / 16
	if n == 0 {
		return
	}
	r.bytes = r.bytes[:0]
	for _, v := range r.quads {
		r.bytes = binary.LittleEndian.AppendUint32(r.bytes, math.Float32bits(v))
	}
	g := r.gl
	p := r.textProg
	g.UseProgram(p.id)
	g.BindVertexArray(r.textVAO)
	g.BindBuffer(gl.ARRAY_BUFFER, r.textVBO)
	g.BufferSubData(gl.ARRAY_BUFFER, 0, r.bytes)
	g.ActiveTexture(gl.TEXTURE0)
	g.BindTexture(gl.TEXTURE_2D, r.glyphs.tex)
	g.Uniform1i(p.loc["u_atlas"], 0)
	p.set4(g, "u_target", float32(r.fbW), float32(r.fbH))
	p.set4(g, "u_color", rgba(op.Color)...)
	g.DrawElements(gl.TRIANGLES, int32(n*6), glUnsignedShort, 0)
	r.quads = r.quads[:0]
}
