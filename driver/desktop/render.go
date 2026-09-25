//go:build linux || windows || darwin

package desktop

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"math"
	"os"
	"slices"
	"time"
	"unsafe"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
)

// GL constants the gl package leaves out.
const (
	glColorBufferBit     = 0x4000
	glLinear             = 0x2601
	glLinearMipmapLinear = 0x2703
	glUnsignedShort      = 0x1403
	glR8                 = 0x8229
	glRed                = 0x1903
	glStaticDraw         = 0x88E4
	glTexture1           = 0x84C1
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
//
// Text draws from a glyph atlas: each glyph is rasterized once per size
// and quarter-pixel shift; see glyphs.go. Images upload once and stay
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

	glyphs glyphTexture
	images map[*paint.Image]*imageTexture

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
	// quicker when most of it changed: it saves the copy.
	direct bool
	// redrawn is the device-pixel area the last frame redrew, for
	// tests.
	redrawn geom.Rect
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

// program is a linked shader.
type program struct{ id uint32 }

// Each vertex is vertFloats floats, in eight attributes of two or four:
//
//	a_pos    where the vertex lands, in normalized device coordinates
//	a_local  the point in the shape's own space, for its distance field
//	a_rect   the shape's rectangle in its own space
//	a_param  corner radius, stroke width, kind, and a flag
//	a_color0 the fill, the shadow's colour or the glyph's; for an image
//	         or a layer, the opacity in alpha
//	a_color1 the gradient's end colour
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
uniform sampler2D u_tex;
out vec4 fragColor;

void main() {
	int kind = int(v_param.z + 0.5);
	if (kind == 1) {
		// A shadow: the shape's distance field, offset, spread and
		// softened.
		float spread = v_extra.w;
		vec4 r = v_rect + vec4(-spread, -spread, spread, spread);
		float d = sdRRect(v_local - v_extra.xy, r, v_param.x + spread);
		float blur = max(v_extra.z, 0.5);
		fragColor = premul(v_color0) * (1.0 - smoothstep(-blur, blur, d));
		return;
	}
	if (kind == 2) {
		fragColor = premul(v_color0) * texture(u_atlas, v_extra.xy).r;
		return;
	}
	if (kind == 3) {
		float cov = coverage(sdRRect(v_local, v_rect, v_param.x));
		fragColor = texture(u_tex, v_extra.xy) * v_color0.a * cov;
		return;
	}
	if (kind == 4) {
		float cov = 1.0;
		if (v_param.w > 0.5) {
			cov = coverage(sdRRect(v_local, v_rect, v_param.x));
		}
		fragColor = texture(u_tex, v_uv) * v_color0.a * cov;
		return;
	}
	float d = sdRRect(v_local, v_rect, v_param.x);
	vec4 fill = v_color0;
	if (v_param.w > 0.5) {
		vec2 g = v_extra.zw - v_extra.xy;
		float t = clamp(dot(v_local - v_extra.xy, g) / max(dot(g, g), 1e-6), 0.0, 1.0);
		fill = mix(v_color0, v_color1, t);
	}
	vec4 col = premul(fill) * coverage(d);
	float sw = v_param.y;
	if (sw > 0.0) {
		vec4 s = premul(v_stroke) * coverage(abs(d) - sw * 0.5);
		col = s + col * (1.0 - s.a);
	}
	fragColor = col;
}
`

func newRenderer(g gl.Context, isES bool, sh *shared) (*renderer, error) {
	r := &renderer{gl: g, shared: sh, images: map[*paint.Image]*imageTexture{}}
	var err error
	if r.drawProg, r.blurProg, err = sh.programs(g, isES); err != nil {
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
	g.BlendFuncSeparate(gl.ONE, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
	return r, nil
}

// buildPrograms compiles and links the two shared programs and points
// their samplers at their texture units: the glyph atlas on unit 0, and
// an image, a layer or a blur's source on unit 1.
func buildPrograms(g gl.Context, isES bool) (draw, blur program, err error) {
	header := "#version 150\n"
	if isES {
		header = "#version 300 es\nprecision highp float;\n"
	}
	if draw, err = link(g, header+vertexShader, header+sdfFunc+drawShader); err != nil {
		return program{}, program{}, err
	}
	g.UseProgram(draw.id)
	g.Uniform1i(g.GetUniformLocation(draw.id, "u_atlas"), 0)
	g.Uniform1i(g.GetUniformLocation(draw.id, "u_tex"), 1)
	if blur, err = link(g, header+vertexShader, header+blurShader); err != nil {
		return program{}, program{}, err
	}
	g.UseProgram(blur.id)
	g.Uniform1i(g.GetUniformLocation(blur.id, "u_tex"), 1)
	return draw, blur, nil
}

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
	r.stack = r.stack[:0]
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
	// A frame that changed more than half the window goes straight to
	// it, unless it needs the canvas for a backdrop. The canvas then
	// falls out of date, and the next frame that draws there draws all
	// of it.
	s, ws := box.Size(), window.Size()
	decided, fullOnly := r.shared.redraw()
	r.direct = !backdrop && (s.W*s.H > ws.W*ws.H/2 || fullOnly)
	if r.fit(&r.layers[0]) || !r.canvasOK || scale != r.canvasScale {
		if !r.direct {
			box = window
		}
	}
	whole := box == window
	r.redrawn = box
	if r.direct {
		// Straight to the window, which holds nothing after a swap.
		box, whole = window, true
		r.canvasOK = false
	}
	// Until the policy settles, frames are timed, finished on the GPU
	// at both ends so the time is this frame's alone.
	timing := !decided
	var start time.Time
	if timing {
		g.Finish()
		start = time.Now()
	}
	g.BindFramebuffer(gl.FRAMEBUFFER, r.fbo(0))
	g.Viewport(0, 0, int32(fbW), int32(fbH))
	r.bindDraw()
	bg, opaque := background(ops)
	if r.direct {
		clearWindow(g, bg, opaque)
	}
	if !box.Empty() {
		if !whole {
			g.Enable(gl.SCISSOR_TEST)
			g.Scissor(scissor(box, fbW, fbH))
		}
		g.Clear(glColorBufferBit)
		g.ClearColor(0, 0, 0, 0)
		r.replay(ops)
		r.flush()
		if !whole {
			g.Disable(gl.SCISSOR_TEST)
		}
	}
	if !r.direct {
		r.canvasOK, r.canvasScale = true, scale
		g.BindFramebuffer(gl.FRAMEBUFFER, 0)
		clearWindow(g, bg, opaque)
		g.Clear(glColorBufferBit)
		g.ClearColor(0, 0, 0, 0)
		r.composite(r.layers[0].tex, nil, 1, false, 0)
		r.flush()
	}
	if timing {
		g.Finish()
		switch took := time.Since(start); {
		case r.direct:
			r.shared.timeFrame(false, took)
		case !whole:
			r.shared.timeFrame(true, took)
		}
	}
	r.evictImages()
}

// debugClear is set by GUNIM_DEBUG_CLEAR=1, which clears the window to
// magenta before each frame, so a part of the window no frame covers
// shows. It is for finding where a stray band of colour comes from.
var debugClear = os.Getenv("GUNIM_DEBUG_CLEAR") == "1"

// clearWindow sets the colour the window's framebuffer clears to: the
// frame's background when it has an opaque one, transparent when it
// has none, or magenta under GUNIM_DEBUG_CLEAR. Offscreen targets
// clear to transparent, and the caller sets that back after the clear.
func clearWindow(g gl.Context, bg [4]float32, opaque bool) {
	switch {
	case debugClear:
		g.ClearColor(1, 0, 1, 1)
	case opaque:
		g.ClearColor(bg[0], bg[1], bg[2], 1)
	}
}

// background returns the colour of a frame's background: its first op,
// when that is a plain opaque rectangle from the window's top left
// corner, as a window's surface paints. A frame drawn for a smaller
// window than the buffer holds leaves a strip the clear fills, and the
// background colour makes that strip look like the window's own.
func background(ops []paint.Op) ([4]float32, bool) {
	if len(ops) == 0 {
		return [4]float32{}, false
	}
	op, ok := ops[0].(*paint.RRectOp)
	if !ok || op.Radius != 0 || op.Transform != paint.Identity || op.Fill.Gradient != nil ||
		op.Fill.Solid.A != 0xff || op.Shadow.Color.A != 0 || op.Stroke.Width > 0 ||
		op.Rect.Min.X > 0 || op.Rect.Min.Y > 0 {
		return [4]float32{}, false
	}
	return rgba(op.Fill.Solid), true
}

// redrawPolicy learns whether redrawing only what changed is worth it
// here. It redraws the changed part into the canvas and copies the
// canvas to the window, where drawing everything goes straight to the
// window. Software GL shades each pixel slowly, so the smaller redraw
// wins, by about a third on llvmpipe. A GPU shades the whole window
// quickly, so the copy costs more than it saves, about a fifth more on
// an RTX 3070.
//
// So the first frames of each kind are timed, and once there are
// enough of both, the faster kind stays for every window. When one
// kind never comes, timing stops after a while and the partial redraw
// stays.
type redrawPolicy struct {
	decided bool
	// partial is the choice, once decided.
	partial bool
	// full and part hold the times of frames drawn each way, and timed
	// counts every frame timed.
	full, part []time.Duration
	timed      int
}

// policySamples is how many frames of each kind decide the policy, and
// policyTimed how many frames are timed before it gives up deciding.
const (
	policySamples = 6
	policyTimed   = 600
)

// fullOnly reports whether every frame should go straight to the
// window.
func (p *redrawPolicy) fullOnly() bool { return p.decided && !p.partial }

// add records a frame's time, drawn in part or in full.
func (p *redrawPolicy) add(partial bool, took time.Duration) {
	if partial {
		p.part = append(p.part, took)
	} else {
		p.full = append(p.full, took)
	}
	p.timed++
	if len(p.full) >= policySamples && len(p.part) >= policySamples {
		p.decided, p.partial = true, median(p.part) < median(p.full)
	} else if p.timed >= policyTimed {
		p.decided, p.partial = true, true
	}
}

// median returns the middle of ds, which it sorts.
func median(ds []time.Duration) time.Duration {
	slices.Sort(ds)
	return ds[len(ds)/2]
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
	for _, c := range corners {
		p := t.Apply(c.local)
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

// openLayer starts drawing into a fresh offscreen target.
func (r *renderer) openLayer(op *paint.LayerOp) {
	r.flush()
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
	r.flush()
	depth := len(r.stack)
	op := r.stack[depth-1]
	r.stack = r.stack[:depth-1]
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
		return 0
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
