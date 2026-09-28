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

// The contrast glyph coverage is enhanced by, for greyscale glyphs and
// for glyphs on subpixels.
const (
	greyContrast = 1.0
	lcdContrast  = 0.5
)

// gammaRatios are Direct2D's coefficients for correcting glyph coverage
// blended in gamma space, for gammas from 1 to 2.2 in steps of a tenth.
var gammaRatios = [...][4]float32{
	{0, 0, 0, 0},
	{0.0166, -0.0807, 0.2227, -0.0751},
	{0.0350, -0.1760, 0.4325, -0.1370},
	{0.0543, -0.2821, 0.6302, -0.1876},
	{0.0739, -0.3963, 0.8167, -0.2287},
	{0.0933, -0.5161, 0.9926, -0.2616},
	{0.1121, -0.6395, 1.1588, -0.2877},
	{0.1300, -0.7649, 1.3159, -0.3080},
	{0.1469, -0.8911, 1.4644, -0.3234},
	{0.1627, -1.0170, 1.6051, -0.3347},
	{0.1773, -1.1420, 1.7385, -0.3426},
	{0.1908, -1.2652, 1.8650, -0.3476},
	{0.2031, -1.3864, 1.9851, -0.3501},
}

// ratiosFor returns the gamma ratios nearest gamma, scaled as the
// shader uses them.
func ratiosFor(gamma float32) [4]float32 {
	i := int(math.Round(float64((gamma - 1) * 10)))
	r := gammaRatios[min(max(i, 0), len(gammaRatios)-1)]
	for k := range r {
		r[k] /= 4
	}
	return r
}

// glyphTexture is a renderer's copy of a shared atlas: the glyphs it
// has drawn, from the atlas's epoch. The greyscale atlas has one
// channel, and the subpixel one three.
type glyphTexture struct {
	tex   uint32
	epoch int
	have  map[glyphKey]bool
}

func (r *renderer) initGlyphs() {
	r.glyphs.have = map[glyphKey]bool{}
	r.lcdGlyphs.have = map[glyphKey]bool{}
	r.glyphs.tex = r.newAtlas(gl.TEXTURE0, glR8, glRed, 1)
}

// newAtlas makes an empty atlas texture on unit, with channels bytes a
// pixel.
func (r *renderer) newAtlas(unit uint32, internal int32, format uint32, channels int) uint32 {
	g := r.gl
	tex := g.CreateTexture()
	g.ActiveTexture(unit)
	g.BindTexture(gl.TEXTURE_2D, tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexImage2D(gl.TEXTURE_2D, 0, internal, atlasSize, atlasSize, format, gl.UNSIGNED_BYTE,
		make([]byte, atlasSize*atlasSize*channels))
	g.ActiveTexture(gl.TEXTURE0)
	return tex
}

// text queues a shaped run's glyphs.
//
// A run under a plain translation snaps each glyph to the pixel grid,
// to the nearest quarter pixel across and a whole pixel down, so text
// at rest is sharp. Under a scale or rotation, such as a dialog growing
// into place, the glyphs keep their resting size in the atlas and the
// quads carry the transform; they soften while moving and sharpen when
// the transform settles back to a translation.
//
// Glyphs use the panel's subpixels only at rest, and straight on the
// canvas or the window, whose pixels behind them are opaque.
func (r *renderer) text(op *paint.TextOp) {
	if len(op.Glyphs) == 0 || op.Size <= 0 || op.Color.A == 0 {
		return
	}
	r.uses(0)
	t := op.Transform
	plain := t.A == 1 && t.B == 0 && t.D == 0 && t.E == 1
	sizePx := op.Size * r.scale
	raster := text.Raster{Hint: r.textRendering.Hinting == text.HintingLight}
	l := look{kind: kindGlyph, color0: rgba(op.Color), color1: r.gamma, stroke: greyContrast}
	if r.subpixels && plain && r.depth == 0 {
		raster.LCD = true
		l.radius, l.stroke = 1, lcdContrast
		if r.textRendering.Smoothing == text.SubpixelBGR {
			l.radius = 2
		}
	}
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
		key := glyphKeyFor(faceID, gly.ID, sizePx, shift, raster)
		slot, ok := r.glyph(key, func() text.Mask {
			return face.Rasterize(key.id, sizePx, float32(key.shift)/subpixel, key.raster)
		})
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

// glyph returns where a glyph sits in its atlas, rasterizing it with raster and copying it into this
// renderer's texture on first use. It reports false for a glyph with
// nothing to draw.
func (r *renderer) glyph(key glyphKey, raster func() text.Mask) (glyphSlot, bool) {
	slot, epoch, ok := r.shared.glyph(key, raster)
	a, unit, format, channels := &r.glyphs, uint32(gl.TEXTURE0), uint32(glRed), 1
	if key.raster.LCD {
		a, unit, format, channels = &r.lcdGlyphs, glTexture2, glRGB, 3
		if a.tex == 0 {
			a.tex, a.epoch = r.newAtlas(unit, glRGB8, format, channels), epoch
		}
	}
	g := r.gl
	if epoch != a.epoch {
		// The atlas started again, here or in another window. What is
		// queued was placed in the old one, so it draws first.
		r.flush()
		clear(a.have)
		a.epoch = epoch
		g.ActiveTexture(unit)
		g.BindTexture(gl.TEXTURE_2D, a.tex)
		g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, atlasSize, atlasSize, format, gl.UNSIGNED_BYTE,
			make([]byte, atlasSize*atlasSize*channels))
		g.ActiveTexture(gl.TEXTURE0)
	}
	if !ok {
		return glyphSlot{}, false
	}
	if !a.have[key] {
		// Queued glyphs are drawn later, from the texture as it will be
		// then, which holds them all: a new glyph only fills an empty
		// place.
		g.ActiveTexture(unit)
		g.BindTexture(gl.TEXTURE_2D, a.tex)
		g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(slot.x), int32(slot.y), int32(slot.w), int32(slot.h), format,
			gl.UNSIGNED_BYTE, slot.pix)
		g.ActiveTexture(gl.TEXTURE0)
		a.have[key] = true
	}
	return slot.glyphSlot, true
}
