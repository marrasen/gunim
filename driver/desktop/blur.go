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

const blurShader = `
in vec2 v_local;
in vec2 v_uv;
uniform sampler2D u_src;
// u_dst is the size in pixels of the target this pass draws into.
uniform vec2 u_dst;
// u_dir is (1, 0) for the horizontal pass and (0, 1) for the vertical.
uniform vec2 u_dir;
// u_sigma is the standard deviation, in pixels of the target.
uniform float u_sigma;
out vec4 fragColor;

void main() {
	vec2 uv = v_uv;
	vec2 texel = u_dir / u_dst;
	float s = max(u_sigma, 0.0001);
	int n = min(int(ceil(3.0 * s)), 64);
	vec4 acc = texture(u_src, uv);
	float total = 1.0;
	for (int i = 1; i <= n; i++) {
		float w = exp(-0.5 * float(i * i) / (s * s));
		acc += w * (texture(u_src, uv + texel * float(i)) + texture(u_src, uv - texel * float(i)));
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
	p := r.blurProg
	g.UseProgram(p.id)
	g.BindVertexArray(r.vao)
	r.common(p, r.window(), paint.Identity)
	p.set4(g, "u_dst", float32(w), float32(h))
	p.set4(g, "u_sigma", s)
	g.Uniform1i(p.loc["u_src"], 0)
	g.ActiveTexture(gl.TEXTURE0)
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
		g.BindTexture(gl.TEXTURE_2D, pass.src)
		p.set4(g, "u_dir", pass.dir[0], pass.dir[1])
		r.quad()
	}

	g.Disable(gl.SCISSOR_TEST)
	g.Enable(gl.BLEND)
	g.Viewport(0, 0, int32(r.fbW), int32(r.fbH))
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
