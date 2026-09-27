//go:build linux || windows || darwin

package desktop

import (
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
)

// blurFactors are the resolutions a blur runs at, as divisors of the
// window's. A wide blur runs at a quarter of the resolution, where it
// costs a sixteenth as much and looks the same, since it removes the
// detail the lower resolution drops.
var blurFactors = [...]int{1, 2, 4}

// maxTaps is the most samples the blur shader takes on each side of a
// pixel; it covers three standard deviations up to a sigma of about 21
// pixels at the resolution the blur runs at.
const maxTaps = 64

// blurShader reads its source on unit 1. v_extra.xy is one texel of
// the target along the pass's direction, and v_extra.z the standard
// deviation in the target's pixels.
const blurShader = `
in vec2 v_uv;
in vec4 v_extra;
uniform sampler2D u_tex;
out vec4 fragColor;

void main() {
	vec2 uv = v_uv;
	vec2 texel = v_extra.xy;
	float s = max(v_extra.z, 0.0001);
	int n = min(int(ceil(3.0 * s)), 64);
	vec4 acc = texture(u_tex, uv);
	float total = 1.0;
	for (int i = 1; i <= n; i++) {
		float w = exp(-0.5 * float(i * i) / (s * s));
		acc += w * (texture(u_tex, uv + texel * float(i)) + texture(u_tex, uv - texel * float(i)));
		total += 2.0 * w;
	}
	fragColor = acc / total;
}
`

// blur returns a window-sized texture holding src blurred with a
// Gaussian of standard deviation sigma device pixels, correct within
// region, a device-pixel rectangle. Sampling it with window-normalized
// coordinates works whatever resolution it ran at.
//
// The result lives in scratch targets that the next blur at the same
// resolution overwrites.
func (r *renderer) blur(src uint32, region geom.Rect, sigma float32) uint32 {
	fi := 0
	for fi < len(blurFactors)-1 && sigma/float32(blurFactors[fi]) > 8 {
		fi++
	}
	k := blurFactors[fi]
	w, h := (r.fbW+k-1)/k, (r.fbH+k-1)/k
	pair := &r.blurs[fi]
	r.fitSize(&pair[0], w, h)
	r.fitSize(&pair[1], w, h)
	s := sigma / float32(k)
	reach := float32(math.Ceil(float64(min(3*s, maxTaps)))) + 1

	g := r.gl
	r.flush()
	g.UseProgram(r.blurProg.id)
	g.Viewport(0, 0, int32(w), int32(h))
	g.Disable(gl.BLEND)
	g.Enable(gl.SCISSOR_TEST)

	// The vertical pass reads the horizontal pass up to reach beyond
	// the region, so the horizontal pass covers twice as far.
	scaled := geom.Rect{Min: region.Min.Mul(1 / float32(k)), Max: region.Max.Mul(1 / float32(k))}
	passes := [2]struct {
		src, dst uint32
		dir      [2]float32
		grow     float32
	}{
		{src, pair[0].fbo, [2]float32{1, 0}, 2 * reach},
		{pair[0].tex, pair[1].fbo, [2]float32{0, 1}, reach},
	}
	for _, pass := range passes {
		sx, sy, sw, sh := scissor(grow4(scaled, pass.grow), w, h)
		g.Scissor(sx, sy, sw, sh)
		g.BindFramebuffer(gl.FRAMEBUFFER, pass.dst)
		r.uses(pass.src)
		r.quad(corners(r.window(), geom.Rect{}), paint.Identity, r.scale, &look{
			extra: [4]float32{pass.dir[0] / float32(w), pass.dir[1] / float32(h), s, 0},
		})
		r.flush()
	}

	r.applyClip()
	g.Enable(gl.BLEND)
	g.Viewport(0, 0, int32(r.fbW), int32(r.fbH))
	g.UseProgram(r.drawProg.id)
	return pair[1].tex
}

// scissor turns a rectangle with its origin at the top left into GL's
// scissor box, with its origin at the bottom left, clamped to a w by h
// target.
func scissor(rc geom.Rect, w, h int) (x, y, sw, sh int32) {
	x0 := max(0, int(math.Floor(float64(rc.Min.X))))
	y0 := max(0, int(math.Floor(float64(rc.Min.Y))))
	x1 := min(w, int(math.Ceil(float64(rc.Max.X))))
	y1 := min(h, int(math.Ceil(float64(rc.Max.Y))))
	if x1 <= x0 || y1 <= y0 {
		return 0, 0, 0, 0
	}
	return int32(x0), int32(h - y1), int32(x1 - x0), int32(y1 - y0)
}
