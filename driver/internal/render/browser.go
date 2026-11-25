//go:build linux || windows || darwin || (js && wasm)

package render

import (
	"math"
	"runtime"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// In a browser, the glyphs no face a program carries has are drawn by the browser itself, which takes much longer than
// drawing an outline. So a frame makes only as many as it has time for, and the frames after it the rest.

// browserGlyphs says there is a browser to draw glyphs.
const browserGlyphs = runtime.GOOS == "js"

const (
	// browserTime is how long a frame makes new glyphs of the browser's for, a third of a frame at 120 Hz. A screenful
	// of Chinese is some hundreds of them, at up to half a millisecond each.
	browserTime = 2500 * time.Microsecond
	// browserFade is how long a glyph that came late takes to fade in.
	browserFade = 120 * time.Millisecond
	// browserLate is how many late glyphs are told apart.
	browserLate = 8192
	// browserMax is how large a glyph of the browser's is made, in device pixels each way. One larger is stretched.
	browserMax = 1800
	// keptBytes is how much of the browser's glyphs is kept in each of two turns, besides what the atlas holds.
	keptBytes = 8 << 20
)

// browserState is a renderer's part in drawing the browser's glyphs: how long this frame has spent on them, the frame's
// number, and the glyphs left to a later frame.
type browserState struct {
	spent time.Duration
	frame uint64
	late  map[glyphKey]lateGlyph
}

// lateGlyph is a glyph left to a later frame: the frame that last left it, and when one first drew it, or the zero time.
type lateGlyph struct {
	left uint64
	from time.Time
}

// newFrame starts a frame.
func (b *browserState) newFrame() {
	b.spent = 0
	b.frame++
}

// keptMasks keeps the masks of the browser's glyphs, so an atlas that starts again costs the browser nothing. It holds
// those made or asked for since its last turn, and those of the turn before, and turns as it fills.
type keptMasks struct {
	now, old map[glyphKey]text.Mask
	bytes    int
}

func (k *keptMasks) get(key glyphKey) (text.Mask, bool) {
	if m, ok := k.now[key]; ok {
		return m, true
	}
	m, ok := k.old[key]
	if ok {
		k.put(key, m)
	}
	return m, ok
}

func (k *keptMasks) has(key glyphKey) bool {
	_, ok := k.now[key]
	if !ok {
		_, ok = k.old[key]
	}
	return ok
}

func (k *keptMasks) put(key glyphKey, m text.Mask) {
	// A rough size for the entry and its place in the map, beyond its pixels.
	const entry = 160
	if k.now == nil || k.bytes >= keptBytes {
		k.now, k.old, k.bytes = map[glyphKey]text.Mask{}, k.now, 0
	}
	k.now[key] = m
	k.bytes += len(m.Pix) + entry
}

// fromBrowser reports whether the browser draws face's glyphs.
func fromBrowser(face *text.Face) bool { return browserGlyphs && face.FromBrowser() }

// browserMask returns a glyph's mask: one kept, or drawn. It runs with the shared atlas's lock held.
func (r *Renderer) browserMask(key glyphKey, draw func() text.Mask) text.Mask {
	if m, ok := r.shared.kept.get(key); ok {
		return m
	}
	start := time.Now()
	m := draw()
	r.browser.spent += time.Since(start)
	r.shared.kept.put(key, m)
	return m
}

// browserSize returns the size a glyph of the browser's is made at to be shown at sizePx device pixels.
func browserSize(face *text.Face, id uint32, sizePx float32) float32 {
	// With room for what reaches past its advance and the line's height.
	return min(sizePx, browserMax/max(face.Reach(id)+1.5, 3))
}

// browserGlyph queues a glyph the browser makes, for text of sizePx device pixels under shape, its origin at, with l.
func (r *Renderer) browserGlyph(key glyphKey, face *text.Face, sizePx float32, at geom.Point, shape paint.Transform, upright bool, reach float32, l *look) {
	if size := browserSize(face, key.id, sizePx); size < sizePx {
		k := sizePx / size
		shape.A, shape.B, shape.D, shape.E = shape.A*k, shape.B*k, shape.D*k, shape.E*k
		sizePx = size
		key.size = int32(math.Round(float64(size) * 64))
	}
	if r.later(key, at, reach, upright) {
		return
	}
	slot, ok := r.browserSlot(key, face, sizePx)
	if !ok {
		return
	}
	show := *l
	if k := r.fade(key, at, reach, upright); k < 1 {
		show.color0[3] *= k
	}
	if !key.color {
		r.uses(0)
	}
	shape.C, shape.F = at.X, at.Y
	r.quad(corners(slot.rect(), slot.uv()), shape, 1, &show)
}

// browserSlot returns where a glyph the browser makes sits in its atlas, made or added anew there where it is not.
func (r *Renderer) browserSlot(key glyphKey, face *text.Face, sizePx float32) (glyphSlot, bool) {
	return r.glyph(key, func() text.Mask {
		return r.browserMask(key, func() text.Mask {
			if key.color {
				return face.RasterizeColor(key.id, sizePx)
			}
			return face.Rasterize(key.id, sizePx, float32(key.shift)/subpixel, key.raster)
		})
	})
}

// later reports whether a glyph the browser makes waits for a frame to come, as this one has spent its time on others.
// The frame to come draws it whatever its damage. at is the glyph's origin and reach how far about it its size goes, in
// device pixels, and upright says no rotation has it.
func (r *Renderer) later(key glyphKey, at geom.Point, reach float32, upright bool) bool {
	if r.browser.spent < browserTime || r.shared.has(key) {
		return false
	}
	if r.browser.late == nil || len(r.browser.late) >= browserLate {
		r.browser.late = map[glyphKey]lateGlyph{}
	}
	r.browser.late[key] = lateGlyph{left: r.browser.frame}
	r.drawAgain(at, reach, upright)
	return true
}

// fade returns how much of a glyph the browser made shows: all of it, or less as one that came late fades in.
func (r *Renderer) fade(key glyphKey, at geom.Point, reach float32, upright bool) float32 {
	if len(r.browser.late) == 0 {
		return 1
	}
	late, ok := r.browser.late[key]
	if !ok {
		return 1
	}
	if late.from.IsZero() {
		if late.left+1 < r.browser.frame {
			// It was out of sight in the frame that was to draw it, and is there at once where it comes back.
			delete(r.browser.late, key)
			return 1
		}
		late.from = r.now
		r.browser.late[key] = late
	}
	age := r.now.Sub(late.from)
	if age < 0 || age >= browserFade {
		delete(r.browser.late, key)
		return 1
	}
	r.drawAgain(at, reach, upright)
	return float32(age) / float32(browserFade)
}

// drawAgain has the next frame draw a glyph's place again, whatever its damage, and asks for that frame.
func (r *Renderer) drawAgain(at geom.Point, reach float32, upright bool) {
	w, h := float32(r.fbW), float32(r.fbH)
	area := geom.Rect{Max: geom.Pt(w, h)}
	if upright {
		// Text goes on to the right, as a word of Arabic does from its one glyph.
		area = geom.Rect{Min: geom.Pt(at.X-2*reach, at.Y-2*reach), Max: geom.Pt(max(at.X, w), at.Y+2*reach)}
	}
	r.Stretched = r.Stretched.Union(geom.Rect{Min: area.Min.Mul(1 / r.scale), Max: area.Max.Mul(1 / r.scale)})
	r.SharpAt = r.now
}

// browserReach widens hi, the right end of op's glyphs, for those of the browser's that are words.
func browserReach(op *paint.TextOp, hi float32) float32 {
	if !browserGlyphs {
		return hi
	}
	var face *text.Face
	for i, g := range op.Glyphs {
		if i == 0 || g.Face != op.Glyphs[i-1].Face {
			face, _ = text.Lookup(g.Face)
		}
		if face != nil && face.FromBrowser() {
			hi = max(hi, g.At.X+face.Reach(g.ID)*op.Size)
		}
	}
	return hi
}
