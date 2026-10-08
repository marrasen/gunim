//go:build linux || windows || darwin

package render

import (
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
)

const (
	// imageIdle is how long a texture stays after the last frame that
	// drew its image.
	imageIdle = 10 * time.Second
	// imageBudget is how many bytes of image textures a window keeps
	// before it lets go of the ones it drew longest ago.
	imageBudget = 256 << 20
)

// imageTexture is an image uploaded to the GPU.
type imageTexture struct {
	tex   uint32
	bytes int
	used  time.Time
}

// image queues an image, uploading it on first use.
func (r *Renderer) image(op *paint.ImageOp) {
	iw, ih := op.Image.Size()
	if iw == 0 || ih == 0 || op.Opacity <= 0 {
		return
	}
	tex := r.texture(op.Image)
	r.uses(tex)
	src := op.Src
	if src.Empty() {
		src = geom.Rc(0, 0, float32(iw), float32(ih))
	}
	uv := geom.Rect{
		Min: geom.Pt(src.Min.X/float32(iw), src.Min.Y/float32(ih)),
		Max: geom.Pt(src.Max.X/float32(iw), src.Max.Y/float32(ih)),
	}
	r.quad(corners(op.Rect, uv), op.Transform, r.scale, &look{
		rect: op.Rect, radius: op.Radius, kind: kindImage, add: op.Blend == paint.BlendAdd,
		color0: [4]float32{0, 0, 0, op.Opacity},
	})
}

// texture returns img's texture, uploading it with mipmaps on first
// use, so it stays smooth drawn at a fraction of its size.
func (r *Renderer) texture(img *paint.Image) uint32 {
	now := time.Now()
	if t, ok := r.images[img]; ok {
		t.used = now
		return t.tex
	}
	w, h := img.Size()
	g := r.GL
	t := &imageTexture{tex: g.CreateTexture(), bytes: w * h * 4 * 4 / 3, used: now}
	g.ActiveTexture(glTexture1)
	g.BindTexture(gl.TEXTURE_2D, t.tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinearMipmapLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, img.Pix())
	g.GenerateMipmap(gl.TEXTURE_2D)
	g.ActiveTexture(gl.TEXTURE0)
	// The batch may hold a quad reading whatever was on unit 1; flush
	// binds it again.
	r.images[img] = t
	return t.tex
}

// evictImages lets go of textures the window has stopped drawing, and
// of the least recently drawn when they fill the budget.
func (r *Renderer) evictImages() {
	now := time.Now()
	total := 0
	for m, t := range r.images {
		if now.Sub(t.used) > imageIdle {
			r.dropImage(m)
			continue
		}
		total += t.bytes
	}
	for total > imageBudget {
		var oldest *paint.Image
		for m, t := range r.images {
			if oldest == nil || t.used.Before(r.images[oldest].used) {
				oldest = m
			}
		}
		total -= r.images[oldest].bytes
		r.dropImage(oldest)
	}
}

func (r *Renderer) dropImage(m *paint.Image) {
	r.GL.DeleteTexture(r.images[m].tex)
	delete(r.images, m)
}
