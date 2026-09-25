//go:build linux || windows || darwin

package desktop

import (
	"image"
	"math"
	"sync"
	"time"

	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/text"
)

// shared is what every window's renderer shares: the shader programs,
// the glyph atlas as the CPU sees it, and the redraw policy, since how
// fast this machine draws is the same for every window.
//
// A program is compiled once, by the first render thread to need it,
// and used by all of them, which saves each new window, and every popup,
// the time it takes to compile. That is safe because the programs hold
// no state a renderer changes: everything a draw needs travels with
// its vertices, and the samplers are set once, as the programs are
// linked. Programs belong to the share group every window's context is
// in.
//
// The glyph atlas is shared as packing and pixels. Each glyph is
// rasterized once for every window, and each window's renderer copies
// the glyphs it draws into a texture of its own. Changing one texture
// from several threads at once would need fences between them; copying
// into textures of their own needs none.
type shared struct {
	mu     sync.Mutex
	built  bool
	err    error
	draw   program
	blur   program
	atlas  sharedAtlas
	policy redrawPolicy
}

// redraw reports whether the redraw policy is decided, and whether it
// has settled on drawing every frame in full.
func (s *shared) redraw() (decided, fullOnly bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policy.decided, s.policy.fullOnly()
}

// timeFrame records how long a frame took, drawn in part or in full.
func (s *shared) timeFrame(partial bool, took time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.policy.decided {
		s.policy.add(partial, took)
	}
}

// programs returns the shared programs, building them on first use
// with the calling thread's context.
func (s *shared) programs(g gl.Context, isES bool) (draw, blur program, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.built {
		s.built = true
		s.draw, s.blur, s.err = buildPrograms(g, isES)
		// The other threads may use them as soon as the lock goes, so
		// they must be complete in the share group by then.
		g.Finish()
	}
	return s.draw, s.blur, s.err
}

// sharedAtlas packs glyph masks into an atlasSize square, in shelves.
// When it fills, it starts again from empty and moves to a new epoch,
// which tells each renderer to clear its texture.
type sharedAtlas struct {
	x, y, rowH int
	epoch      int
	slots      map[glyphKey]sharedSlot
}

// sharedSlot is a glyph's place in the atlas and its mask.
type sharedSlot struct {
	glyphSlot
	pix []byte
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

// glyph returns a glyph's place in the atlas, rasterizing it on first
// use, and the atlas's epoch. It reports false for a glyph with nothing
// to draw.
func (s *shared) glyph(key glyphKey, face *text.Face, sizePx float32) (sharedSlot, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := &s.atlas
	if a.slots == nil {
		a.slots, a.epoch = map[glyphKey]sharedSlot{}, 1
	}
	if slot, ok := a.slots[key]; ok {
		return slot, a.epoch, slot.w > 0
	}
	m := face.Rasterize(key.id, sizePx, float32(key.shift)/subpixel)
	if m.W == 0 || m.W+1 > atlasSize || m.H+1 > atlasSize {
		a.slots[key] = sharedSlot{}
		return sharedSlot{}, a.epoch, false
	}
	if a.x+m.W+1 > atlasSize {
		a.x, a.y, a.rowH = 0, a.y+a.rowH, 0
	}
	if a.y+m.H+1 > atlasSize {
		clear(a.slots)
		a.x, a.y, a.rowH = 0, 0, 0
		a.epoch++
	}
	slot := sharedSlot{glyphSlot: glyphSlot{x: a.x, y: a.y, w: m.W, h: m.H, off: m.Offset}, pix: m.Pix}
	a.x += m.W + 1
	a.rowH = max(a.rowH, m.H+1)
	a.slots[key] = slot
	return slot, a.epoch, true
}

// glyphKeyFor returns the atlas key for a glyph at a device size and
// subpixel shift.
func glyphKeyFor(face, id uint32, sizePx float32, shift uint8) glyphKey {
	return glyphKey{face: face, id: id, size: int32(math.Round(float64(sizePx) * 64)), shift: shift}
}
