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

// patternsWidth is the widest a texture of patterns is, which holds them
// side by side in rows.
const patternsWidth = 2048

// patternMost is the widest pattern made, in pixels, and twice it the
// tallest: cells bigger than that, under a big zoom, stretch patterns
// made at it.
const patternMost = 128

// patternTextures is how many textures of patterns a renderer keeps, one
// for each grid and size of its cells drawn lately.
const patternTextures = 8

// scaledBytes is how many bytes of masks made at other sizes a renderer
// keeps, for grids drawn by turns under a scale.
const scaledBytes = 16 << 20

// edgeShift is how many bits of a fraction of a pixel a grid's corner and
// a cell's size keep. The cells program finds the edges of cells with the
// same whole numbers as the renderer does, so the two never disagree.
const (
	edgeShift = 12
	edgeHalf  = 1 << (edgeShift - 1)
	// edgeMost bounds the grid's corner and the far edge of its cells,
	// in fractions, so no sum overflows 32 bits.
	edgeMost = 1 << 29
)

// cellsState is a renderer's means of drawing cells.
type cellsState struct {
	prog        program
	failed      bool
	vao, vbo    uint32
	tex         uint32
	texW        int
	row         int
	uO, uS      int32
	uWhole      int32
	uRow0       int32
	uRows       int32
	uLo, uSlot  int32
	uKind, uFit int32
	uBase, uPer int32
	// maxTex is the biggest texture the GPU takes.
	maxTex int
	// pats are the textures of patterns, each for a Patterns at the sizes
	// its cells come in, and cur the one the run queued draws from.
	pats []*patternTexture
	cur  *patternTexture
	// scaled are masks made at sizes other than their Patterns', by
	// Patterns, mask and size, and scaledSize their bytes.
	scaled     map[scaledMask][]byte
	scaledSize int
	pending    *paint.CellsOp
	// grid is where the run of rows queued lands, on says it lands on
	// the device's pixels rather than under a turn, and row0 is the
	// grid's row the run starts at, or 0 under a turn.
	grid     cellGrid
	on       bool
	row0     int
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
	// inCells and spilled count the glyphs drawn each way, and
	// patternsMade the textures of patterns made, for tests.
	inCells, spilled, patternsMade int

	// zooms is how the glyphs of each drawing of a grid are sized, by
	// the grid, claimed the one each grid and transform took this frame,
	// and frame counts the frames drawn.
	zooms   map[uintptr][]*cellZoom
	claimed map[zoomClaim]*cellZoom
	frame   int
}

// cellGrid is where a grid's cells land: cell i, across or down, starts
// at edge(i), in whole pixels. o is the grid's corner and s a cell's
// size, in 1/2^edgeShift pixels: whole at rest, and with any fraction
// under a scale. Every row of a grid lands on the same edges, whichever
// run it is drawn in.
type cellGrid struct {
	o, s [2]int32
}

// edge is where cell i starts along axis 0, across, or 1, down. The
// cells program finds it the same way.
func (g cellGrid) edge(i, axis int) int {
	return int((g.o[axis] + int32(i)*g.s[axis] + edgeHalf) >> edgeShift)
}

// whole reports whether the cells are a whole number of pixels, each as
// wide and as tall as every other.
func (g cellGrid) whole() bool {
	const frac = 1<<edgeShift - 1
	return g.s[0]&frac == 0 && g.s[1]&frac == 0
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
		c.lo[a] = max(int(g.s[a]>>edgeShift), 1)
		c.hi[a] = c.lo[a]
		if g.s[a]&(1<<edgeShift-1) != 0 {
			c.hi[a] = c.lo[a] + 1
		}
	}
	return c
}

// patternTexture is a texture of a Patterns' masks, made at each of the
// sizes its cells come in, side by side, per to a row of the texture,
// each in a slot as big as the biggest. Where those sizes are too big,
// fit is set and the masks are made at one smaller size, made, and
// stretched to the cells.
type patternTexture struct {
	tex  uint32
	pats *paint.Patterns
	at   cellSizes
	// made are the sizes the masks are made at: at's, or one smaller.
	made cellSizes
	fit  bool
	// per is how many slots a row holds, room how many masks the texture
	// has room for and sent how many are on it.
	per, room, sent int
	// used is when it was last drawn from.
	used time.Time
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

// zoomClaim names one drawing of a grid in a frame: the grid, and the
// transform it is drawn under, the same in every pass over the frame's
// damage.
type zoomClaim struct {
	grid uintptr
	t    paint.Transform
}

// zoomNear is how far apart, as a ratio, the scales of a drawing of a
// grid may be from one frame to the next and still be the same drawing.
// A grid shown twice, as a pane and its small copy in an overview, has a
// state for each, and a frame that draws only one of them takes the
// state nearest its scale, never the other's.
const zoomNear = 1.25

// zoomsMost is how many drawings of one grid keep their states.
const zoomsMost = 4

// cellZoom is how a drawing of a grid has its glyphs sized while its
// scale changes, as a big mask's is (see bigmasks.go): at the size they
// show at, which a grid at rest keeps; but while its scale moves, from
// one frame to the next, they draw as text stretched from the size last
// made, and are made at the new size once the scale holds for a frame,
// or every bigMaskStale while it never does. A zoom then fills the atlas
// with a few sizes of a screen of glyphs, rather than one each frame.
type cellZoom struct {
	// sharp is the size last made and made when.
	sharp float32
	made  time.Time
	// frame is the frame the grid was last drawn in, and at when, k its
	// scale then, and want, size and stretch what was found for it.
	frame         int
	at            time.Time
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

// The cells program's edges are cellGrid's: whole numbers, in 1/4096
// pixel, so it and the renderer agree to the pixel.
const cellsFragment = `
uniform sampler2D u_cells;
uniform sampler2D u_patterns;
uniform sampler2D u_atlas;
uniform sampler2D u_lcd;
uniform ivec2 u_o;
uniform ivec2 u_s;
uniform int u_whole;
uniform int u_row0;
uniform int u_rows;
uniform ivec2 u_lo;
uniform ivec2 u_slot;
uniform ivec2 u_kind;
uniform int u_fit;
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

// edge is where cell i starts, in whole pixels, for a grid whose corner
// is o and whose cells are s apart, in 1/4096 pixel.
int edge(int i, int o, int s) { return (o + i * s + 2048) >> 12; }

// cellOf is the cell that holds pixel p: found in floats, and set right
// by the edges.
int cellOf(int p, int o, int s) {
	int c = int(floor((float(p) + 0.5 - float(o) / 4096.0) * 4096.0 / float(s)));
	if (edge(c, o, s) > p) {
		c--;
	} else if (edge(c + 1, o, s) <= p) {
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
	ivec2 lo = ivec2(0, u_row0), hi = ivec2(u_cols - 1, u_row0 + u_rows - 1);
	ivec2 cell, start, size;
	// left is where the cell before starts.
	int left;
	if (u_whole != 0) {
		// Cells a whole number of pixels, as at rest: a division finds
		// the cell, and the edges are a cell apart.
		size = u_s >> 12;
		ivec2 e0 = (u_o + 2048) >> 12;
		cell = clamp(ivec2(floor((v_px - vec2(e0)) / vec2(size))), lo, hi);
		start = e0 + cell * size;
		left = start.x - size.x;
	} else {
		cell = clamp(ivec2(cellOf(p.x, u_o.x, u_s.x), cellOf(p.y, u_o.y, u_s.y)), lo, hi);
		start = ivec2(edge(cell.x, u_o.x, u_s.x), edge(cell.y, u_o.y, u_s.y));
		size = ivec2(edge(cell.x + 1, u_o.x, u_s.x), edge(cell.y + 1, u_o.y, u_s.y)) - start;
		left = edge(cell.x - 1, u_o.x, u_s.x);
	}
	ivec2 inside = clamp(p - start, ivec2(0), max(size - 1, ivec2(0)));
	int x = cell.x * 3;
	int y = u_base + cell.y - u_row0;
	vec4 bg = texelFetch(u_cells, ivec2(x, y), 0);
	vec4 fg = texelFetch(u_cells, ivec2(x + 1, y), 0);
	vec4 pt = texelFetch(u_cells, ivec2(x + 2, y), 0);
	int id = u16(pt.r, pt.g);
	float a = 0.0;
	if (id > 0) {
		int k = id - 1;
		ivec2 at;
		if (u_fit != 0) {
			// Made smaller than the cell, and stretched to it.
			at = inside * u_slot / max(size, ivec2(1));
		} else {
			// The pattern made at this cell's size: the narrower or the
			// wider, the shorter or the taller.
			ivec2 kind = min(ivec2(greaterThan(size, u_lo)), u_kind - 1);
			k = k * u_kind.x * u_kind.y + kind.y * u_kind.x + kind.x;
			at = min(inside, u_lo + kind - 1);
		}
		a = texelFetch(u_patterns, ivec2((k % u_per) * u_slot.x + at.x, (k / u_per) * u_slot.y + at.y), 0).r;
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
		int nstart = d < 0 ? left : (d == 0 ? start.x : start.x + size.x);
		ivec2 q = p - ivec2(nstart, start.y) - off;
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
	s.uO = g.GetUniformLocation(id, "u_o")
	s.uS = g.GetUniformLocation(id, "u_s")
	s.uWhole = g.GetUniformLocation(id, "u_whole")
	s.uRow0 = g.GetUniformLocation(id, "u_row0")
	s.uRows = g.GetUniformLocation(id, "u_rows")
	s.uLo = g.GetUniformLocation(id, "u_lo")
	s.uSlot = g.GetUniformLocation(id, "u_slot")
	s.uKind = g.GetUniformLocation(id, "u_kind")
	s.uFit = g.GetUniformLocation(id, "u_fit")
	s.uBase = g.GetUniformLocation(id, "u_base")
	s.uPer = g.GetUniformLocation(id, "u_per")
	s.maxTex = max(g.GetInteger(gl.MAX_TEXTURE_SIZE), 1024)

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
// skew, where they are drawn at rest and turned. The grid's corner is
// that of its first row, op.Row rows above op's.
func (r *Renderer) cellGridOf(op *paint.CellsOp, cw, ch int) (cellGrid, bool) {
	t := op.Transform
	turned := cellGrid{s: [2]int32{int32(cw) << edgeShift, int32(ch) << edgeShift}}
	if t.B != 0 || t.D != 0 || !(t.A > 0) || !(t.E > 0) {
		return turned, false
	}
	o := t.Apply(geom.Pt(op.At.X, op.At.Y-float32(op.Row)*op.Size.H)).Mul(r.scale)
	far := float64(max(len(op.Cells), op.Row+cellsRows) + 2)
	var g cellGrid
	for a, v := range [2][2]float32{{o.X, float32(cw) * t.A}, {o.Y, float32(ch) * t.E}} {
		corner, step := float64(v[0]), float64(v[1])
		// A size within a thousandth of a whole pixel is that pixel, so a
		// rounding slip does not make every so many cells a pixel wider.
		if n := math.Round(step); n >= 1 && math.Abs(step-n) < 1e-3 {
			step = n
		}
		oq, sq := math.Round(corner*(1<<edgeShift)), math.Round(step*(1<<edgeShift))
		if sq < 1 || math.Abs(oq) >= edgeMost || math.Abs(oq)+far*sq >= edgeMost {
			// Too far off, or too big, for the fractions: drawn as under
			// a turn, stretched from the cells at rest. Left so on purpose:
			// a grid this far off, half a million pixels, is off screen.
			return turned, false
		}
		g.o[a], g.s[a] = int32(oq), int32(sq)
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
		s.zooms, s.claimed = map[uintptr][]*cellZoom{}, map[zoomClaim]*cellZoom{}
	}
	claim := zoomClaim{op.Grid, op.Transform}
	z := s.claimed[claim]
	if z != nil && z.want == want {
		return z.size, z.stretch
	}
	if z == nil {
		z = s.zoomFor(op.Grid, k, want, r.now)
		s.claimed[claim] = z
	}
	// Moving: the frame before drew it at another scale. A change of the
	// screen's scale, or a jump, is no zoom to wait out.
	moving := z.seen && z.frame == s.frame-1 && z.k != k
	z.seen, z.frame, z.at, z.k, z.want = true, s.frame, r.now, k, want
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

// zoomFor returns the state of the drawing of grid at scale k, which no
// other drawing has taken this frame: the one whose scale was nearest
// k, within zoomNear, or a new one, which lets go of the one drawn
// longest ago past zoomsMost.
func (s *cellsState) zoomFor(grid uintptr, k, want float32, now time.Time) *cellZoom {
	zooms := s.zooms[grid]
	var near *cellZoom
	far := float32(zoomNear)
	for _, z := range zooms {
		if z.frame == s.frame && z.seen {
			continue
		}
		if d := max(k/z.k, z.k/k); d <= far {
			near, far = z, d
		}
	}
	if near != nil {
		return near
	}
	if len(zooms) >= zoomsMost {
		old := 0
		for i, z := range zooms {
			if z.at.Before(zooms[old].at) {
				old = i
			}
		}
		zooms = append(zooms[:old], zooms[old+1:]...)
	}
	z := &cellZoom{sharp: want, made: now, k: k}
	s.zooms[grid] = append(zooms, z)
	return z
}

// cellsFrame starts a frame for the cells.
func (s *cellsState) cellsFrame() {
	s.frame++
	clear(s.claimed)
}

// evictCells lets go of what drawing cells keeps for grids the window
// has stopped drawing: textures of patterns, and how their drawings'
// glyphs are sized.
func (r *Renderer) evictCells() {
	s := &r.cellsState
	kept := s.pats[:0]
	for _, t := range s.pats {
		if r.now.Sub(t.used) > imageIdle {
			r.GL.DeleteTexture(t.tex)
			if s.cur == t {
				s.cur = nil
			}
			continue
		}
		kept = append(kept, t)
	}
	clear(s.pats[len(kept):])
	s.pats = kept
	for grid, zooms := range s.zooms {
		live := zooms[:0]
		for _, z := range zooms {
			if r.now.Sub(z.at) <= imageIdle {
				live = append(live, z)
			}
		}
		if len(live) == 0 {
			delete(s.zooms, grid)
			continue
		}
		s.zooms[grid] = live
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
	s.grid, s.on, s.row0 = grid, on, 0
	if on {
		s.row0 = op.Row
	}
	s.upload = s.upload[:0]
	s.table = s.table[:0]
	if s.index == nil {
		s.index = map[cellGlyph]uint16{}
	}
	clear(s.index)
	s.cur = r.sendPatterns(op.Patterns, grid.sizes())
}

// sendPatterns returns the texture of pats' masks at the sizes at, with
// those the GPU does not have yet uploaded. Each Patterns and sizes keeps
// a texture of its own while it is drawn, so a grid drawn twice a frame
// at two sizes makes neither again.
func (r *Renderer) sendPatterns(pats *paint.Patterns, at cellSizes) *patternTexture {
	s := &r.cellsState
	g := r.GL
	n := len(pats.Masks)
	var pt *patternTexture
	for _, t := range s.pats {
		if t.pats == pats && t.at == at {
			pt = t
			break
		}
	}
	if pt == nil || n > pt.room {
		if pt == nil {
			if len(s.pats) >= patternTextures {
				// The one drawn longest ago goes.
				old := 0
				for i, t := range s.pats {
					if t.used.Before(s.pats[old].used) {
						old = i
					}
				}
				g.DeleteTexture(s.pats[old].tex)
				s.pats = append(s.pats[:old], s.pats[old+1:]...)
			}
			pt = &patternTexture{pats: pats, at: at}
			s.pats = append(s.pats, pt)
		} else {
			g.DeleteTexture(pt.tex)
		}
		r.layOutPatterns(pt, n)
		pt.tex = r.cellsTexture(patternsUnit, glR8, glRed, pt.per*pt.made.hi[0],
			(pt.room*pt.made.kinds()+pt.per-1)/pt.per*pt.made.hi[1], 1)
		pt.sent = 0
		s.patternsMade++
	}
	pt.used = r.now
	if pt.sent == n {
		return pt
	}
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, pt.tex)
	g.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	made := pt.made
	kinds, across := made.kinds(), made.hi[0]-made.lo[0]+1
	for k := pt.sent; k < n; k++ {
		for kind := range kinds {
			w, h := made.lo[0]+kind%across, made.lo[1]+kind/across
			m := r.patternMask(pats, k, w, h)
			if m == nil {
				continue
			}
			slot := k*kinds + kind
			x, y := (slot%pt.per)*made.hi[0], (slot/pt.per)*made.hi[1]
			g.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(w), int32(h), glRed, gl.UNSIGNED_BYTE, m)
		}
	}
	g.ActiveTexture(gl.TEXTURE0)
	pt.sent = n
	return pt
}

// layOutPatterns sets the sizes pt's masks are made at and how they lie
// in its texture, with room for n masks and more. Cells too big for the
// texture, or past patternMost, have their masks made at one size small
// enough, and stretched. Left so on purpose: such cells, past 128 by 256
// device pixels, come only with a zoom far in, where a pixel more or
// less of a line does not show.
func (r *Renderer) layOutPatterns(pt *patternTexture, n int) {
	most := r.cellsState.maxTex
	width := min(patternsWidth, most)
	made, fit := pt.at, false
	pt.room = max(64, 2*n)
	for {
		slot := made.hi
		pt.per = max(width/slot[0], 1)
		rows := (pt.room*made.kinds() + pt.per - 1) / pt.per
		small := slot[0] <= patternMost && slot[1] <= 2*patternMost
		if small && slot[0] <= width && rows*slot[1] <= most || slot == [2]int{1, 1} {
			break
		}
		// A size three quarters as big, the same each way, for every cell.
		fit = true
		w := max(min(slot[0]*3/4, patternMost), 1)
		h := max(min(slot[1]*3/4, 2*patternMost), 1)
		made = cellSizes{lo: [2]int{w, h}, hi: [2]int{w, h}}
	}
	pt.made, pt.fit = made, fit
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
	if s.scaled == nil || s.scaledSize+len(made) > scaledBytes {
		s.scaled, s.scaledSize = map[scaledMask][]byte{}, 0
	}
	s.scaled[key] = made
	s.scaledSize += len(made)
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
	gr, row := s.grid, s.row0+s.rows
	top, bottom := gr.edge(row, 1), gr.edge(row+1, 1)
	for x, c := range op.Cells {
		var glyphAt uint16
		if c.Text {
			pg := s.placed[n]
			n++
			how := pg.how
			if how == inCell {
				// Into the cells either side, which look for it, but not
				// the rows above and below, which may be drawn apart.
				left, right := gr.edge(x-1, 0), gr.edge(x+2, 0)
				dx, dy := pg.gx-gr.edge(x, 0), pg.gy-top
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
	if s.on {
		// The quad covers the pixels the cells land on, and each fragment
		// finds its cell by its pixel.
		x0, x1 := float32(gr.edge(0, 0)), float32(gr.edge(len(op.Cells), 0))
		y0, y1 := float32(gr.edge(s.row0, 1)), float32(gr.edge(s.row0+s.rows, 1))
		sx, sy := 2/float32(r.fbW), 2/float32(r.fbH)
		for i, c := range [4][2]float32{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} {
			s.verts[i*4+0] = c[0]*sx - 1
			s.verts[i*4+1] = 1 - c[1]*sy
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
	pt := s.cur
	made := pt.made
	whole := int32(0)
	if gr.whole() {
		whole = 1
	}
	fit := int32(0)
	if pt.fit {
		fit = 1
	}
	g.UseProgram(s.prog.id)
	g.BindVertexArray(s.vao)
	g.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	g.BufferData(gl.ARRAY_BUFFER, floatBytes(s.verts[:]), gl.STREAM_DRAW)
	g.ActiveTexture(cellsUnit)
	g.BindTexture(gl.TEXTURE_2D, s.tex)
	g.ActiveTexture(patternsUnit)
	g.BindTexture(gl.TEXTURE_2D, pt.tex)
	g.Uniform2iv(s.uO, gr.o[:])
	g.Uniform2iv(s.uS, gr.s[:])
	g.Uniform1i(s.uWhole, whole)
	g.Uniform1i(s.uRow0, int32(s.row0))
	g.Uniform1i(s.uRows, int32(s.rows))
	g.Uniform2iv(s.uLo, []int32{int32(made.lo[0]), int32(made.lo[1])})
	g.Uniform2iv(s.uSlot, []int32{int32(made.hi[0]), int32(made.hi[1])})
	g.Uniform2iv(s.uKind, []int32{int32(made.hi[0] - made.lo[0] + 1), int32(made.hi[1] - made.lo[1] + 1)})
	g.Uniform1i(s.uFit, fit)
	g.Uniform1i(s.uBase, int32(s.firstRow))
	g.Uniform1i(s.uPer, int32(pt.per))
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
	for _, t := range s.pats {
		g.DeleteTexture(t.tex)
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
