package render

import (
	"image/color"
	"math"
	"time"
	"unsafe"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// A grid of character cells is drawn by a program of its own: one quad
// over the rows, whose fragments look their cell up in a texture that
// holds each cell's background, foreground and pattern. A row costs the
// GPU twelve bytes a cell, where drawing its cells as rectangles costs
// hundreds.
//
// Under a scale, as in a zoomed view, a cell is seldom a whole number of
// pixels. The cells then go on whole pixels, some a pixel wider or taller
// than the rest, and each draws its pattern made at its own size, so a
// line in box drawing lands on whole pixels and none falls between
// them. The glyphs rasterize at the size they show at, as text's do.

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

// scaledMost is how many masks made at other sizes a renderer keeps, for
// grids drawn by turns under a scale.
const scaledMost = 8192

// cellsState is a renderer's means of drawing cells.
type cellsState struct {
	prog         program
	failed       bool
	vao, vbo     uint32
	tex          uint32
	texW         int
	row          int
	uStep, uLo   int32
	uSlot, uKind int32
	uBase, uPer  int32
	// pats is the texture of patterns, the Patterns it holds and at which
	// sizes, how many of their masks are uploaded, and how many it has
	// room for.
	pats     uint32
	patsOf   *paint.Patterns
	patsAt   cellSizes
	patsSent int
	patsRoom int
	patsPer  int
	// scaled are masks made at sizes other than their Patterns', by
	// Patterns, mask and size.
	scaled  map[scaledMask][]byte
	pending *paint.CellsOp
	// grid is where the run of rows queued lands on the pixels, and on
	// says it lands on the device's pixels rather than under a turn.
	grid     cellGrid
	on       bool
	firstRow int
	rows     int
	upload   []byte
	verts    [16]float32

	// The glyphs of the run of rows queued: table holds where each is in
	// the atlas and in its cell, two texels a glyph, uploaded after the
	// rows, and index numbers them by their keys and places. mode is 0
	// for greyscale glyphs, 1 or 2 for glyphs on subpixels that run red
	// to blue or blue to red. later are the glyphs drawn as text over the
	// cells: those that reach outside their cells, and those drawn
	// stretched while a zoom moves.
	table []byte
	index map[cellGlyph]uint16
	mode  int
	later []laterText
	// placed is the row being added's glyphs, found before the row
	// joins a run.
	placed                                         []placedGlyph
	uMode, uGamma, uContrast, uTable, uTexW, uCols int32
	// inCells and spilled count the glyphs drawn each way, for tests.
	inCells, spilled int

	// zooms is how each grid's glyphs are sized, by the grid, and frame
	// counts the frames drawn, for them.
	zooms map[uintptr]*cellZoom
	frame int
}

// cellGrid is where a run of cells lands: cell x of the run's row y
// covers the pixels from at plus edge(x) to at plus edge(x+1) across, and
// likewise down. step is a cell's size in pixels, whole at rest, and with
// any fraction under a scale.
type cellGrid struct {
	at   [2]int
	step [2]float32
}

// edge is where cell i starts along axis 0, across, or 1, down, from the
// grid's corner, in whole pixels. The cells program finds it the same
// way.
func (g cellGrid) edge(i, axis int) int {
	return int(math.Floor(float64(float32(float32(i)*g.step[axis])) + 0.5))
}

// cellSizes are the sizes a grid's cells come in, in pixels: lo and
// lo plus one across and down where a cell's size has a fraction, or lo
// alone where it has none.
type cellSizes struct {
	lo, hi [2]int
}

// kinds is how many sizes the cells come in.
func (c cellSizes) kinds() int { return (c.hi[0] - c.lo[0] + 1) * (c.hi[1] - c.lo[1] + 1) }

// sizes returns the sizes the grid's cells come in.
func (g cellGrid) sizes() cellSizes {
	var c cellSizes
	for a := range 2 {
		lo := int(math.Floor(float64(g.step[a])))
		c.lo[a] = max(lo, 1)
		c.hi[a] = c.lo[a]
		if float32(lo) != g.step[a] {
			c.hi[a] = c.lo[a] + 1
		}
	}
	return c
}

// scaledMask names a mask of a Patterns made at another size.
type scaledMask struct {
	pats    *paint.Patterns
	i, w, h int
}

// cellGlyph is a glyph's place in a run's table: its key, and where it
// lies from its cell's corner, which under a scale differs from cell to
// cell.
type cellGlyph struct {
	key    glyphKey
	dx, dy int
}

// laterText is text drawn over the cells, rasterized at raster device
// pixels when not zero, and otherwise at the size it shows at.
type laterText struct {
	op     *paint.TextOp
	raster float32
}

// cellZoom is how a grid's glyphs are sized while its scale changes, as
// a big mask's is (see bigmasks.go): at the size they show at, which a
// grid at rest keeps; but while its scale moves, from one frame to the
// next, they draw as text stretched from the size last made, and are
// made at the new size once the scale holds for a frame, or every
// bigMaskStale while it never does. A zoom then fills the atlas with a
// few sizes of a screen of glyphs, rather than one each frame.
type cellZoom struct {
	// sharp is the size last made and made when.
	sharp float32
	made  time.Time
	// frame is the frame the grid was last drawn in, k its scale then,
	// and want, size and stretch what was found for it.
	frame         int
	k             float32
	want, size    float32
	stretch, seen bool
}

// placedGlyph is where a cell's glyph is: its key and slot in the
// atlas, and its corner in device pixels. how says it goes in the cells,
// is drawn as text, or has nothing to draw yet.
type placedGlyph struct {
	key    glyphKey
	slot   glyphSlot
	gx, gy int
	how    glyphWay
}

// glyphWay is how a cell's glyph is drawn.
type glyphWay uint8

const (
	// asText draws the glyph as text, over the cells.
	asText glyphWay = iota
	// inCell draws it with the cells, where it fits them.
	inCell
	// notYet draws nothing of it: a space, or a glyph the browser is yet
	// to make.
	notYet
)

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
uniform sampler2D u_atlas;
uniform sampler2D u_lcd;
uniform vec2 u_step;
uniform ivec2 u_lo;
uniform ivec2 u_slot;
uniform ivec2 u_kind;
uniform int u_base;
uniform int u_per;
uniform int u_mode;
uniform vec4 u_gamma;
uniform float u_contrast;
uniform int u_table;
uniform int u_texw;
uniform int u_cols;
in vec2 v_px;

int u16(float lo, float hi) { return int(lo * 255.0 + 0.5) + 256 * int(hi * 255.0 + 0.5); }

// edge is where cell i starts, in whole pixels from the run's corner,
// for cells step pixels apart: as cellGrid.edge finds it.
int edge(int i, float step) { return int(floor(float(i) * step + 0.5)); }

// cellOf is the cell that holds pixel p.
int cellOf(int p, float step) {
	int c = int(floor((float(p) + 0.5) / step));
	if (edge(c, step) > p) {
		c--;
	} else if (edge(c + 1, step) <= p) {
		c++;
	}
	return c;
}

// coverage returns the coverage of each channel of the glyph pixel at,
// for text in colour c, enhanced and corrected for gamma as the draw
// program's glyphs are.
vec3 coverage(ivec2 at, vec4 c) {
	vec4 g = u_gamma;
	float k = u_contrast;
	if (u_mode == 0) {
		float a = texelFetch(u_atlas, at, 0).r;
		k *= clamp(4.0 * (0.75 - dot(c.rgb, vec3(0.30, 0.59, 0.11))), 0.0, 1.0);
		a = a * (k + 1.0) / (a * k + 1.0);
		float f = dot(c.rgb, vec3(0.25, 0.5, 0.25));
		a = clamp(a + a * (1.0 - a) * ((g.x * f + g.y) * a + (g.z * f + g.w)), 0.0, 1.0);
		return vec3(a * c.a);
	}
	vec3 m = texelFetch(u_lcd, at, 0).rgb;
	if (u_mode == 2) {
		m = m.bgr;
	}
	m = m * (k + 1.0) / (m * k + 1.0);
	return clamp(m + m * (1.0 - m) * ((g.x * c.rgb + g.y) * m + (g.z * c.rgb + g.w)), 0.0, 1.0) * c.a;
}

void main() {
	ivec2 p = ivec2(floor(v_px));
	ivec2 cell = ivec2(clamp(cellOf(p.x, u_step.x), 0, u_cols - 1), max(cellOf(p.y, u_step.y), 0));
	ivec2 start = ivec2(edge(cell.x, u_step.x), edge(cell.y, u_step.y));
	ivec2 size = ivec2(edge(cell.x + 1, u_step.x), edge(cell.y + 1, u_step.y)) - start;
	ivec2 inside = clamp(p - start, ivec2(0), max(size - 1, ivec2(0)));
	int x = cell.x * 3;
	int y = u_base + cell.y;
	vec4 bg = texelFetch(u_cells, ivec2(x, y), 0);
	vec4 fg = texelFetch(u_cells, ivec2(x + 1, y), 0);
	vec4 pt = texelFetch(u_cells, ivec2(x + 2, y), 0);
	int id = u16(pt.r, pt.g);
	float a = 0.0;
	if (id > 0) {
		// The pattern made at this cell's size: the narrower or the wider,
		// the shorter or the taller.
		ivec2 kind = min(ivec2(greaterThan(size, u_lo)), u_kind - 1);
		int k = (id - 1) * u_kind.x * u_kind.y + kind.y * u_kind.x + kind.x;
		inside = min(inside, u_lo + kind - 1);
		a = texelFetch(u_patterns, ivec2((k % u_per) * u_slot.x + inside.x, (k / u_per) * u_slot.y + inside.y), 0).r;
	}
	vec4 f = vec4(fg.rgb * fg.a, fg.a) * a;
	vec3 col = f.rgb + bg.rgb * bg.a * (1.0 - f.a);
	vec3 cover = vec3(f.a + bg.a * (1.0 - f.a));
	// The glyphs of this cell and the cells either side, which may
	// reach into it, in the order text draws them: left to right.
	for (int d = -1; d <= 1; d++) {
		int nx = cell.x + d;
		if (nx < 0 || nx >= u_cols) {
			continue;
		}
		vec4 np = texelFetch(u_cells, ivec2(nx * 3 + 2, y), 0);
		int gl = u16(np.b, np.a);
		if (gl == 0) {
			continue;
		}
		int t = (gl - 1) * 2;
		vec4 e0 = texelFetch(u_cells, ivec2(t % u_texw, u_table + t / u_texw), 0);
		vec4 e1 = texelFetch(u_cells, ivec2((t + 1) % u_texw, u_table + (t + 1) / u_texw), 0);
		ivec2 gsize = ivec2(int(e1.r * 255.0 + 0.5), int(e1.g * 255.0 + 0.5));
		ivec2 off = ivec2(int(e1.b * 255.0 + 0.5), int(e1.a * 255.0 + 0.5)) - 128;
		ivec2 q = p - ivec2(edge(nx, u_step.x), start.y) - off;
		if (q.x >= 0 && q.y >= 0 && q.x < gsize.x && q.y < gsize.y) {
			vec4 nfg = d == 0 ? fg : texelFetch(u_cells, ivec2(nx * 3 + 1, y), 0);
			vec3 m = coverage(ivec2(u16(e0.r, e0.g), u16(e0.b, e0.a)) + q, nfg);
			col = nfg.rgb * m + col * (1.0 - m);
			cover = m + cover * (1.0 - m);
		}
	}
	float alpha = max(cover.r, max(cover.g, cover.b));
	fragColor = vec4(col, alpha);
#ifdef DUAL
	fragCover = vec4(cover, alpha);
#endif
}
`

// cellsReady builds what drawing cells needs, once, and reports whether
// it can.
func (r *Renderer) cellsReady() bool {
	s := &r.cellsState
	if s.prog.id != 0 || s.failed {
		return !s.failed
	}
	g := r.GL
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
	g.Uniform1i(g.GetUniformLocation(id, "u_atlas"), 0)
	g.Uniform1i(g.GetUniformLocation(id, "u_lcd"), 2)
	s.uMode = g.GetUniformLocation(id, "u_mode")
	s.uGamma = g.GetUniformLocation(id, "u_gamma")
	s.uContrast = g.GetUniformLocation(id, "u_contrast")
	s.uTable = g.GetUniformLocation(id, "u_table")
	s.uTexW = g.GetUniformLocation(id, "u_texw")
	s.uCols = g.GetUniformLocation(id, "u_cols")
	s.uStep = g.GetUniformLocation(id, "u_step")
	s.uLo = g.GetUniformLocation(id, "u_lo")
	s.uSlot = g.GetUniformLocation(id, "u_slot")
	s.uKind = g.GetUniformLocation(id, "u_kind")
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
func (r *Renderer) cells(op *paint.CellsOp) {
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
	t := op.Transform
	plain := t.A == 1 && t.B == 0 && t.D == 0 && t.E == 1
	mode := 0
	if r.subpixels && plain && r.depth == 0 {
		mode = 1
		if r.textRendering.Smoothing == text.SubpixelBGR {
			mode = 2
		}
	}
	grid, on := r.cellGridOf(op, cw, ch)
	// Glyphs go in the cells at rest and under an upright scale, at the
	// size they show at, unless that size is still moving.
	sizePx, stretch := op.TextSize*r.scale, false
	k, upright := uprightScale(t)
	if upright {
		sizePx, stretch = r.cellGlyphSize(op, k)
	}
	// Found first: finding a glyph can start the atlas again, which
	// draws what is queued.
	r.placeGlyphs(op, upright && on && !stretch, sizePx, mode)
	// Rows join the run before them only with nothing drawn in between,
	// which would end up beneath them, and with room for their glyphs.
	if p := s.pending; p == nil || len(r.verts) > 0 || !follows(p, op, s.rows) || s.mode != mode ||
		s.firstRow+s.rows+1+s.tableRows(len(op.Cells)) > cellsRows {
		r.flush()
		r.startCells(op, grid, on)
		s.mode = mode
	}
	var raster float32
	if stretch {
		raster = sizePx
		r.stretched(r.deviceRect(t, geom.Rect{Min: op.At, Max: end}))
	}
	r.addRow(op, raster)
	s.rows++
}

// cellGridOf returns where op's cells land, for cells cw by ch device
// pixels at rest, and whether they land on the device's pixels: at rest
// and under any scale that keeps them upright, but not under a turn or a
// skew, where they are drawn at rest and turned.
func (r *Renderer) cellGridOf(op *paint.CellsOp, cw, ch int) (cellGrid, bool) {
	t := op.Transform
	if t.B != 0 || t.D != 0 || !(t.A > 0) || !(t.E > 0) {
		return cellGrid{step: [2]float32{float32(cw), float32(ch)}}, false
	}
	o := t.Apply(op.At)
	g := cellGrid{at: [2]int{int(math.Round(float64(o.X * r.scale))), int(math.Round(float64(o.Y * r.scale)))}}
	for a, v := range [2]float32{float32(cw) * t.A, float32(ch) * t.E} {
		// A size within a thousandth of a whole pixel is that pixel, so a
		// rounding slip does not make every so many cells a pixel wider.
		if n := float32(math.Round(float64(v))); n >= 1 && math.Abs(float64(v-n)) < 1e-3 {
			v = n
		}
		g.step[a] = v
	}
	return g, true
}

// cellGlyphSize returns the size, in device pixels, op's glyphs
// rasterize at under an upright scale k, and whether they draw stretched
// from it, as text, as cellZoom tells.
func (r *Renderer) cellGlyphSize(op *paint.CellsOp, k float32) (float32, bool) {
	s := &r.cellsState
	want := op.TextSize * r.scale
	if k != 1 {
		// As text under a scale rasterizes.
		want = max(float32(math.Round(float64(want*k)*4))/4, 0.25)
	}
	if s.zooms == nil {
		s.zooms = map[uintptr]*cellZoom{}
	}
	z := s.zooms[op.Grid]
	if z == nil {
		z = &cellZoom{sharp: want, made: r.now}
		s.zooms[op.Grid] = z
	}
	if z.seen && z.frame == s.frame {
		if z.want == want {
			return z.size, z.stretch
		}
		// The grid drawn again in the same frame at another size, as in a
		// small copy of a window: made at its size, sharp.
		return want, false
	}
	// Moving: the frame before drew the grid at another scale. A change
	// of the screen's scale, or a jump, is no zoom to wait out.
	moving := z.seen && z.frame == s.frame-1 && z.k != k
	z.seen, z.frame, z.k, z.want = true, s.frame, k, want
	switch {
	case want == z.sharp:
		z.size, z.stretch = want, false
	case moving && r.now.Sub(z.made) < bigMaskStale:
		z.size, z.stretch = z.sharp, true
	default:
		z.sharp, z.made = want, r.now
		z.size, z.stretch = want, false
	}
	return z.size, z.stretch
}

// cellsFrame starts a frame for the cells: it counts it, and lets go of
// the sizes of grids not drawn lately.
func (s *cellsState) cellsFrame() {
	s.frame++
	if len(s.zooms) > 64 {
		for id, z := range s.zooms {
			if z.frame < s.frame-2 {
				delete(s.zooms, id)
			}
		}
	}
}

// follows reports whether op is the row after the rows of p queued.
func follows(p, op *paint.CellsOp, rows int) bool {
	return p.Grid == op.Grid && op.Row == p.Row+rows && p.Patterns == op.Patterns && p.Size == op.Size &&
		p.Transform == op.Transform && p.At.X == op.At.X && len(p.Cells) == len(op.Cells)
}

// startCells begins a run of rows with op, landing as grid says,
// uploading any of its patterns not yet on the GPU at the sizes its cells
// come in.
func (r *Renderer) startCells(op *paint.CellsOp, grid cellGrid, on bool) {
	s := &r.cellsState
	g := r.GL
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
	if s.row+2 > cellsRows {
		s.row = 0
	}
	s.pending, s.firstRow, s.rows = op, s.row, 0
	s.grid, s.on = grid, on
	s.upload = s.upload[:0]
	s.table = s.table[:0]
	if s.index == nil {
		s.index = map[cellGlyph]uint16{}
	}
	clear(s.index)
	r.sendPatterns(op.Patterns, grid.sizes())
}

// sendPatterns uploads the masks of pats the GPU does not have yet, made
// at each of the sizes the cells come in.
func (r *Renderer) sendPatterns(pats *paint.Patterns, at cellSizes) {
	s := &r.cellsState
	g := r.GL
	n := len(pats.Masks)
	kinds := at.kinds()
	if s.patsOf != pats || s.patsAt != at || n > s.patsRoom {
		if s.pats != 0 {
			g.DeleteTexture(s.pats)
		}
		s.patsPer = max(patternsWidth/at.hi[0], 1)
		s.patsRoom = max(256, 2*n)
		rows := (s.patsRoom*kinds + s.patsPer - 1) / s.patsPer
		s.pats = r.cellsTexture(patternsUnit, glR8, glRed, s.patsPer*at.hi[0], rows*at.hi[1], 1)
		s.patsOf, s.patsAt, s.patsSent = pats, at, 0
	}
	if s.patsSent == n {
		return
	}
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.pats)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	across := at.hi[0] - at.lo[0] + 1
	for k := s.patsSent; k < n; k++ {
		for kind := range kinds {
			w, h := at.lo[0]+kind%across, at.lo[1]+kind/across
			m := r.patternMask(pats, k, w, h)
			if m == nil {
				continue
			}
			slot := k*kinds + kind
			x, y := (slot%s.patsPer)*at.hi[0], (slot/s.patsPer)*at.hi[1]
			g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h), glRed, gl.UNSIGNED_BYTE, m)
		}
	}
	g.ActiveTexture(gl.TEXTURE0)
	s.patsSent = n
}

// patternMask returns mask i of pats at w by h device pixels: its own
// at its own size, and otherwise made by pats at that size, or shrunk or
// stretched from its own. It returns nil for a mask of the wrong length.
func (r *Renderer) patternMask(pats *paint.Patterns, i, w, h int) []byte {
	m := pats.Masks[i]
	if len(m) != pats.W*pats.H {
		return nil
	}
	if w == pats.W && h == pats.H {
		return m
	}
	s := &r.cellsState
	key := scaledMask{pats, i, w, h}
	if made, ok := s.scaled[key]; ok {
		return made
	}
	var made []byte
	if pats.MaskAt != nil {
		made = pats.MaskAt(i, w, h)
	}
	if len(made) != w*h {
		made = resizeMask(m, pats.W, pats.H, w, h)
	}
	if s.scaled == nil || len(s.scaled) >= scaledMost {
		s.scaled = map[scaledMask][]byte{}
	}
	s.scaled[key] = made
	return made
}

// resizeMask shrinks or stretches m, sw by sh pixels, to w by h. Each
// pixel takes the most covered of the pixels of m it lies over, so a line
// a pixel thick stays whole, and sharp, at any size.
func resizeMask(m []byte, sw, sh, w, h int) []byte {
	out := make([]byte, w*h)
	span := func(i, from, to int) (int, int) {
		lo := i * from / to
		hi := ((i+1)*from + to - 1) / to
		return lo, max(hi, lo+1)
	}
	for y := range h {
		y0, y1 := span(y, sh, h)
		for x := range w {
			x0, x1 := span(x, sw, w)
			var v byte
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					v = max(v, m[yy*sw+xx])
				}
			}
			out[y*w+x] = v
		}
	}
	return out
}

// cellsTexture makes a texture of w by h texels on unit, read texel by
// texel.
func (r *Renderer) cellsTexture(unit uint32, internal int32, format uint32, w, h, channels int) uint32 {
	g := r.GL
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

// tableRows is how many rows of the texture the glyph table takes, with
// room for n more glyphs.
func (s *cellsState) tableRows(n int) int {
	if s.texW == 0 {
		return 1
	}
	return (len(s.table)/4 + 2*n + s.texW - 1) / s.texW
}

// placeGlyphs finds the glyphs of op's cells in the atlas at sizePx
// device pixels, and where each lands, into placed. With inCells false,
// or for a colour glyph, all are drawn as text.
func (r *Renderer) placeGlyphs(op *paint.CellsOp, inCells bool, sizePx float32, mode int) {
	s := &r.cellsState
	for range 3 {
		epochs := [2]int{r.glyphs.epoch, r.lcdGlyphs.epoch}
		s.placed = s.placed[:0]
		raster := text.Raster{Hint: r.textRendering.Hinting == text.HintingLight, LCD: mode != 0}
		var (
			face   *text.Face
			faceID uint32
		)
		for x, c := range op.Cells {
			if !c.Text {
				continue
			}
			pg := placedGlyph{}
			if gly := c.Glyph; inCells && sizePx > 0 {
				if face == nil || gly.Face != faceID {
					face, _ = text.Lookup(gly.Face)
					faceID = gly.Face
				}
				if face != nil && !face.IsColor(gly.ID) {
					o := op.Transform.Apply(geom.Pt(op.At.X+float32(x)*op.Size.W+gly.At.X, op.At.Y+op.Baseline))
					ox, oy := o.X*r.scale, float32(math.Round(float64(o.Y*r.scale)))
					fx := float32(math.Floor(float64(ox)))
					sh := int(math.Round(float64(ox-fx) * subpixel))
					if sh == subpixel {
						fx, sh = fx+1, 0
					}
					pg.key = glyphKeyFor(faceID, gly.ID, sizePx, uint8(sh), raster)
					made := fromBrowser(face)
					if made && browserSize(face, gly.ID, sizePx) < sizePx {
						// Stretched, as text.
						s.placed = append(s.placed, pg)
						continue
					}
					if made && r.later(pg.key, geom.Pt(fx, oy), sizePx, true) {
						pg.how = notYet
						s.placed = append(s.placed, pg)
						continue
					}
					var slot glyphSlot
					var ok bool
					if made {
						slot, ok = r.browserSlot(pg.key, face, sizePx)
					} else {
						slot, ok = r.glyph(pg.key, func() text.Mask {
							return face.Rasterize(pg.key.id, sizePx, float32(pg.key.shift)/subpixel, pg.key.raster)
						})
					}
					pg.slot, pg.gx, pg.gy = slot, int(fx)+slot.off.X, int(oy)+slot.off.Y
					switch {
					case ok && slot.w == 0:
						// Nothing to draw, as a space has.
						pg.how, pg.slot = notYet, glyphSlot{}
					case ok:
						pg.how = inCell
					}
					if made && r.fade(pg.key, geom.Pt(fx, oy), sizePx, true) < 1 {
						// Faded in, as text.
						pg.how = asText
					}
				}
			}
			s.placed = append(s.placed, pg)
		}
		if epochs == [2]int{r.glyphs.epoch, r.lcdGlyphs.epoch} {
			return
		}
		// The atlas started again part way: what was found before is gone.
	}
}

// addRow adds op's cells to the rows uploaded together when they are
// drawn, one upload a run of rows rather than one a row, with their
// glyphs from placed, and the glyphs that do not fit their cells to
// later, rasterized at raster device pixels when not zero.
func (r *Renderer) addRow(op *paint.CellsOp, raster float32) {
	s := &r.cellsState
	b := s.upload
	n := 0
	var spill map[color.NRGBA]*paint.TextOp
	gr := s.grid
	top, bottom := gr.at[1]+gr.edge(s.rows, 1), gr.at[1]+gr.edge(s.rows+1, 1)
	for x, c := range op.Cells {
		var glyphAt uint16
		if c.Text {
			pg := s.placed[n]
			n++
			how := pg.how
			if how == inCell {
				// Into the cells either side, which look for it, but not
				// the rows above and below, which may be drawn apart.
				left, right := gr.at[0]+gr.edge(x-1, 0), gr.at[0]+gr.edge(x+2, 0)
				dx, dy := pg.gx-(gr.at[0]+gr.edge(x, 0)), pg.gy-top
				sl := pg.slot
				if pg.gx < left || pg.gx+sl.w > right || pg.gy < top || pg.gy+sl.h > bottom ||
					dx < -128 || dx >= 128 || dy >= 128 || sl.w >= 256 || sl.h >= 256 {
					how = asText
				} else {
					at := cellGlyph{pg.key, dx, dy}
					i, ok := s.index[at]
					if !ok {
						s.table = append(s.table, byte(sl.x), byte(sl.x>>8), byte(sl.y), byte(sl.y>>8),
							byte(sl.w), byte(sl.h), byte(dx+128), byte(dy+128))
						i = uint16(len(s.table) / 8)
						s.index[at] = i
					}
					glyphAt = i
					s.inCells++
				}
			}
			if how == asText {
				s.spilled++
				if spill == nil {
					spill = map[color.NRGBA]*paint.TextOp{}
				}
				t := spill[c.FG]
				if t == nil {
					t = &paint.TextOp{Size: op.TextSize, Color: c.FG, Transform: op.Transform}
					spill[c.FG] = t
					s.later = append(s.later, laterText{t, raster})
				}
				gly := c.Glyph
				gly.At = geom.Pt(op.At.X+float32(x)*op.Size.W+gly.At.X, op.At.Y+op.Baseline)
				t.Glyphs = append(t.Glyphs, gly)
			}
		}
		b = append(b, c.BG.R, c.BG.G, c.BG.B, c.BG.A, c.FG.R, c.FG.G, c.FG.B, c.FG.A,
			byte(c.Pattern), byte(c.Pattern>>8), byte(glyphAt), byte(glyphAt>>8))
	}
	s.upload = b
}

// flushCells draws the rows of cells queued, if any.
func (r *Renderer) flushCells() {
	s := &r.cellsState
	op := s.pending
	if op == nil || s.rows == 0 {
		s.pending = nil
		return
	}
	s.pending = nil
	g := r.GL
	g.ActiveTexture(cellsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.tex)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, int32(s.firstRow), int32(len(op.Cells)*3), int32(s.rows), gl.RGBA, gl.UNSIGNED_BYTE, s.upload)
	tableRow := s.firstRow + s.rows
	if n := len(s.table) / 4; n > 0 {
		// The table, row after row of the texture, after the cells'.
		for at, row := 0, tableRow; at < n; at, row = at+s.texW, row+1 {
			w := min(s.texW, n-at)
			g.TexSubImage2D(gl.TEXTURE_2D, 0, 0, int32(row), int32(w), 1, gl.RGBA, gl.UNSIGNED_BYTE, s.table[at*4:(at+w)*4])
		}
	}
	if s.mode != 0 {
		g.ActiveTexture(glTexture2)
		g.BindTexture(gl.TEXTURE_2D, r.lcdGlyphs.tex)
	}
	gr := s.grid
	w, h := gr.edge(len(op.Cells), 0), gr.edge(s.rows, 1)
	if s.on {
		// The quad covers the pixels the cells land on, and each fragment
		// finds its pixel from the run's corner.
		x0, y0 := float32(gr.at[0]), float32(gr.at[1])
		sx, sy := 2/float32(r.fbW), 2/float32(r.fbH)
		for i, c := range [4][2]float32{{0, 0}, {float32(w), 0}, {float32(w), float32(h)}, {0, float32(h)}} {
			s.verts[i*4+0] = (x0+c[0])*sx - 1
			s.verts[i*4+1] = 1 - (y0+c[1])*sy
			s.verts[i*4+2] = c[0]
			s.verts[i*4+3] = c[1]
		}
	} else {
		// Turned: the cells at rest, turned as the quad is.
		lw, lh := op.Size.W*float32(len(op.Cells)), op.Size.H*float32(s.rows)
		local := [4]geom.Point{op.At, geom.Pt(op.At.X+lw, op.At.Y), geom.Pt(op.At.X+lw, op.At.Y+lh), geom.Pt(op.At.X, op.At.Y+lh)}
		sx, sy := 2*r.scale/float32(r.fbW), 2*r.scale/float32(r.fbH)
		for i, l := range local {
			p := op.Transform.Apply(l)
			s.verts[i*4+0] = p.X*sx - 1
			s.verts[i*4+1] = 1 - p.Y*sy
			s.verts[i*4+2] = (l.X - op.At.X) * r.scale
			s.verts[i*4+3] = (l.Y - op.At.Y) * r.scale
		}
	}
	at := s.patsAt
	g.UseProgram(s.prog.id)
	g.BindVertexArray(s.vao)
	g.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	g.BufferData(gl.ARRAY_BUFFER, floatBytes(s.verts[:]), gl.STREAM_DRAW)
	g.ActiveTexture(cellsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.tex)
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.pats)
	g.Uniform2fv(s.uStep, gr.step[:])
	g.Uniform2iv(s.uLo, []int32{int32(at.lo[0]), int32(at.lo[1])})
	g.Uniform2iv(s.uSlot, []int32{int32(at.hi[0]), int32(at.hi[1])})
	g.Uniform2iv(s.uKind, []int32{int32(at.hi[0] - at.lo[0] + 1), int32(at.hi[1] - at.lo[1] + 1)})
	g.Uniform1i(s.uBase, int32(s.firstRow))
	g.Uniform1i(s.uPer, int32(s.patsPer))
	g.Uniform1i(s.uMode, int32(s.mode))
	g.Uniform4fv(s.uGamma, r.gamma[:])
	contrast := float32(greyContrast)
	if s.mode != 0 {
		contrast = lcdContrast
	}
	g.Uniform1fv(s.uContrast, []float32{contrast})
	g.Uniform1i(s.uTable, int32(tableRow))
	g.Uniform1i(s.uTexW, int32(s.texW))
	g.Uniform1i(s.uCols, int32(len(op.Cells)))
	g.DrawElements(gl.TRIANGLES, 6, glUnsignedShort, 0)
	r.draws++
	if framesDebug {
		r.Stats.Flushes++
		r.Stats.Quads++
		r.Stats.Bytes += len(s.upload) + len(s.verts)*4
	}
	s.row = tableRow + s.tableRows(0)
	r.bindDraw()
	// The glyphs drawn as text, over the cells.
	later := s.later
	s.later = nil
	for _, t := range later {
		r.rasterAt = t.raster
		r.text(t.op)
	}
	r.rasterAt = 0
}

// releaseCells frees what drawing cells made.
func (r *Renderer) releaseCells() {
	s := &r.cellsState
	g := r.GL
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
func (r *Renderer) deviceRect(t paint.Transform, b geom.Rect) geom.Rect {
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
