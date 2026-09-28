//go:build linux || windows || darwin

package desktop

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"math"
	"os"
	"unsafe"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// GL constants the gl package leaves out.
const (
	glColorBufferBit     = 0x4000
	glLinear             = 0x2601
	glLinearMipmapLinear = 0x2703
	glUnsignedShort      = 0x1403
	glR8                 = 0x8229
	glRed                = 0x1903
	glRGB8               = 0x8051
	glRGB                = 0x1907
	glStaticDraw         = 0x88E4
	glTexture1           = 0x84C1
	glTexture2           = 0x84C2
	glOneMinusSrc1Color  = 0x88FA
	glOneMinusSrc1Alpha  = 0x88FB
)

// A renderer replays a [paint] op list with OpenGL. It lives on one
// window's render thread, with that window's context current.
//
// Every shape is one quad and one signed distance field, so a rounded
// rectangle stays crisp at any size, scale or fractional position.
// Shapes, shadows, glyphs, images and finished layers all go through
// one program, and each quad carries everything its pixels need in its
// vertices: the transform is applied on the CPU, and the colours and
// geometry ride along. So a run of ops of any of those kinds is one
// draw call. A batch ends when the frame moves to another target, when
// it needs another image, or when it is full.
//
// A layer draws into an offscreen texture the size of the window and
// is composited back with its opacity and, when it clips, its rounded
// bounds. A layer's Blur and Backdrop are separable Gaussian blurs, run
// at a reduced resolution when the radius is large; see blur.go.
// An opaque layer that clips to an upright rectangle, or not at all,
// draws in place instead, under a scissor.
//
// Text draws from a glyph atlas: each glyph is rasterized once per size
// and quarter-pixel shift; see glyphs.go. Masks, such as icons, share
// the greyscale atlas; see masks.go. Images upload once and stay
// on the GPU while frames use them; see images.go.
type renderer struct {
	gl     gl.Context
	shared *shared

	vao, vbo, ibo uint32
	drawProg      program
	blurProg      program

	// verts is the batch being built: vertFloats floats a vertex, four
	// vertices a quad. tex is the texture the batch's images or layers
	// read, bound to unit 1 as it draws, or 0 for none.
	verts []float32
	tex   uint32
	// draws counts draw calls, for tests and benchmarks.
	draws int

	glyphs, lcdGlyphs glyphTexture
	images            map[*paint.Image]*imageTexture
	// scratchX is where the next mask goes in the scratch strip, and scratches counts the masks put there, for tests.
	scratchX, scratches int
	// dual says the draw program blends each channel by a colour of its
	// own, which glyphs on subpixels need.
	dual bool
	// textRendering is how the window draws text, and subpixels says
	// its glyphs may use the panel's subpixels: it asks for them, its
	// surface is opaque, and the program blends by channel. gamma holds
	// the ratios the shader corrects coverage by.
	textRendering text.Rendering
	subpixels     bool
	gamma         [4]float32

	// layers holds one offscreen target per nesting depth, reused from
	// frame to frame and resized with the window. layers[0] is the
	// canvas: the frame is drawn there and copied to the window.
	//
	// The canvas keeps the last frame, so a frame redraws only the part
	// that changed, and the copy puts the whole of it on screen. A
	// Backdrop needs it too: it reads what has been drawn so far, and
	// the window's own framebuffer cannot be read back.
	layers []target
	// canvasOK says the canvas holds the last frame, at canvasScale.
	canvasOK    bool
	canvasScale float32
	// direct is set for a frame drawn straight to the window, which is
	// quicker when most of it changed: it saves the copy. big says the
	// frame before changed most of the window too.
	direct, big bool
	// redrawn is the device-pixel area the last frame redrew, for
	// tests.
	redrawn geom.Rect
	// under is the window's background, where a frame's first op leaves
	// it uncovered; see driver.Backgrounder.
	under color.NRGBA
	// corner is the radius, in device pixels, the window's corners are
	// cut to on the screen, or 0 for square corners, and edge the width
	// of the edge a cut window leaves for the border its drawn shadow
	// draws; see shape.
	corner, edge float32
	// cull is the device-pixel area a frame redraws in part, while its
	// commands are queued, and empty otherwise. A quad drawn straight
	// to the canvas wholly outside it is left out: the scissor would
	// throw all of it away, after it had been sent and set up.
	cull geom.Rect
	// windowFBO is the framebuffer the window shows: 0, or on Windows
	// the texture DXGI presents. flipWindow is set for that texture,
	// whose rows Direct3D reads from the top where OpenGL writes them
	// from the bottom, so the canvas goes to it upside down.
	windowFBO  uint32
	flipWindow bool
	// blurs holds two scratch targets for each downsampling factor.
	blurs [len(blurFactors)][2]target
	// stack is the layers open, innermost last. depth is the target
	// being drawn into: 0 for the canvas, or the window for a frame
	// drawn straight to it, and layers[depth] beneath. clip is the
	// device-pixel box drawing is scissored to, with its origin at the
	// top left.
	stack []openLayer
	depth int
	clip  geom.Rect

	fbW, fbH int
	scale    float32
}

// target is an offscreen texture and the framebuffer that draws to it.
type target struct {
	tex, fbo uint32
	w, h     int
}

// program is a linked shader.
type program struct{ id uint32 }

// openLayer is a layer being drawn: the op that opened it, whether it
// draws in place, straight into the target around it, and the clip
// around it.
type openLayer struct {
	op      *paint.LayerOp
	inPlace bool
	clip    geom.Rect
}

// Each vertex is vertFloats floats, in eight attributes of two or four:
//
//	a_pos    where the vertex lands, in normalized device coordinates
//	a_local  the point in the shape's own space, for its distance field
//	a_rect   the shape's rectangle in its own space
//	a_param  corner radius, stroke width, kind, and a flag; for a glyph,
//	         its subpixel order and contrast in place of the first two
//	a_color0 the fill, the shadow's colour or the glyph's; for an image
//	         or a layer, the opacity in alpha
//	a_color1 the gradient's end colour, or a glyph's gamma ratios
//	a_extra  the gradient's ends; the shadow's offset, blur and spread;
//	         or texture coordinates
//	a_stroke the stroke's colour
const vertFloats = 28

// The kinds of quad, in a_param.z.
const (
	kindShape = iota
	kindShadow
	kindGlyph
	kindImage
	kindLayer
)

// maxQuads is the most quads one draw call carries: as many as 16-bit
// indices reach.
const maxQuads = 1 << 14

var attribs = [...]struct {
	name string
	size int32
}{
	{"a_pos", 2}, {"a_local", 2}, {"a_rect", 4}, {"a_param", 4},
	{"a_color0", 4}, {"a_color1", 4}, {"a_extra", 4}, {"a_stroke", 4},
}

const vertexShader = `
in vec2 a_pos;
in vec2 a_local;
in vec4 a_rect;
in vec4 a_param;
in vec4 a_color0;
in vec4 a_color1;
in vec4 a_extra;
in vec4 a_stroke;
out vec2 v_local;
flat out vec4 v_rect;
flat out vec4 v_param;
flat out vec4 v_color0;
flat out vec4 v_color1;
out vec4 v_extra;
flat out vec4 v_stroke;
// v_uv is where this point falls in a texture the size of the target,
// for passes that read one. It comes from the position the quad is
// drawn at, which holds for any target; gl_FragCoord flips under some
// drivers when one program draws both offscreen and to the window.
out vec2 v_uv;

void main() {
	v_local = a_local;
	v_rect = a_rect;
	v_param = a_param;
	v_color0 = a_color0;
	v_color1 = a_color1;
	v_extra = a_extra;
	v_stroke = a_stroke;
	v_uv = a_pos * 0.5 + 0.5;
	gl_Position = vec4(a_pos, 0.0, 1.0);
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

// drawOut declares the draw program's outputs: its colour and, where
// the program blends by channel, how much of what is behind each
// channel of it hides.
const drawOut = `
#ifdef DUAL
layout(location = 0, index = 0) out vec4 fragColor;
layout(location = 0, index = 1) out vec4 fragCover;
#else
out vec4 fragColor;
#endif
`

const drawShader = `
in vec2 v_local;
flat in vec4 v_rect;
flat in vec4 v_param;
flat in vec4 v_color0;
flat in vec4 v_color1;
in vec4 v_extra;
flat in vec4 v_stroke;
in vec2 v_uv;
uniform sampler2D u_atlas;
uniform sampler2D u_lcd;
uniform sampler2D u_tex;

// glyph returns a glyph's colour, with its coverage enhanced by the
// contrast in v_param.y and corrected for gamma by the ratios in
// v_color1. v_param.x is 0 for a greyscale glyph, and 1 or 2 for one
// on subpixels that run red to blue or blue to red; cover gets the
// coverage of each channel.
vec4 glyph(out vec4 cover) {
	vec4 c = v_color0;
	vec4 g = v_color1;
	float k = v_param.y;
	if (v_param.x < 0.5) {
		float a = texture(u_atlas, v_extra.xy).r;
		// The contrast applies to dark text and fades out for light.
		k *= clamp(4.0 * (0.75 - dot(c.rgb, vec3(0.30, 0.59, 0.11))), 0.0, 1.0);
		a = a * (k + 1.0) / (a * k + 1.0);
		float f = dot(c.rgb, vec3(0.25, 0.5, 0.25));
		a = clamp(a + a * (1.0 - a) * ((g.x * f + g.y) * a + (g.z * f + g.w)), 0.0, 1.0);
		vec4 col = premul(c) * a;
		cover = vec4(col.a);
		return col;
	}
	vec3 m = texture(u_lcd, v_extra.xy).rgb;
	if (v_param.x > 1.5) {
		m = m.bgr;
	}
	m = m * (k + 1.0) / (m * k + 1.0);
	m = clamp(m + m * (1.0 - m) * ((g.x * c.rgb + g.y) * m + (g.z * c.rgb + g.w)), 0.0, 1.0) * c.a;
	cover = vec4(m, max(m.r, max(m.g, m.b)));
	return vec4(c.rgb * m, cover.a);
}

vec4 shade(out vec4 cover) {
	int kind = int(v_param.z + 0.5);
	if (kind == 2) {
		return glyph(cover);
	}
	vec4 col;
	if (kind == 1) {
		// A shadow: the shape's distance field, offset, spread and
		// softened.
		float spread = v_extra.w;
		vec4 r = v_rect + vec4(-spread, -spread, spread, spread);
		float d = sdRRect(v_local - v_extra.xy, r, v_param.x + spread);
		float blur = max(v_extra.z, 0.5);
		col = premul(v_color0) * (1.0 - smoothstep(-blur, blur, d));
	} else if (kind == 3) {
		float cov = coverage(sdRRect(v_local, v_rect, v_param.x));
		col = texture(u_tex, v_extra.xy) * v_color0.a * cov;
	} else if (kind == 4) {
		float cov = 1.0;
		if (v_param.w > 0.5) {
			cov = coverage(sdRRect(v_local, v_rect, v_param.x));
		}
		col = texture(u_tex, v_uv) * v_color0.a * cov;
	} else {
		float d = sdRRect(v_local, v_rect, v_param.x);
		vec4 fill = v_color0;
		if (v_param.w > 0.5) {
			vec2 g = v_extra.zw - v_extra.xy;
			float t = clamp(dot(v_local - v_extra.xy, g) / max(dot(g, g), 1e-6), 0.0, 1.0);
			fill = mix(v_color0, v_color1, t);
		}
		col = premul(fill) * coverage(d);
		float sw = v_param.y;
		if (sw > 0.0) {
			vec4 s = premul(v_stroke) * coverage(abs(d) - sw * 0.5);
			col = s + col * (1.0 - s.a);
		}
	}
	cover = vec4(col.a);
	return col;
}

void main() {
	vec4 cover;
	fragColor = shade(cover);
#ifdef DUAL
	fragCover = cover;
#endif
}
`

func newRenderer(g gl.Context, isES bool, sh *shared) (*renderer, error) {
	r := &renderer{gl: g, shared: sh, images: map[*paint.Image]*imageTexture{}}
	var err error
	if r.drawProg, r.blurProg, r.dual, err = sh.programs(g, isES); err != nil {
		return nil, err
	}

	r.vao = g.CreateVertexArray()
	g.BindVertexArray(r.vao)
	r.vbo = g.CreateBuffer()
	g.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	off := 0
	for i, a := range attribs {
		g.EnableVertexAttribArray(uint32(i))
		g.VertexAttribPointer(uint32(i), a.size, gl.FLOAT, false, vertFloats*4, off)
		off += int(a.size) * 4
	}
	// Every batch uses the same two triangles per quad.
	idx := make([]byte, 0, maxQuads*6*2)
	for q := range maxQuads {
		b := uint16(q * 4)
		for _, i := range [6]uint16{b, b + 1, b + 2, b, b + 2, b + 3} {
			idx = binary.LittleEndian.AppendUint16(idx, i)
		}
	}
	r.ibo = g.CreateBuffer()
	g.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.ibo)
	g.BufferInit(gl.ELEMENT_ARRAY_BUFFER, len(idx), glStaticDraw)
	g.BufferSubData(gl.ELEMENT_ARRAY_BUFFER, 0, idx)

	r.initGlyphs()
	g.Enable(gl.BLEND)
	if r.dual {
		g.BlendFuncSeparate(gl.ONE, glOneMinusSrc1Color, gl.ONE, glOneMinusSrc1Alpha)
	} else {
		g.BlendFuncSeparate(gl.ONE, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	}
	return r, nil
}

// setText sets how the renderer draws text, for a window whose surface
// blends with what is behind it when transparent.
func (r *renderer) setText(tr text.Rendering, transparent bool) {
	r.textRendering = tr
	r.subpixels = tr.Smoothing.Subpixel() && r.dual && !transparent
	r.gamma = ratiosFor(tr.Gamma)
}

// buildPrograms compiles and links the two shared programs and points
// their samplers at their texture units: the glyph atlas on unit 0, an
// image, a layer or a blur's source on unit 1, and the subpixel glyph
// atlas on unit 2. The draw program blends by channel where the context
// has dual-source blending, and dual says so.
func buildPrograms(g gl.Context, isES bool) (draw, blur program, dual bool, err error) {
	header, dualHeader := "#version 150\n", "#version 330\n#define DUAL\n"
	if isES {
		header = "#version 300 es\nprecision highp float;\n"
		dualHeader = "#version 300 es\n#extension GL_EXT_blend_func_extended : require\nprecision highp float;\n" +
			"#define DUAL\n"
	}
	draw, err = link(g, dualHeader+vertexShader, dualHeader+sdfFunc+drawOut+drawShader)
	dual = err == nil && !noDual
	if !dual {
		if draw.id != 0 {
			g.DeleteProgram(draw.id)
		}
		if draw, err = link(g, header+vertexShader, header+sdfFunc+drawOut+drawShader); err != nil {
			return program{}, program{}, false, err
		}
	}
	g.UseProgram(draw.id)
	g.Uniform1i(g.GetUniformLocation(draw.id, "u_atlas"), 0)
	g.Uniform1i(g.GetUniformLocation(draw.id, "u_tex"), 1)
	g.Uniform1i(g.GetUniformLocation(draw.id, "u_lcd"), 2)
	if blur, err = link(g, header+vertexShader, header+blurShader); err != nil {
		return program{}, program{}, false, err
	}
	g.UseProgram(blur.id)
	g.Uniform1i(g.GetUniformLocation(blur.id, "u_tex"), 1)
	return draw, blur, dual, nil
}

// noDual is set by GUNIM_NO_DUAL_SOURCE=1, which draws as a context
// without dual-source blending would, with greyscale text.
var noDual = os.Getenv("GUNIM_NO_DUAL_SOURCE") == "1"

// link compiles and links a program, with the attributes at the
// locations the vertex layout gives them.
func link(g gl.Context, vs, fs string) (program, error) {
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
	for i, a := range attribs {
		g.BindAttribLocation(id, uint32(i), a.name)
	}
	g.LinkProgram(id)
	if g.GetProgrami(id, gl.LINK_STATUS) == gl.FALSE {
		defer g.DeleteProgram(id)
		return program{}, fmt.Errorf("desktop: link shader: %s", g.GetProgramInfoLog(id))
	}
	return program{id: id}, nil
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

// release frees the renderer's GL objects. The programs belong to every
// window and stay.
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
	for m := range r.images {
		r.dropImage(m)
	}
	g.DeleteTexture(r.glyphs.tex)
	if r.lcdGlyphs.tex != 0 {
		g.DeleteTexture(r.lcdGlyphs.tex)
	}
	g.DeleteBuffer(r.vbo)
	g.DeleteBuffer(r.ibo)
	g.DeleteVertexArray(r.vao)
}

// draw replays ops into the canvas, within damage, the logical-pixel
// area that changed since the last frame, and copies the canvas to the
// window.
func (r *renderer) draw(ops []paint.Op, damage geom.Rect, fbW, fbH int, scale float32) {
	if fbW <= 0 || fbH <= 0 {
		return
	}
	g := r.gl
	r.fbW, r.fbH, r.scale = fbW, fbH, scale
	r.stack, r.depth = r.stack[:0], 0
	r.scratchX = 0
	if len(r.layers) == 0 {
		r.layers = append(r.layers, target{})
	}
	backdrop := false
	for _, op := range ops {
		// A blur spreads a change past its bounds, and a backdrop reads
		// what the canvas holds.
		if l, ok := op.(*paint.LayerOp); ok && (l.Opts.Backdrop > 0 || l.Opts.Blur > 0) {
			damage = paint.Everything
			backdrop = backdrop || l.Opts.Backdrop > 0
		}
	}
	box := r.deviceBox(damage)
	window := geom.Rect{Max: geom.Pt(float32(fbW), float32(fbH))}
	// A frame that changed more than half the window, after one that
	// did too, goes straight to it, unless it needs the canvas for a
	// backdrop. The canvas then falls out of date, and the next frame
	// that draws there draws all of it. After a small frame the canvas
	// is kept, since big frames between small ones, as an animation's
	// between the breaths of a mark beside it, would each leave the
	// next small one to draw the whole window.
	s, ws := box.Size(), window.Size()
	big := s.W*s.H > ws.W*ws.H/2
	r.direct = !backdrop && !r.flipWindow && big && r.big && r.corner == 0
	r.big = big
	if r.fit(&r.layers[0]) || !r.canvasOK || scale != r.canvasScale {
		if !r.direct {
			box = window
		}
	}
	r.redrawn = box
	if r.direct {
		// Straight to the window, which holds nothing after a swap.
		box = window
		r.canvasOK = false
	}
	g.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(0))
	g.Viewport(0, 0, int32(fbW), int32(fbH))
	r.bindDraw()
	bg, alpha := background(ops)
	if alpha == 0 && r.under.A > 0 {
		bg, alpha = rgba(r.under), float32(r.under.A)/0xff
	}
	if r.direct {
		clearWindow(g, bg, alpha)
	}
	if !box.Empty() {
		r.setClip(box)
		g.Clear(glColorBufferBit)
		g.ClearColor(0, 0, 0, 0)
		if box != window {
			r.cull = box
		}
		r.replay(ops)
		r.flush()
		r.cull = geom.Rect{}
		r.setClip(window)
	}
	if !r.direct {
		r.canvasOK, r.canvasScale = true, scale
		g.BindFramebuffer(gl.FRAMEBUFFER, r.windowFBO)
		if r.corner > 0 {
			// Transparent outside the rounded corners, the background inside them
			g.Clear(glColorBufferBit)
			if alpha > 0 {
				c := [4]float32{bg[0], bg[1], bg[2], alpha}
				rect, radius := r.shape()
				r.quad(corners(r.window(), geom.Rect{}), paint.Identity, r.scale, &look{
					rect: rect, radius: radius, kind: kindShape, color0: c, color1: c,
				})
			}
		} else {
			clearWindow(g, bg, alpha)
			g.Clear(glColorBufferBit)
			g.ClearColor(0, 0, 0, 0)
		}
		r.present(r.layers[0].tex)
		r.flush()
	}
	r.evictImages()
}

// debugClear is set by GUNIM_DEBUG_CLEAR=1, which clears the window to
// magenta before each frame, so a part of the window no frame covers
// shows. It is for finding where a stray band of colour comes from.
var debugClear = os.Getenv("GUNIM_DEBUG_CLEAR") == "1"

// clearWindow sets the colour the window's framebuffer clears to: bg
// at alpha, premultiplied as the window blends, which is the frame's
// background when it has an opaque one, else the window's own, or
// transparent with alpha zero; or magenta under GUNIM_DEBUG_CLEAR.
// Offscreen targets clear to transparent, and the caller sets that back
// after the clear.
func clearWindow(g gl.Context, bg [4]float32, alpha float32) {
	switch {
	case debugClear:
		g.ClearColor(1, 0, 1, 1)
	case alpha > 0:
		g.ClearColor(bg[0]*alpha, bg[1]*alpha, bg[2]*alpha, alpha)
	}
}

// background returns the colour of a frame's background, and 1, or 0
// when it has none: its first op,
// when that is a plain opaque rectangle from the window's top left
// corner, as a window's surface paints. A frame drawn for a smaller
// window than the buffer holds leaves a strip the clear fills, and the
// background colour makes that strip look like the window's own.
func background(ops []paint.Op) (bg [4]float32, alpha float32) {
	if len(ops) == 0 {
		return [4]float32{}, 0
	}
	op, ok := ops[0].(*paint.RRectOp)
	if !ok || op.Radius != 0 || op.Transform != paint.Identity || op.Fill.Gradient != nil ||
		op.Fill.Solid.A != 0xff || op.Shadow.Color.A != 0 || op.Stroke.Width > 0 ||
		op.Rect.Min.X > 0 || op.Rect.Min.Y > 0 {
		return [4]float32{}, 0
	}
	return rgba(op.Fill.Solid), 1
}

// present queues the canvas's copy to the window, upside down for a
// window that reads its rows from the top.
func (r *renderer) present(canvas uint32) {
	if !r.flipWindow {
		r.composite(canvas, nil, 1, false, 0)
		return
	}
	r.uses(canvas)
	win := r.window()
	// Drawn as an image, whose texture coordinates are its own: the top
	// of the window takes the texture's first row, where a layer's copy
	// would take its last.
	uv := geom.Rect{Max: geom.Pt(1, 1)}
	rect, radius := win, float32(0)
	if r.corner > 0 {
		rect, radius = r.shape()
	}
	r.quad(corners(win, uv), paint.Identity, r.scale, &look{
		rect: rect, radius: radius, kind: kindImage, color0: [4]float32{0, 0, 0, 1},
	})
}

// shape is the part of a window with cut corners that shows its frame, in logical pixels: the window less its edge,
// with corners that much tighter.
func (r *renderer) shape() (rect geom.Rect, radius float32) {
	w := r.window()
	e := r.edge / r.scale
	return geom.Rect{Min: geom.Pt(w.Min.X+e, w.Min.Y+e), Max: geom.Pt(w.Max.X-e, w.Max.Y-e)}, max(r.corner-r.edge, 0) / r.scale
}

// deviceBox turns damage in logical pixels into the whole device pixels
// it touches, within the window.
func (r *renderer) deviceBox(d geom.Rect) geom.Rect {
	x0 := max(0, float32(math.Floor(float64(d.Min.X*r.scale)))-1)
	y0 := max(0, float32(math.Floor(float64(d.Min.Y*r.scale)))-1)
	x1 := min(float32(r.fbW), float32(math.Ceil(float64(d.Max.X*r.scale)))+1)
	y1 := min(float32(r.fbH), float32(math.Ceil(float64(d.Max.Y*r.scale)))+1)
	if x1 <= x0 || y1 <= y0 || d.Empty() {
		return geom.Rect{}
	}
	return geom.Rect{Min: geom.Pt(x0, y0), Max: geom.Pt(x1, y1)}
}

// replay queues ops into the canvas.
func (r *renderer) replay(ops []paint.Op) {
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
		case *paint.ImageOp:
			r.image(op)
		case *paint.MaskOp:
			r.mask(op)
		}
	}
	// A layer left open by a node that forgot to close it still shows.
	for len(r.stack) > 0 {
		r.closeLayer()
	}
}

// bindDraw makes the draw program and the renderer's vertices current,
// with the glyph atlas on unit 0.
func (r *renderer) bindDraw() {
	g := r.gl
	g.UseProgram(r.drawProg.id)
	g.BindVertexArray(r.vao)
	g.BindBuffer(gl.ARRAY_BUFFER, r.vbo)
	g.ActiveTexture(gl.TEXTURE0)
	g.BindTexture(gl.TEXTURE_2D, r.glyphs.tex)
}

// flush draws the batch.
func (r *renderer) flush() {
	n := len(r.verts) / (4 * vertFloats)
	if n == 0 {
		return
	}
	g := r.gl
	if r.tex != 0 {
		g.ActiveTexture(glTexture1)
		g.BindTexture(gl.TEXTURE_2D, r.tex)
		g.ActiveTexture(gl.TEXTURE0)
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(&r.verts[0])), len(r.verts)*4)
	// A fresh store each time lets the driver keep the last one for the
	// draw still reading it.
	g.BufferInit(gl.ARRAY_BUFFER, len(data), gl.STREAM_DRAW)
	g.BufferSubData(gl.ARRAY_BUFFER, 0, data)
	g.DrawElements(gl.TRIANGLES, int32(n*6), glUnsignedShort, 0)
	r.draws++
	r.verts = r.verts[:0]
	r.tex = 0
}

// uses readies the batch for a quad that reads tex on unit 1, drawing
// what is queued first when it reads another texture or is full.
func (r *renderer) uses(tex uint32) {
	if len(r.verts)/(4*vertFloats) >= maxQuads || (tex != 0 && r.tex != 0 && r.tex != tex) {
		r.flush()
	}
	if tex != 0 {
		r.tex = tex
	}
}

// quadVert is one corner of a quad: the point in the shape's own space
// and, for images and glyphs, its texture coordinates.
type quadVert struct {
	local geom.Point
	uv    geom.Point
}

// look is what every pixel of a quad shares.
type look struct {
	rect           geom.Rect
	radius, stroke float32
	kind           int
	flag           bool
	color0, color1 [4]float32
	extra          [4]float32
	strokeColor    [4]float32
}

// quad queues a quad with corners in the shape's own space, placed
// through t. Glyph quads arrive already in device pixels, with t the
// identity and scale 1.
func (r *renderer) quad(corners [4]quadVert, t paint.Transform, scale float32, l *look) {
	flag := float32(0)
	if l.flag {
		flag = 1
	}
	sx, sy := 2*scale/float32(r.fbW), 2*scale/float32(r.fbH)
	var at [4]geom.Point
	for i, c := range corners {
		at[i] = t.Apply(c.local)
	}
	if !r.cull.Empty() && len(r.stack) == 0 {
		lo, hi := at[0], at[0]
		for _, p := range at[1:] {
			lo = geom.Pt(min(lo.X, p.X), min(lo.Y, p.Y))
			hi = geom.Pt(max(hi.X, p.X), max(hi.Y, p.Y))
		}
		if hi.X*scale < r.cull.Min.X || lo.X*scale > r.cull.Max.X || hi.Y*scale < r.cull.Min.Y || lo.Y*scale > r.cull.Max.Y {
			return
		}
	}
	for i, c := range corners {
		p := at[i]
		extra := l.extra
		if l.kind == kindGlyph || l.kind == kindImage {
			extra[0], extra[1] = c.uv.X, c.uv.Y
		}
		r.verts = append(r.verts,
			p.X*sx-1, 1-p.Y*sy,
			c.local.X, c.local.Y,
			l.rect.Min.X, l.rect.Min.Y, l.rect.Max.X, l.rect.Max.Y,
			l.radius, l.stroke, float32(l.kind), flag,
			l.color0[0], l.color0[1], l.color0[2], l.color0[3],
			l.color1[0], l.color1[1], l.color1[2], l.color1[3],
			extra[0], extra[1], extra[2], extra[3],
			l.strokeColor[0], l.strokeColor[1], l.strokeColor[2], l.strokeColor[3],
		)
	}
}

// corners returns a rectangle's corners in the order the index buffer
// expects, with texture coordinates spanning uv.
func corners(q, uv geom.Rect) [4]quadVert {
	return [4]quadVert{
		{q.Min, uv.Min},
		{geom.Pt(q.Max.X, q.Min.Y), geom.Pt(uv.Max.X, uv.Min.Y)},
		{q.Max, uv.Max},
		{geom.Pt(q.Min.X, q.Max.Y), geom.Pt(uv.Min.X, uv.Max.Y)},
	}
}

func (r *renderer) rrect(op *paint.RRectOp) {
	r.uses(0)
	// The shadow goes first, underneath, on a quad grown to hold it.
	if sh := op.Shadow; sh.Color.A > 0 {
		grow := sh.Blur + sh.Spread + 2
		r.quad(corners(grow4(op.Rect.Add(sh.Offset), grow), geom.Rect{}), op.Transform, r.scale, &look{
			rect: op.Rect, radius: op.Radius, kind: kindShadow,
			color0: rgba(sh.Color),
			extra:  [4]float32{sh.Offset.X, sh.Offset.Y, sh.Blur, sh.Spread},
		})
	}

	l := look{rect: op.Rect, radius: op.Radius, kind: kindShape, color0: rgba(op.Fill.Solid)}
	l.color1 = l.color0
	if gr := op.Fill.Gradient; gr != nil {
		l.color0, l.color1 = rgba(gr.Start), rgba(gr.End)
		l.extra = [4]float32{gr.From.X, gr.From.Y, gr.To.X, gr.To.Y}
		l.flag = true
	}
	if l.color0[3] == 0 && l.color1[3] == 0 && (op.Stroke.Width <= 0 || op.Stroke.Color.A == 0) {
		return
	}
	l.stroke, l.strokeColor = op.Stroke.Width, rgba(op.Stroke.Color)
	r.quad(corners(grow4(op.Rect, op.Stroke.Width/2+2), geom.Rect{}), op.Transform, r.scale, &l)
}

// openLayer starts drawing into a fresh offscreen target, or goes on
// drawing into the current one, scissored, for a layer that can draw
// in place.
func (r *renderer) openLayer(op *paint.LayerOp) {
	r.flush()
	r.stack = append(r.stack, openLayer{op: op, clip: r.clip})
	if box, ok := r.inPlace(op); ok {
		r.stack[len(r.stack)-1].inPlace = true
		r.setClip(intersect(r.clip, box))
		return
	}
	r.depth++
	for len(r.layers) <= r.depth {
		r.layers = append(r.layers, target{})
	}
	t := &r.layers[r.depth]
	r.fit(t)
	g := r.gl
	g.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	g.Clear(glColorBufferBit)
}

// inPlace reports whether a layer draws the same straight into the
// target around it as composited from a target of its own, and the
// device-pixel box it clips to: it is opaque, blurs nothing, and clips
// to an upright rectangle or not at all.
func (r *renderer) inPlace(op *paint.LayerOp) (geom.Rect, bool) {
	o, t := op.Opts, op.Transform
	switch {
	case o.Opacity < 1 || o.Blur > 0 || o.Backdrop > 0:
		return geom.Rect{}, false
	case !o.Clip:
		return r.region(op, false), true
	case o.Radius > 0 || t.B != 0 || t.D != 0:
		return geom.Rect{}, false
	}
	b := r.region(op, true)
	round := func(v float32) float32 { return float32(math.Round(float64(v))) }
	return geom.Rect{Min: geom.Pt(round(b.Min.X), round(b.Min.Y)), Max: geom.Pt(round(b.Max.X), round(b.Max.Y))}, true
}

// closeLayer composites the innermost layer into the one around it.
//
// A Backdrop goes first: what the parent holds so far is blurred and
// drawn back over itself within the layer's bounds, rounded when the
// layer clips. The layer's contents go on top, blurred first when the
// layer asks for Blur.
func (r *renderer) closeLayer() {
	r.flush()
	top := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	r.setClip(top.clip)
	if top.inPlace {
		return
	}
	depth := r.depth
	r.depth--
	op := top.op
	o := op.Opts
	radius := float32(0)
	if o.Clip {
		radius = o.Radius
	}

	if o.Backdrop > 0 {
		behind := r.blur(r.layers[depth-1].tex, r.region(op, true), o.Backdrop*r.scale)
		r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(depth-1))
		r.composite(behind, op, o.Opacity, true, radius)
		// The next blur at this resolution reuses the texture.
		r.flush()
	}

	contents := r.layers[depth].tex
	if o.Blur > 0 {
		contents = r.blur(contents, r.region(op, o.Clip), o.Blur*r.scale)
	}
	r.gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(depth-1))
	r.composite(contents, op, o.Opacity, o.Clip, radius)
	// The next layer at this depth draws into the same texture.
	r.flush()
}

// composite queues tex, a window-sized texture, drawn into the bound
// target at opacity. With clip it covers op's bounds, rounded by
// radius; without, the whole window. A nil op composites the whole
// window unclipped.
func (r *renderer) composite(tex uint32, op *paint.LayerOp, opacity float32, clip bool, radius float32) {
	r.uses(tex)
	l := look{kind: kindLayer, color0: [4]float32{0, 0, 0, opacity}}
	if clip && op != nil {
		b := op.Opts.Bounds
		l.rect, l.radius, l.flag = b, radius, true
		r.quad(corners(grow4(b, 2), geom.Rect{}), op.Transform, r.scale, &l)
		return
	}
	r.quad(corners(r.window(), geom.Rect{}), paint.Identity, r.scale, &l)
}

// fbo returns the framebuffer for nesting depth d: the canvas, or the
// window for a frame drawn straight to it, at depth 0, and a layer's
// target beneath it.
func (r *renderer) fbo(d int) uint32 {
	if d == 0 && r.direct {
		return r.windowFBO
	}
	return r.layers[d].fbo
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

// fit sizes t to the window, creating it on first use, and reports
// whether it changed.
func (r *renderer) fit(t *target) bool { return r.fitSize(t, r.fbW, r.fbH) }

// fitSize sizes t to w by h pixels, creating it on first use, and
// reports whether it changed. A changed target holds nothing.
func (r *renderer) fitSize(t *target, w, h int) bool {
	g := r.gl
	if t.tex != 0 && t.w == w && t.h == h {
		return false
	}
	if t.tex == 0 {
		t.tex = g.CreateTexture()
		t.fbo = g.CreateFramebuffer()
	}
	t.w, t.h = w, h
	g.ActiveTexture(glTexture1)
	g.BindTexture(gl.TEXTURE_2D, t.tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, glLinear)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(t.w), int32(t.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
	g.ActiveTexture(gl.TEXTURE0)
	g.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	g.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.tex, 0)
	return true
}

func grow4(r geom.Rect, by float32) geom.Rect {
	return geom.Rect{
		Min: geom.Pt(r.Min.X-by, r.Min.Y-by),
		Max: geom.Pt(r.Max.X+by, r.Max.Y+by),
	}
}

func rgba(c color.NRGBA) [4]float32 {
	return [4]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255, float32(c.A) / 255}
}

// setClip scissors drawing to c, a device-pixel box with its origin at
// the top left.
func (r *renderer) setClip(c geom.Rect) {
	r.clip = c
	r.applyClip()
}

// applyClip sets GL's scissor to the renderer's clip, or turns it off
// where the clip holds the whole target.
func (r *renderer) applyClip() {
	g, c := r.gl, r.clip
	if c.Min.X <= 0 && c.Min.Y <= 0 && c.Max.X >= float32(r.fbW) && c.Max.Y >= float32(r.fbH) {
		g.Disable(gl.SCISSOR_TEST)
		return
	}
	g.Enable(gl.SCISSOR_TEST)
	g.Scissor(scissor(c, r.fbW, r.fbH))
}

// intersect returns the part of a that b covers too.
func intersect(a, b geom.Rect) geom.Rect {
	return geom.Rect{
		Min: geom.Pt(max(a.Min.X, b.Min.X), max(a.Min.Y, b.Min.Y)),
		Max: geom.Pt(min(a.Max.X, b.Max.X), min(a.Max.Y, b.Max.Y)),
	}
}
