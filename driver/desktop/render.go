//go:build linux || windows || darwin

package desktop

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
)

// GL constants the gl package leaves out.
const (
	glColorBufferBit = 0x4000
	glLinear         = 0x2601
	glUnsignedShort  = 0x1403
	glR8             = 0x8229
	glRed            = 0x1903
)

// A renderer replays a [paint] op list with OpenGL. It lives on one
// window's render thread, with that window's context current.
//
// Every shape is one quad and one signed distance field, so a rounded
// rectangle stays crisp at any size, scale or fractional position. A
// layer draws into an offscreen texture the size of the window and is
// composited back with its opacity and, when it clips, its rounded
// bounds.
//
// Text draws from a glyph atlas: each glyph is rasterized once per
// size and quarter-pixel shift, and every run is a batch of quads.
//
// A layer's Blur and Backdrop are separable Gaussian blurs, run at a
// reduced resolution when the radius is large; see blur.go.
type renderer struct {
	gl gl.Context

	vao, vbo, ibo uint32
	shape         program
	layer         program

	glyphs   atlas
	textProg program
	textVAO  uint32
	textVBO  uint32
	textIBO  uint32
	// quads is the text batch being built, four vertices of x, y, u, v
	// each per glyph, in device pixels.
	quads []float32
	bytes []byte

	// layers holds one offscreen target per nesting depth, reused from
	// frame to frame and resized with the window. layers[0] is the
	// frame itself when the frame is drawn offscreen.
	layers []target
	// offscreen is true for a frame drawn into layers[0] and copied to
	// the window at the end, which a Backdrop needs: it reads what has
	// been drawn so far, and the window's own framebuffer cannot be
	// read back.
	offscreen bool
	blurProg  program
	// blurs holds two scratch targets for each downsampling factor.
	blurs [len(blurFactors)][2]target
	// stack is the targets being drawn into, innermost last, with the
	// op that opened each.
	stack []*paint.LayerOp

	fbW, fbH int
	scale    float32
}

// target is an offscreen texture and the framebuffer that draws to it.
type target struct {
	tex, fbo uint32
	w, h     int
}

// program is a linked shader and its uniform locations.
type program struct {
	id  uint32
	loc map[string]int32
}

func (p program) set4(g gl.Context, name string, v ...float32) {
	switch len(v) {
	case 1:
		g.Uniform1fv(p.loc[name], v)
	case 2:
		g.Uniform2fv(p.loc[name], v)
	case 3:
		g.Uniform3fv(p.loc[name], v)
	default:
		g.Uniform4fv(p.loc[name], v)
	}
}

const vertexShader = `
in vec2 a_pos;
// u_quad is the local rectangle the quad covers, as min and size.
uniform vec4 u_quad;
// u_row0 and u_row1 are the paint transform, in logical pixels.
uniform vec3 u_row0;
uniform vec3 u_row1;
uniform float u_scale;
uniform vec2 u_target;
out vec2 v_local;
// v_uv is where this point falls in a texture the size of the target,
// for passes that read one. It comes from the position the quad is
// drawn at, which holds for any target; gl_FragCoord flips under some
// drivers when one program draws both offscreen and to the window.
out vec2 v_uv;

void main() {
	vec2 local = u_quad.xy + a_pos * u_quad.zw;
	v_local = local;
	vec3 h = vec3(local, 1.0);
	vec2 p = vec2(dot(u_row0, h), dot(u_row1, h)) * u_scale;
	v_uv = vec2(p.x / u_target.x, 1.0 - p.y / u_target.y);
	gl_Position = vec4(p.x / u_target.x * 2.0 - 1.0, 1.0 - p.y / u_target.y * 2.0, 0.0, 1.0);
}
`

const sdfFunc = `
float sdRRect(vec2 p, vec4 rect, float r) {
	vec2 c = (rect.xy + rect.zw) * 0.5;
	vec2 halfSize = (rect.zw - rect.xy) * 0.5;
	r = clamp(r, 0.0, min(halfSize.x, halfSize.y));
	vec2 q = abs(p - c) - halfSize + r;
	return length(max(q, 0.0)) + min(max(q.x, q.y), 0.0) - r;
}

// coverage turns a distance into antialiased coverage, one device pixel
// wide whatever the transform.
float coverage(float d) {
	float w = max(fwidth(d), 1e-4);
	return clamp(0.5 - d / w, 0.0, 1.0);
}

vec4 premul(vec4 c) { return vec4(c.rgb * c.a, c.a); }
`

const shapeShader = `
in vec2 v_local;
uniform vec4 u_rect;
uniform float u_radius;
// u_shadow is 1 when this pass draws the shape's shadow.
uniform float u_shadow;
uniform vec4 u_fill0;
uniform vec4 u_fill1;
// u_grad is the gradient's from and to points; u_useGrad is 1 when the
// fill is a gradient.
uniform vec4 u_grad;
uniform float u_useGrad;
uniform vec4 u_stroke;
uniform float u_strokeWidth;
uniform vec4 u_shadowColor;
// u_shadowGeom is offset.xy, blur, spread.
uniform vec4 u_shadowGeom;
out vec4 fragColor;

void main() {
	if (u_shadow > 0.5) {
		float spread = u_shadowGeom.w;
		vec4 r = u_rect + vec4(-spread, -spread, spread, spread);
		float d = sdRRect(v_local - u_shadowGeom.xy, r, u_radius + spread);
		float blur = max(u_shadowGeom.z, 0.5);
		fragColor = premul(u_shadowColor) * (1.0 - smoothstep(-blur, blur, d));
		return;
	}
	float d = sdRRect(v_local, u_rect, u_radius);
	vec4 fill = u_fill0;
	if (u_useGrad > 0.5) {
		vec2 g = u_grad.zw - u_grad.xy;
		float t = clamp(dot(v_local - u_grad.xy, g) / max(dot(g, g), 1e-6), 0.0, 1.0);
		fill = mix(u_fill0, u_fill1, t);
	}
	vec4 col = premul(fill) * coverage(d);
	if (u_strokeWidth > 0.0) {
		vec4 s = premul(u_stroke) * coverage(abs(d) - u_strokeWidth * 0.5);
		col = s + col * (1.0 - s.a);
	}
	fragColor = col;
}
`

const textVertexShader = `
in vec2 a_pos;
in vec2 a_uv;
uniform vec2 u_target;
out vec2 v_uv;

void main() {
	v_uv = a_uv;
	gl_Position = vec4(a_pos.x / u_target.x * 2.0 - 1.0, 1.0 - a_pos.y / u_target.y * 2.0, 0.0, 1.0);
}
`

const textShader = `
in vec2 v_uv;
uniform sampler2D u_atlas;
uniform vec4 u_color;
out vec4 fragColor;

void main() {
	fragColor = premul(u_color) * texture(u_atlas, v_uv).r;
}
`

const layerShader = `
in vec2 v_local;
in vec2 v_uv;
uniform sampler2D u_tex;
uniform float u_opacity;
uniform vec4 u_rect;
uniform float u_radius;
uniform float u_clip;
out vec4 fragColor;

void main() {
	vec4 c = texture(u_tex, v_uv);
	float cov = 1.0;
	if (u_clip > 0.5) {
		cov = coverage(sdRRect(v_local, u_rect, u_radius));
	}
	fragColor = c * u_opacity * cov;
}
`

func newRenderer(g gl.Context, isES bool) (*renderer, error) {
	header := "#version 150\n"
	if isES {
		header = "#version 300 es\nprecision highp float;\n"
	}
	r := &renderer{gl: g}
	var err error
	if r.shape, err = link(g, header+vertexShader, header+sdfFunc+shapeShader,
		"u_quad", "u_row0", "u_row1", "u_scale", "u_target",
		"u_rect", "u_radius", "u_shadow", "u_fill0", "u_fill1", "u_grad", "u_useGrad",
		"u_stroke", "u_strokeWidth", "u_shadowColor", "u_shadowGeom"); err != nil {
		return nil, err
	}
	if r.layer, err = link(g, header+vertexShader, header+sdfFunc+layerShader,
		"u_quad", "u_row0", "u_row1", "u_scale", "u_target",
		"u_tex", "u_opacity", "u_rect", "u_radius", "u_clip"); err != nil {
		return nil, err
	}

	if r.textProg, err = link(g, header+textVertexShader, header+sdfFunc+textShader,
		"u_target", "u_atlas", "u_color"); err != nil {
		return nil, err
	}
	r.initText()
	if r.blurProg, err = link(g, header+vertexShader, header+blurShader,
		"u_quad", "u_row0", "u_row1", "u_scale", "u_target",
		"u_src", "u_dst", "u_dir", "u_sigma"); err != nil {
		return nil, err
	}

	// One unit quad serves every draw; the vertex shader places it.
	r.vao = g.CreateVertexArray()
	g.BindVertexArray(r.vao)
	r.vbo = g.CreateBuffer()
	g.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	verts := floatBytes(0, 0, 1, 0, 1, 1, 0, 1)
	g.BufferInit(gl.ARRAY_BUFFER, len(verts), gl.STREAM_DRAW)
	g.BufferSubData(gl.ARRAY_BUFFER, 0, verts)
	g.EnableVertexAttribArray(0)
	g.VertexAttribPointer(0, 2, gl.FLOAT, false, 8, 0)
	r.ibo = g.CreateBuffer()
	g.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.ibo)
	idx := []byte{0, 0, 1, 0, 2, 0, 0, 0, 2, 0, 3, 0}
	g.BufferInit(gl.ELEMENT_ARRAY_BUFFER, len(idx), gl.STREAM_DRAW)
	g.BufferSubData(gl.ELEMENT_ARRAY_BUFFER, 0, idx)

	g.Enable(gl.BLEND)
	g.BlendFuncSeparate(gl.ONE, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	return r, nil
}

// link compiles and links a program and looks up its uniforms. The
// vertex position is attribute 0.
func link(g gl.Context, vs, fs string, uniforms ...string) (program, error) {
	v, err := compile(g, gl.VERTEX_SHADER, vs)
	if err != nil {
		return program{}, err
	}
	defer g.DeleteShader(v)
	f, err := compile(g, gl.FRAGMENT_SHADER, fs)
	if err != nil {
		return program{}, err
	}
	defer g.DeleteShader(f)

	id := g.CreateProgram()
	g.AttachShader(id, v)
	g.AttachShader(id, f)
	g.BindAttribLocation(id, 0, "a_pos")
	g.BindAttribLocation(id, 1, "a_uv")
	g.LinkProgram(id)
	if g.GetProgrami(id, gl.LINK_STATUS) == gl.FALSE {
		defer g.DeleteProgram(id)
		return program{}, fmt.Errorf("desktop: link shader: %s", g.GetProgramInfoLog(id))
	}
	p := program{id: id, loc: map[string]int32{}}
	for _, u := range uniforms {
		p.loc[u] = g.GetUniformLocation(id, u)
	}
	return p, nil
}

func compile(g gl.Context, kind uint32, src string) (uint32, error) {
	s := g.CreateShader(kind)
	g.ShaderSource(s, src)
	g.CompileShader(s)
	if g.GetShaderi(s, gl.COMPILE_STATUS) == gl.FALSE {
		defer g.DeleteShader(s)
		return 0, fmt.Errorf("desktop: compile shader: %s", g.GetShaderInfoLog(s))
	}
	return s, nil
}

// release frees the renderer's GL objects.
func (r *renderer) release() {
	g := r.gl
	for _, t := range r.layers {
		g.DeleteFramebuffer(t.fbo)
		g.DeleteTexture(t.tex)
	}
	for _, pair := range r.blurs {
		for _, t := range pair {
			g.DeleteFramebuffer(t.fbo)
			g.DeleteTexture(t.tex)
		}
	}
	g.DeleteProgram(r.blurProg.id)
	g.DeleteTexture(r.glyphs.tex)
	g.DeleteBuffer(r.textVBO)
	g.DeleteBuffer(r.textIBO)
	g.DeleteVertexArray(r.textVAO)
	g.DeleteProgram(r.textProg.id)
	g.DeleteBuffer(r.vbo)
	g.DeleteBuffer(r.ibo)
	g.DeleteVertexArray(r.vao)
	g.DeleteProgram(r.shape.id)
	g.DeleteProgram(r.layer.id)
}

// draw replays ops into the window's framebuffer.
func (r *renderer) draw(ops []paint.Op, fbW, fbH int, scale float32) {
	if fbW <= 0 || fbH <= 0 {
		return
	}
	g := r.gl
	r.fbW, r.fbH, r.scale = fbW, fbH, scale
	r.stack = r.stack[:0]
	r.offscreen = false
	for _, op := range ops {
		if l, ok := op.(*paint.LayerOp); ok && l.Opts.Backdrop > 0 {
			r.offscreen = true
			break
		}
	}
	if r.offscreen {
		if len(r.layers) == 0 {
			r.layers = append(r.layers, target{})
		}
		r.fit(&r.layers[0])
	}
	g.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(0))
	g.Viewport(0, 0, int32(fbW), int32(fbH))
	g.Clear(glColorBufferBit)
	g.BindVertexArray(r.vao)

	for _, op := range ops {
		switch op := op.(type) {
		case *paint.RRectOp:
			r.rrect(op)
		case *paint.LayerOp:
			r.openLayer(op)
		case *paint.LayerEndOp:
			r.closeLayer()
		case *paint.TextOp:
			r.text(op)
		}
	}
	// A layer left open by a node that forgot to close it still shows.
	for len(r.stack) > 0 {
		r.closeLayer()
	}

	if r.offscreen {
		g.BindFramebuffer(gl.FRAMEBUFFER, 0)
		g.Clear(glColorBufferBit)
		r.composite(r.layers[0].tex, nil, 1, false, 0)
	}
}

// fbo returns the framebuffer for nesting depth d: the window, or the
// offscreen frame, at depth 0, and a layer's target beneath it.
func (r *renderer) fbo(d int) uint32 {
	if d == 0 && !r.offscreen {
		return 0
	}
	return r.layers[d].fbo
}

// common sets the uniforms both programs share.
func (r *renderer) common(p program, quad geom.Rect, t paint.Transform) {
	g := r.gl
	s := quad.Size()
	p.set4(g, "u_quad", quad.Min.X, quad.Min.Y, s.W, s.H)
	p.set4(g, "u_row0", t.A, t.B, t.C)
	p.set4(g, "u_row1", t.D, t.E, t.F)
	p.set4(g, "u_scale", r.scale)
	p.set4(g, "u_target", float32(r.fbW), float32(r.fbH))
}

func (r *renderer) quad() {
	r.gl.DrawElements(gl.TRIANGLES, 6, glUnsignedShort, 0)
}

func (r *renderer) rrect(op *paint.RRectOp) {
	g := r.gl
	p := r.shape
	g.UseProgram(p.id)
	p.set4(g, "u_rect", op.Rect.Min.X, op.Rect.Min.Y, op.Rect.Max.X, op.Rect.Max.Y)
	p.set4(g, "u_radius", op.Radius)

	// The shadow goes first, underneath, on a quad grown to hold it.
	if sh := op.Shadow; sh.Color.A > 0 {
		grow := sh.Blur + sh.Spread + 2
		quad := grow4(op.Rect.Add(sh.Offset), grow)
		r.common(p, quad, op.Transform)
		p.set4(g, "u_shadow", 1)
		p.set4(g, "u_shadowColor", rgba(sh.Color)...)
		p.set4(g, "u_shadowGeom", sh.Offset.X, sh.Offset.Y, sh.Blur, sh.Spread)
		r.quad()
	}

	fill0, fill1 := rgba(op.Fill.Solid), rgba(op.Fill.Solid)
	useGrad := float32(0)
	grad := []float32{0, 0, 0, 0}
	if gr := op.Fill.Gradient; gr != nil {
		fill0, fill1 = rgba(gr.Start), rgba(gr.End)
		grad = []float32{gr.From.X, gr.From.Y, gr.To.X, gr.To.Y}
		useGrad = 1
	}
	if fill0[3] == 0 && fill1[3] == 0 && (op.Stroke.Width <= 0 || op.Stroke.Color.A == 0) {
		return
	}
	r.common(p, grow4(op.Rect, op.Stroke.Width/2+2), op.Transform)
	p.set4(g, "u_shadow", 0)
	p.set4(g, "u_fill0", fill0...)
	p.set4(g, "u_fill1", fill1...)
	p.set4(g, "u_grad", grad...)
	p.set4(g, "u_useGrad", useGrad)
	p.set4(g, "u_stroke", rgba(op.Stroke.Color)...)
	p.set4(g, "u_strokeWidth", op.Stroke.Width)
	r.quad()
}

// openLayer starts drawing into a fresh offscreen target.
func (r *renderer) openLayer(op *paint.LayerOp) {
	depth := len(r.stack) + 1
	for len(r.layers) <= depth {
		r.layers = append(r.layers, target{})
	}
	t := &r.layers[depth]
	r.fit(t)
	r.stack = append(r.stack, op)
	g := r.gl
	g.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	g.Clear(glColorBufferBit)
}

// closeLayer composites the innermost layer into the one around it.
//
// A Backdrop goes first: what the parent holds so far is blurred and
// drawn back over itself within the layer's bounds, rounded when the
// layer clips. The layer's contents go on top, blurred first when the
// layer asks for Blur.
func (r *renderer) closeLayer() {
	depth := len(r.stack)
	op := r.stack[depth-1]
	r.stack = r.stack[:depth-1]
	o := op.Opts
	radius := float32(0)
	if o.Clip {
		radius = o.Radius
	}

	if o.Backdrop > 0 && (depth > 1 || r.offscreen) {
		behind := r.blur(r.layers[depth-1].tex, r.region(op, true), o.Backdrop*r.scale)
		r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(depth-1))
		r.composite(behind, op, o.Opacity, true, radius)
	}

	contents := r.layers[depth].tex
	if o.Blur > 0 {
		contents = r.blur(contents, r.region(op, o.Clip), o.Blur*r.scale)
	}
	r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(depth-1))
	r.composite(contents, op, o.Opacity, o.Clip, radius)
}

// composite draws tex, a window-sized texture, into the bound target at
// opacity. With clip it covers op's bounds, rounded by radius; without,
// the whole window. A nil op composites the whole window unclipped.
func (r *renderer) composite(tex uint32, op *paint.LayerOp, opacity float32, clip bool, radius float32) {
	g := r.gl
	p := r.layer
	g.UseProgram(p.id)
	g.BindVertexArray(r.vao)
	g.ActiveTexture(gl.TEXTURE0)
	g.BindTexture(gl.TEXTURE_2D, tex)
	g.Uniform1i(p.loc["u_tex"], 0)
	p.set4(g, "u_opacity", opacity)
	p.set4(g, "u_radius", radius)
	if clip && op != nil {
		b := op.Opts.Bounds
		p.set4(g, "u_rect", b.Min.X, b.Min.Y, b.Max.X, b.Max.Y)
		p.set4(g, "u_clip", 1)
		r.common(p, grow4(b, 2), op.Transform)
	} else {
		p.set4(g, "u_clip", 0)
		r.common(p, r.window(), paint.Identity)
	}
	r.quad()
}

// window returns the whole window in logical pixels.
func (r *renderer) window() geom.Rect {
	return geom.Rect{Max: geom.Pt(float32(r.fbW)/r.scale, float32(r.fbH)/r.scale)}
}

// region returns the device-pixel rectangle a layer covers: its bounds
// under its transform when bounded, or the whole window.
func (r *renderer) region(op *paint.LayerOp, bounded bool) geom.Rect {
	if !bounded {
		return geom.Rect{Max: geom.Pt(float32(r.fbW), float32(r.fbH))}
	}
	b, t := op.Opts.Bounds, op.Transform
	first := t.Apply(b.Min)
	out := geom.Rect{Min: first, Max: first}
	for _, c := range []geom.Point{{X: b.Max.X, Y: b.Min.Y}, b.Max, {X: b.Min.X, Y: b.Max.Y}} {
		p := t.Apply(c)
		out.Min = geom.Pt(min(out.Min.X, p.X), min(out.Min.Y, p.Y))
		out.Max = geom.Pt(max(out.Max.X, p.X), max(out.Max.Y, p.Y))
	}
	return geom.Rect{Min: out.Min.Mul(r.scale), Max: out.Max.Mul(r.scale)}
}

// fit sizes t to the window, creating it on first use.
func (r *renderer) fit(t *target) { r.fitSize(t, r.fbW, r.fbH) }

// fitSize sizes t to w by h pixels, creating it on first use.
func (r *renderer) fitSize(t *target, w, h int) {
	g := r.gl
	if t.tex != 0 && t.w == w && t.h == h {
		return
	}
	if t.tex == 0 {
		t.tex = g.CreateTexture()
		t.fbo = g.CreateFramebuffer()
	}
	t.w, t.h = w, h
	g.BindTexture(gl.TEXTURE_2D, t.tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(t.w), int32(t.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
	g.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	g.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.tex, 0)
}

func grow4(r geom.Rect, by float32) geom.Rect {
	return geom.Rect{
		Min: geom.Pt(r.Min.X-by, r.Min.Y-by),
		Max: geom.Pt(r.Max.X+by, r.Max.Y+by),
	}
}

func rgba(c color.NRGBA) []float32 {
	return []float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255, float32(c.A) / 255}
}

func floatBytes(vs ...float32) []byte {
	b := make([]byte, 0, len(vs)*4)
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	return b
}
