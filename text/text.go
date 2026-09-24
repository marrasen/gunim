// Package text shapes strings into positioned glyphs and rasterizes
// those glyphs for a driver to draw.
//
// Shaping is the step that turns a string and a font into glyphs: it
// picks each glyph, applies kerning and ligatures, and places them. It
// runs on the UI goroutine during layout, and its result is a [Run]
// that measures itself and paints itself. Rasterizing turns one glyph
// into a coverage mask at one device size, and runs on a driver's
// render thread when a glyph first appears.
//
// Both are pure Go, built on go-text/typesetting, the HarfBuzz port
// that Gio and Ebitengine also use.
package text

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A Face is a font, ready to shape and rasterize. It is safe for
// concurrent use: the UI goroutine shapes with it while render threads
// rasterize from it.
type Face struct {
	id   uint32
	upem float32
	// ascent and descent are the font's line extents in font units,
	// both positive.
	ascent, descent, gap float32

	mu     sync.Mutex
	face   *font.Face
	shaper shaping.HarfbuzzShaper
}

var (
	facesMu sync.RWMutex
	faces   []*Face

	defaultOnce sync.Once
	defaultFace *Face
)

// Parse reads a TrueType or OpenType font.
func Parse(data []byte) (*Face, error) {
	ff, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("text: %w", err)
	}
	f := &Face{face: ff, upem: float32(ff.Upem())}
	if ext, ok := ff.FontHExtents(); ok {
		f.ascent, f.descent, f.gap = ext.Ascender, -ext.Descender, ext.LineGap
	} else {
		f.ascent, f.descent = 0.8*f.upem, 0.2*f.upem
	}

	facesMu.Lock()
	f.id = uint32(len(faces))
	faces = append(faces, f)
	facesMu.Unlock()
	return f, nil
}

// Default returns Go Regular, the face gunim uses when a widget names
// none.
func Default() *Face {
	defaultOnce.Do(func() {
		f, err := Parse(goregular.TTF)
		if err != nil {
			panic("text: the built-in Go Regular font fails to parse: " + err.Error())
		}
		defaultFace = f
	})
	return defaultFace
}

// Lookup returns the face a [paint.Glyph] names. A driver calls it to
// rasterize the glyphs of a [paint.TextOp].
func Lookup(id uint32) (*Face, bool) {
	facesMu.RLock()
	defer facesMu.RUnlock()
	if int(id) >= len(faces) {
		return nil, false
	}
	return faces[id], true
}

// A Run is one line of shaped text, laid out along a baseline that
// starts at the origin.
type Run struct {
	Face *Face
	// Size is the font size in logical pixels.
	Size float32
	// Glyphs are positioned relative to the start of the baseline, with
	// y growing downward.
	Glyphs []paint.Glyph
	// Advance is how far the pen moved: the run's width.
	Advance float32
	// Ascent and Descent are the line's extents above and below the
	// baseline, both positive.
	Ascent, Descent float32
}

// Height returns the line's height, ascent plus descent.
func (r Run) Height() float32 { return r.Ascent + r.Descent }

// Box returns the run's size: its advance by its height.
func (r Run) Box() geom.Size { return geom.Sz(r.Advance, r.Height()) }

// Paint draws the run with the top-left of its line box at topLeft.
func (r Run) Paint(p *paint.Painter, topLeft geom.Point, c color.NRGBA) {
	if len(r.Glyphs) == 0 {
		return
	}
	defer p.Push(paint.Translate(geom.Pt(topLeft.X, topLeft.Y+r.Ascent)))()
	p.Text(r.Glyphs, r.Size, c, geom.Rect{
		Min: geom.Pt(0, -r.Ascent),
		Max: geom.Pt(r.Advance, r.Descent),
	})
}

// Shape lays s out as one line at size logical pixels.
//
// The script comes from the first letter that has one, which covers
// single-script labels. Mixed scripts, right-to-left text and line
// breaking arrive with paragraph layout.
func (f *Face) Shape(s string, size float32) Run {
	scale := size / f.upem
	run := Run{Face: f, Size: size, Ascent: f.ascent * scale, Descent: f.descent * scale}
	if s == "" {
		return run
	}
	runes := []rune(s)
	in := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: di.DirectionLTR,
		Face:      f.face,
		Size:      fixed.Int26_6(math.Round(float64(size) * 64)),
		Script:    scriptOf(runes),
		Language:  language.DefaultLanguage(),
	}

	f.mu.Lock()
	out := f.shaper.Shape(in)
	f.mu.Unlock()

	run.Glyphs = make([]paint.Glyph, 0, len(out.Glyphs))
	var pen float32
	for _, g := range out.Glyphs {
		run.Glyphs = append(run.Glyphs, paint.Glyph{
			ID:   uint32(g.GlyphID),
			At:   geom.Pt(pen+fromFixed(g.XOffset), -fromFixed(g.YOffset)),
			Face: f.id,
		})
		pen += fromFixed(g.Advance)
	}
	run.Advance = pen
	return run
}

func scriptOf(runes []rune) language.Script {
	for _, r := range runes {
		if s := language.LookupScript(r); s != language.Common && s != language.Inherited && s != language.Unknown {
			return s
		}
	}
	return language.Latin
}

func fromFixed(v fixed.Int26_6) float32 { return float32(v) / 64 }

// A Mask is one glyph's coverage at one device size: 0 outside the
// glyph, 255 inside, and antialiased in between.
type Mask struct {
	// Pix holds W*H coverage bytes, row by row.
	Pix  []byte
	W, H int
	// Offset is where the mask's top-left corner sits relative to the
	// glyph's origin on the baseline, in device pixels, y down.
	Offset image.Point
}

// Rasterize renders glyph id at sizePx device pixels, with its origin
// shifted right by dx, a fraction of a pixel. A driver keeps a few
// shifts of each glyph so text can sit at any fractional position and
// still look the same.
//
// A glyph with no outline, such as a space, returns an empty mask.
func (f *Face) Rasterize(id uint32, sizePx, dx float32) Mask {
	f.mu.Lock()
	outline, ok := f.face.GlyphDataOutline(font.GID(id))
	f.mu.Unlock()
	if !ok || len(outline.Segments) == 0 {
		return Mask{}
	}

	scale := sizePx / f.upem
	pt := func(p font.SegmentPoint) (float32, float32) { return p.X*scale + dx, -p.Y * scale }

	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, seg := range outline.Segments {
		for _, a := range seg.ArgsSlice() {
			x, y := pt(a)
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	x0, y0 := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	x1, y1 := int(math.Ceil(float64(maxX))), int(math.Ceil(float64(maxY)))
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		return Mask{}
	}

	r := vector.NewRasterizer(w, h)
	local := func(p font.SegmentPoint) (float32, float32) {
		x, y := pt(p)
		return x - float32(x0), y - float32(y0)
	}
	// MoveTo leaves the previous contour open, so each one is closed
	// before the next begins.
	open := false
	for _, seg := range outline.Segments {
		switch seg.Op {
		case opentype.SegmentOpMoveTo:
			if open {
				r.ClosePath()
			}
			open = true
			r.MoveTo(local(seg.Args[0]))
		case opentype.SegmentOpLineTo:
			r.LineTo(local(seg.Args[0]))
		case opentype.SegmentOpQuadTo:
			bx, by := local(seg.Args[0])
			cx, cy := local(seg.Args[1])
			r.QuadTo(bx, by, cx, cy)
		case opentype.SegmentOpCubeTo:
			bx, by := local(seg.Args[0])
			cx, cy := local(seg.Args[1])
			dx, dy := local(seg.Args[2])
			r.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	r.ClosePath()
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	return Mask{Pix: dst.Pix, W: w, H: h, Offset: image.Pt(x0, y0)}
}
