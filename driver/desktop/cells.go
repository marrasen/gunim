package desktop

import (
	"math"
	"unsafe"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
)

// A grid of character cells is drawn by a program of its own: one quad
// over the rows, whose fragments look their cell up in a texture that
// holds each cell's background, foreground and pattern. A row costs the
// GPU twelve bytes a cell, where drawing its cells as rectangles costs
// hundreds.

// The texture units the cells program reads, past the draw program's.
const (
	cellsUnit    = gl.TEXTURE0 + 4
	patternsUnit = gl.TEXTURE0 + 5
)

// cellsWidth and cellsRows are the size of the texture rows of cells are
// uploaded into, three texels a cell, row after row through a frame.
const (
	cellsWidth = 4096
	cellsRows  = 512
)

// patternsWidth is the width of the texture of patterns, which holds
// them side by side in rows.
const patternsWidth = 2048

// cellsState is a renderer's means of drawing cells.
type cellsState struct {
	prog         program
	failed       bool
	vao, vbo     uint32
	tex          uint32
	texW         int
	row          int
	uCell, uBase int32
	uPer         int32
	// pats is the texture of patterns, the Patterns it holds, how many
	// of their masks are uploaded, and how many it has room for.
	pats     uint32
	patsOf   *paint.Patterns
	patsSent int
	patsRoom int
	patsPer  int
	pending  *paint.CellsOp
	firstRow int
	rows     int
	upload   []byte
	verts    [16]float32
}

const cellsVertex = `
in vec2 a_pos;
in vec2 a_px;
out vec2 v_px;
void main() {
	gl_Position = vec4(a_pos, 0.0, 1.0);
	v_px = a_px;
}
`

const cellsFragment = `
uniform sampler2D u_cells;
uniform sampler2D u_patterns;
uniform vec2 u_cell;
uniform int u_base;
uniform int u_per;
in vec2 v_px;

void main() {
	vec2 c = floor(v_px / u_cell);
	ivec2 cell = ivec2(c);
	ivec2 inside = ivec2(clamp(floor(v_px - c * u_cell), vec2(0.0), u_cell - 1.0));
	int x = cell.x * 3;
	int y = u_base + cell.y;
	vec4 bg = texelFetch(u_cells, ivec2(x, y), 0);
	vec4 fg = texelFetch(u_cells, ivec2(x + 1, y), 0);
	vec4 pt = texelFetch(u_cells, ivec2(x + 2, y), 0);
	int id = int(pt.r * 255.0 + 0.5) + 256 * int(pt.g * 255.0 + 0.5);
	float a = 0.0;
	if (id > 0) {
		int k = id - 1;
		ivec2 cw = ivec2(u_cell);
		a = texelFetch(u_patterns, ivec2((k % u_per) * cw.x + inside.x, (k / u_per) * cw.y + inside.y), 0).r;
	}
	vec4 b = vec4(bg.rgb * bg.a, bg.a);
	vec4 f = vec4(fg.rgb * fg.a, fg.a) * a;
	vec4 col = f + b * (1.0 - f.a);
	fragColor = col;
#ifdef DUAL
	fragCover = vec4(col.a);
#endif
}
`

// cellsReady builds what drawing cells needs, once, and reports whether
// it can.
func (r *renderer) cellsReady() bool {
	s := &r.cellsState
	if s.prog.id != 0 || s.failed {
		return !s.failed
	}
	g := r.gl
	header := "#version 150\n"
	if r.isES {
		header = "#version 300 es\nprecision highp float;\nprecision highp int;\n"
	}
	fragHeader := header
	if r.dual {
		fragHeader = "#version 330\n#define DUAL\n"
		if r.isES {
			fragHeader = "#version 300 es\n#extension GL_EXT_blend_func_extended : require\nprecision highp float;\nprecision highp int;\n#define DUAL\n"
		}
	}
	v, err := compile(g, gl.VERTEX_SHADER, header+cellsVertex)
	if err != nil {
		s.failed = true
		return false
	}
	defer g.DeleteShader(v)
	f, err := compile(g, gl.FRAGMENT_SHADER, fragHeader+drawOut+cellsFragment)
	if err != nil {
		s.failed = true
		return false
	}
	defer g.DeleteShader(f)
	id := g.CreateProgram()
	g.AttachShader(id, v)
	g.AttachShader(id, f)
	g.BindAttribLocation(id, 0, "a_pos")
	g.BindAttribLocation(id, 1, "a_px")
	g.LinkProgram(id)
	if g.GetProgrami(id, gl.LINK_STATUS) == gl.FALSE {
		g.DeleteProgram(id)
		s.failed = true
		return false
	}
	s.prog = program{id: id}
	g.UseProgram(id)
	g.Uniform1i(g.GetUniformLocation(id, "u_cells"), 4)
	g.Uniform1i(g.GetUniformLocation(id, "u_patterns"), 5)
	s.uCell = g.GetUniformLocation(id, "u_cell")
	s.uBase = g.GetUniformLocation(id, "u_base")
	s.uPer = g.GetUniformLocation(id, "u_per")

	s.vao = g.CreateVertexArray()
	g.BindVertexArray(s.vao)
	s.vbo = g.CreateBuffer()
	g.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	g.EnableVertexAttribArray(0)
	g.VertexAttribPointer(0, 2, gl.FLOAT, false, 16, 0)
	g.EnableVertexAttribArray(1)
	g.VertexAttribPointer(1, 2, gl.FLOAT, false, 16, 8)
	g.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.ibo)
	r.bindDraw()
	return true
}

// cells queues a row of cells, drawn with the rows before it that it
// follows, or draws what is queued first.
func (r *renderer) cells(op *paint.CellsOp) {
	if len(op.Cells) == 0 || op.Patterns == nil {
		return
	}
	cw, ch := int(math.Round(float64(op.Size.W*r.scale))), int(math.Round(float64(op.Size.H*r.scale)))
	if cw <= 0 || ch <= 0 || cw != op.Patterns.W || ch != op.Patterns.H {
		return
	}
	end := geom.Pt(op.At.X+op.Size.W*float32(len(op.Cells)), op.At.Y+op.Size.H)
	if !r.cull.Empty() && r.outside(r.deviceRect(op.Transform, geom.Rect{Min: op.At, Max: end})) {
		return
	}
	if !r.cellsReady() {
		return
	}
	s := &r.cellsState
	// Rows join the run before them only with nothing drawn in between,
	// which would end up beneath them.
	if p := s.pending; p == nil || len(r.verts) > 0 || !follows(p, op, s.rows) || s.firstRow+s.rows >= cellsRows {
		r.flush()
		r.startCells(op)
	}
	r.addRow(op)
	s.rows++
}

// follows reports whether op is the row after the rows of p queued.
func follows(p, op *paint.CellsOp, rows int) bool {
	return p.Grid == op.Grid && op.Row == p.Row+rows && p.Patterns == op.Patterns && p.Size == op.Size &&
		p.Transform == op.Transform && p.At.X == op.At.X && len(p.Cells) == len(op.Cells)
}

// startCells begins a run of rows with op, uploading any of its
// patterns not yet on the GPU.
func (r *renderer) startCells(op *paint.CellsOp) {
	s := &r.cellsState
	g := r.gl
	if need := len(op.Cells) * 3; need > s.texW || s.tex == 0 {
		if s.tex != 0 {
			g.DeleteTexture(s.tex)
		}
		s.texW = max(need, cellsWidth)
		s.tex = r.cellsTexture(cellsUnit, glRGBA8, gl.RGBA, s.texW, cellsRows, 4)
		s.row = 0
	}
	if s.row >= cellsRows {
		s.row = 0
	}
	s.pending, s.firstRow, s.rows = op, s.row, 0
	s.upload = s.upload[:0]
	r.sendPatterns(op.Patterns)
}

// sendPatterns uploads the masks of pats the GPU does not have yet.
func (r *renderer) sendPatterns(pats *paint.Patterns) {
	s := &r.cellsState
	g := r.gl
	n := len(pats.Masks)
	if s.patsOf != pats || n > s.patsRoom {
		if s.pats != 0 {
			g.DeleteTexture(s.pats)
		}
		s.patsPer = max(patternsWidth/pats.W, 1)
		s.patsRoom = max(256, 2*n)
		rows := (s.patsRoom + s.patsPer - 1) / s.patsPer
		s.pats = r.cellsTexture(patternsUnit, glR8, glRed, s.patsPer*pats.W, rows*pats.H, 1)
		s.patsOf, s.patsSent = pats, 0
	}
	if s.patsSent == n {
		return
	}
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.pats)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	for k := s.patsSent; k < n; k++ {
		m := pats.Masks[k]
		if len(m) != pats.W*pats.H {
			continue
		}
		x, y := (k%s.patsPer)*pats.W, (k/s.patsPer)*pats.H
		g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(pats.W), int32(pats.H), glRed, gl.UNSIGNED_BYTE, m)
	}
	g.ActiveTexture(gl.TEXTURE0)
	s.patsSent = n
}

// cellsTexture makes a texture of w by h texels on unit, read texel by
// texel.
func (r *renderer) cellsTexture(unit uint32, internal int32, format uint32, w, h, channels int) uint32 {
	g := r.gl
	tex := g.CreateTexture()
	g.ActiveTexture(unit)
	g.BindTexture(gl.TEXTURE_2D, tex)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	g.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexImage2D(gl.TEXTURE_2D, 0, internal, int32(w), int32(h), format, gl.UNSIGNED_BYTE, make([]byte, w*h*channels))
	g.ActiveTexture(gl.TEXTURE0)
	return tex
}

// addRow adds op's cells to the rows uploaded together when they are
// drawn: one upload a run of rows, rather than one a row.
func (r *renderer) addRow(op *paint.CellsOp) {
	s := &r.cellsState
	b := s.upload
	for _, c := range op.Cells {
		b = append(b, c.BG.R, c.BG.G, c.BG.B, c.BG.A, c.FG.R, c.FG.G, c.FG.B, c.FG.A,
			byte(c.Pattern), byte(c.Pattern>>8), 0, 0)
	}
	s.upload = b
}

// flushCells draws the rows of cells queued, if any.
func (r *renderer) flushCells() {
	s := &r.cellsState
	op := s.pending
	if op == nil || s.rows == 0 {
		s.pending = nil
		return
	}
	s.pending = nil
	g := r.gl
	g.ActiveTexture(cellsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.tex)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, int32(s.firstRow), int32(len(op.Cells)*3), int32(s.rows), gl.RGBA, gl.UNSIGNED_BYTE, s.upload)
	w := op.Size.W * float32(len(op.Cells))
	h := op.Size.H * float32(s.rows)
	local := [4]geom.Point{op.At, geom.Pt(op.At.X+w, op.At.Y), geom.Pt(op.At.X+w, op.At.Y+h), geom.Pt(op.At.X, op.At.Y+h)}
	sx, sy := 2*r.scale/float32(r.fbW), 2*r.scale/float32(r.fbH)
	for i, l := range local {
		p := op.Transform.Apply(l)
		s.verts[i*4+0] = p.X*sx - 1
		s.verts[i*4+1] = 1 - p.Y*sy
		s.verts[i*4+2] = (l.X - op.At.X) * r.scale
		s.verts[i*4+3] = (l.Y - op.At.Y) * r.scale
	}
	g.UseProgram(s.prog.id)
	g.BindVertexArray(s.vao)
	g.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	g.BufferInit(gl.ARRAY_BUFFER, len(s.verts)*4, gl.STREAM_DRAW)
	g.BufferSubData(gl.ARRAY_BUFFER, 0, floatBytes(s.verts[:]))
	g.ActiveTexture(cellsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.tex)
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.pats)
	g.Uniform2fv(s.uCell, []float32{float32(op.Patterns.W), float32(op.Patterns.H)})
	g.Uniform1i(s.uBase, int32(s.firstRow))
	g.Uniform1i(s.uPer, int32(s.patsPer))
	g.DrawElements(gl.TRIANGLES, 6, glUnsignedShort, 0)
	r.draws++
	if framesDebug {
		r.stats.flushes++
		r.stats.quads++
		r.stats.bytes += len(s.upload) + len(s.verts)*4
	}
	s.row = s.firstRow + s.rows
	r.bindDraw()
}

// releaseCells frees what drawing cells made.
func (r *renderer) releaseCells() {
	s := &r.cellsState
	g := r.gl
	if s.prog.id != 0 {
		g.DeleteProgram(s.prog.id)
		g.DeleteBuffer(s.vbo)
		g.DeleteVertexArray(s.vao)
	}
	if s.tex != 0 {
		g.DeleteTexture(s.tex)
	}
	if s.pats != 0 {
		g.DeleteTexture(s.pats)
	}
	*s = cellsState{}
}

// deviceRect is the device-pixel box b covers under t.
func (r *renderer) deviceRect(t paint.Transform, b geom.Rect) geom.Rect {
	lo, hi := t.Apply(b.Min), t.Apply(b.Min)
	for _, c := range [...]geom.Point{{X: b.Max.X, Y: b.Min.Y}, b.Max, {X: b.Min.X, Y: b.Max.Y}} {
		p := t.Apply(c)
		lo = geom.Pt(min(lo.X, p.X), min(lo.Y, p.Y))
		hi = geom.Pt(max(hi.X, p.X), max(hi.Y, p.Y))
	}
	return geom.Rect{Min: lo.Mul(r.scale), Max: hi.Mul(r.scale)}
}

// floatBytes is fs as the bytes it is made of.
func floatBytes(fs []float32) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&fs[0])), len(fs)*4)
}
