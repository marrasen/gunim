//go:build js && wasm

package text

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"syscall/js"
	"time"
	"unicode"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/text/unicode/norm"

	"github.com/marrasen/gunim/paint"
)

// A browser gives Go no font files, but it draws any text itself. What no face a program carries has, such as Chinese
// or an emoji, the browser draws into a canvas of its own here, for the glyph atlas. A glyph of it is a whole
// character as its reader sees it, however many runes it is, or a word of a script whose letters join.

// browserFont is the browser's own fonts in one style, as the glyphs of a [Face].
//
// The glyph of one character is numbered by it, so it holds at any time. Every other glyph, of several runes or fitted
// in a cell, is in a table, under a number never used again. A program may be shown any number of those, as a terminal
// is, so the table drops what nothing asked for or drew in the last browserRoom of them. Then those who keep glyphs ask
// anew: a new [generation] has text shaped again, and a new cells one has cells found again.
type browserFont struct {
	// before and family are the CSS of the font, around its size.
	before, family string
	// cells says the glyphs are fitted in cells.
	cells bool
	// room is how many new glyphs the table takes before it drops what was not asked for since it last did, and added
	// how many it has taken since.
	room     int
	added    int
	last     uint32
	now, old map[browserKey]*browserGlyph
	byID     map[uint32]*browserGlyph
	// widths are the characters' advances, in shares of the font's size.
	widths map[rune]float32
	// tofu is what the browser draws for a character no font has, and has which private use characters a font has.
	tofu browserTofu
	has  map[rune]bool
}

// browserKey is what a glyph of the table draws: a string, and which way it reads. With a cell, in 64ths of the font's
// size, the glyph is fitted in it: w across, up above the baseline and down below it.
type browserKey struct {
	s           string
	rtl         bool
	w, up, down int32
}

// browserGlyph is a glyph, and its advance in a share of the font's size for one outside a cell.
type browserGlyph struct {
	browserKey
	id uint32
	// text is what the browser is asked to draw.
	text    string
	color   bool
	advance float32
}

// What a glyph's number holds above the character it draws: that it reads from the right, or that it is the table's.
const (
	browserRTL   = 1 << 30
	browserTable = 1 << 31
)

const (
	// browserRoom is a table's room.
	browserRoom = 16384
	// browserWidths is how many widths are kept.
	browserWidths = 1 << 15
	// measureSize is the size text is measured at, in pixels.
	measureSize = 64
	// browserMax is the largest picture drawn, in pixels each way, which fits the glyph atlas.
	browserMax = 1900
	// wordMax is how many characters of a word whose letters join are drawn as one.
	wordMax = 24
)

var (
	// browserFaces are the faces that draw with the browser's fonts, by style: bold, italic, monospaced, and in cells.
	browserFaces [16]*Face
	browserOf    = map[*Face][2]*Face{}
	browserPad   browserCanvas
)

// browserTofu is the box a browser draws for a character no font has. same says it draws one box for all of them,
// rather than one with the character's number in it, as Firefox does.
type browserTofu struct {
	tried, same bool
	extents     browserExtents
}

// browserCanvas is the canvas the browser draws glyphs in.
type browserCanvas struct {
	tried  bool
	canvas js.Value
	ctx    js.Value
	w, h   int
	font   string
	rtl    bool
	// color says the browser has an emoji in colour, once probed.
	color, probed bool
	// pix holds the pixels read last.
	pix []byte
}

// ready makes the canvas, and reports whether there is one. It runs with mu held.
func (c *browserCanvas) ready() bool {
	if c.tried {
		return c.ctx.Truthy()
	}
	c.tried = true
	defer func() {
		// A page may forbid a canvas, or have none, as a worker may.
		if recover() != nil {
			c.ctx = js.Value{}
		}
	}()
	if doc := js.Global().Get("document"); doc.Truthy() {
		c.canvas = doc.Call("createElement", "canvas")
	} else if off := js.Global().Get("OffscreenCanvas"); off.Truthy() {
		c.canvas = off.New(1, 1)
	} else {
		return false
	}
	// Its pixels are read back after every glyph, so the browser keeps them where that is quick.
	c.ctx = c.canvas.Call("getContext", "2d", map[string]any{"willReadFrequently": true})
	if !c.ctx.Truthy() {
		return false
	}
	// A browser set to hide what it runs on may give back white or noise, as Firefox does, and that is no glyph.
	for data, i := c.ctx.Call("getImageData", 0, 0, 4, 4).Get("data"), 3; i < 64; i += 4 {
		if data.Index(i).Int() > 32 {
			c.ctx = js.Value{}
			return false
		}
	}
	return true
}

// resize makes the canvas at least w by h. That forgets its settings, so it sets them again.
func (c *browserCanvas) resize(w, h int) {
	if w <= c.w && h <= c.h {
		return
	}
	c.w, c.h = max(c.w, w, 64), max(c.h, h, 64)
	c.canvas.Set("width", c.w)
	c.canvas.Set("height", c.h)
	c.ctx.Set("font", c.font)
	c.read(c.rtl)
}

// set sets the font to f at size pixels, and which way text reads.
func (c *browserCanvas) set(f *browserFont, size float32, rtl bool) {
	if want := f.before + strconv.FormatFloat(float64(size), 'f', 3, 32) + f.family; c.font != want {
		c.ctx.Set("font", want)
		c.font = want
	}
	if rtl != c.rtl {
		c.read(rtl)
	}
}

// read sets which way text reads. Either way it is drawn from its left.
func (c *browserCanvas) read(rtl bool) {
	dir := "ltr"
	if rtl {
		dir = "rtl"
	}
	c.ctx.Set("direction", dir)
	c.ctx.Set("textAlign", "left")
	c.rtl = rtl
}

// browserExtents is how far text reaches from where it is drawn, in pixels, all growing outward.
type browserExtents struct{ advance, left, right, ascent, descent float32 }

// width returns how far s moves the pen, in pixels. The font is set.
func (c *browserCanvas) width(s string) float32 {
	return float32(c.ctx.Call("measureText", s).Get("width").Float())
}

// measure returns how far s reaches. The font is set.
func (c *browserCanvas) measure(s string) browserExtents {
	m := c.ctx.Call("measureText", s)
	get := func(name string) float32 {
		if v := m.Get(name); v.Type() == js.TypeNumber {
			return float32(v.Float())
		}
		return 0
	}
	return browserExtents{
		advance: get("width"), left: get("actualBoundingBoxLeft"), right: get("actualBoundingBoxRight"),
		ascent: get("actualBoundingBoxAscent"), descent: get("actualBoundingBoxDescent"),
	}
}

// draw draws s with its origin dx right of a pixel, and returns the pixels it covers, with sub of them to a pixel
// across, as the canvas holds them: red, green, blue and alpha, not premultiplied. The font is set.
func (c *browserCanvas) draw(s, fill string, e browserExtents, dx float32, sub int) (pix []byte, box pixelBox) {
	floor := func(v float32) int { return int(math.Floor(float64(v))) }
	ceil := func(v float32) int { return int(math.Ceil(float64(v))) }
	// Two pixels more than the browser says the text reaches, for the edge it smooths.
	x0, y0 := floor(dx-e.left)-2, floor(-e.ascent)-2
	x1, y1 := ceil(dx+e.right)+2, ceil(e.descent)+2
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 || w > browserMax || h > browserMax {
		return nil, pixelBox{}
	}
	c.resize(w*sub, h)
	c.ctx.Call("setTransform", sub, 0, 0, 1, 0, 0)
	c.ctx.Call("clearRect", 0, 0, w, h)
	c.ctx.Set("fillStyle", fill)
	c.ctx.Call("fillText", s, float64(dx)-float64(x0), -y0)
	data := c.ctx.Call("getImageData", 0, 0, w*sub, h).Get("data")
	if n := w * sub * h * 4; cap(c.pix) < n {
		c.pix = make([]byte, n)
	}
	c.pix = c.pix[:w*sub*h*4]
	js.CopyBytesToGo(c.pix, data)
	return c.pix, pixelBox{x0: x0, y0: y0, w: w, h: h}
}

// pixelBox is where pixels drawn are from the origin, and how many.
type pixelBox struct{ x0, y0, w, h int }

// platformWarm makes the canvas, and has the browser find its fonts for what is commonest of what no face has, a little
// at a time, so its frames run on between. It reports whether the browser draws.
func platformWarm() bool {
	// Before the lock, which it takes the first time.
	f := Default()
	step := func(do func()) {
		mu.Lock()
		if systemOn && browserPad.ready() {
			do()
		}
		mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	step(func() {})
	step(func() { browserFaceOf(f, false).browser.width('中') })
	step(func() { browserPad.paintsColor() })
	mu.Lock()
	defer mu.Unlock()
	return systemOn && browserPad.ready()
}

// paintsColor reports whether the browser draws emoji in colours of their own. It runs with mu held.
func (c *browserCanvas) paintsColor() bool {
	if c.probed || !c.ready() {
		return c.color
	}
	c.probed = true
	c.set(&browserFont{family: "px sans-serif"}, 32, false)
	const face = "\U0001F600"
	pix, _ := c.draw(face, "#fff", c.measure(face), 0, 1)
	for i := 0; i+3 < len(pix); i += 4 {
		if pix[i+3] > 128 && min(pix[i], pix[i+1], pix[i+2]) < 240 {
			c.color = true
			break
		}
	}
	return c.color
}

// draws reports whether the browser has a glyph for r. Only a rune for private use is in doubt: a browser draws a box
// for one, as for any it lacks, and an icon font's icons are worth showing where the user has the font, but not boxes
// in their place. It runs with mu held.
func (b *browserFont) draws(r rune) bool {
	if !unicode.Is(unicode.Co, r) {
		return true
	}
	if has, ok := b.has[r]; ok {
		return has
	}
	t := b.box()
	has := false
	if t.same {
		browserPad.set(b, measureSize, false)
		has = browserPad.measure(string(r)) != t.extents
	}
	if b.has == nil || len(b.has) >= browserWidths {
		b.has = map[rune]bool{}
	}
	b.has[r] = has
	return has
}

// box returns what the browser draws for a character no font has, found once. It runs with mu held.
func (b *browserFont) box() browserTofu {
	if b.tofu.tried {
		return b.tofu
	}
	b.tofu.tried = true
	// Two no font has. A browser whose boxes hold the rune's number draws them apart, and any rune may be a box there.
	var pix [2][]byte
	for i, s := range []string{"\U0010fffd", "\U0010fffc"} {
		browserPad.set(b, measureSize, false)
		b.tofu.extents = browserPad.measure(s)
		drawn, _ := browserPad.draw(s, "#fff", b.tofu.extents, 0, 1)
		pix[i] = slices.Clone(drawn)
	}
	b.tofu.same = slices.Equal(pix[0], pix[1])
	return b.tofu
}

// browserFaceOf returns the face that draws with the browser's fonts in f's style, in cells or not. It runs with mu
// held.
func browserFaceOf(f *Face, cells bool) *Face {
	which := 0
	if cells {
		which = 1
	}
	if b := browserOf[f][which]; b != nil {
		return b
	}
	i, before := 8*which, ""
	a := f.face.Describe().Aspect
	if a.Style == font.StyleItalic {
		i, before = i|1, "italic "
	}
	if a.Weight >= font.WeightSemibold {
		i, before = i|2, before+"bold "
	}
	family := "px sans-serif"
	wide, wok := f.face.NominalGlyph('M')
	thin, tok := f.face.NominalGlyph('i')
	if wok && tok && f.face.HorizontalAdvance(wide) == f.face.HorizontalAdvance(thin) {
		i, family = i|4, "px monospace"
	}
	if browserFaces[i] == nil {
		// Its metrics are the first face's that falls back to it.
		b := faceOf(font.NewFace(f.face.Font))
		b.browser = newBrowserFont(before, family, cells)
		browserFaces[i] = b
	}
	both := browserOf[f]
	both[which] = browserFaces[i]
	browserOf[f] = both
	return browserFaces[i]
}

func newBrowserFont(before, family string, cells bool) *browserFont {
	return &browserFont{
		before: before, family: family, cells: cells, room: browserRoom,
		now: map[browserKey]*browserGlyph{}, old: map[browserKey]*browserGlyph{}, byID: map[uint32]*browserGlyph{},
		widths: map[rune]float32{},
	}
}

// id returns the number of the glyph outside a cell that draws runes.
func (b *browserFont) id(runes []rune, rtl bool) uint32 {
	if len(runes) != 1 {
		return b.inTable(browserKey{s: string(runes), rtl: rtl}).id
	}
	id := uint32(runes[0])
	if rtl {
		id |= browserRTL
	}
	return id
}

// inTable returns the table's glyph for k, which it adds where it is not there.
func (b *browserFont) inTable(k browserKey) *browserGlyph {
	if g := b.now[k]; g != nil {
		return g
	}
	g := b.old[k]
	if g == nil {
		if b.added >= b.room {
			b.turn()
		}
		b.added++
		b.last++
		g = &browserGlyph{browserKey: k, id: b.last | browserTable}
		g.text, g.color = browserText([]rune(k.s))
		if k.w == 0 {
			browserPad.set(b, measureSize, k.rtl)
			g.advance = browserPad.width(g.text) / measureSize
		}
		b.byID[g.id] = g
	}
	b.keep(g)
	return g
}

// keep holds on to g, which is in the table. It drops no other: only a new glyph does, so what a frame draws, or a driver
// shows, holds until each of them has been kept.
func (b *browserFont) keep(g *browserGlyph) {
	if b.now[g.browserKey] != g {
		delete(b.old, g.browserKey)
		b.now[g.browserKey] = g
	}
}

// turn drops the glyphs that nothing has asked for or drawn since the turn before.
func (b *browserFont) turn() {
	for _, g := range b.old {
		delete(b.byID, g.id)
	}
	b.old, b.now, b.added = b.now, map[browserKey]*browserGlyph{}, 0
	turns.Add(1)
	if b.cells {
		cellGeneration.Add(1)
	} else {
		generation.Add(1)
	}
}

// browserText returns what the browser is asked to draw for runes, and whether that is in colours of its own.
func browserText(runes []rune) (text string, color bool) {
	text = string(runes)
	if !drawsAsEmoji(runes) || !browserPad.paintsColor() {
		return text, false
	}
	if len(runes) == 1 && runes[0] < 0x1f000 {
		// Asked for as an emoji, as a browser's plain font may have the likes of a bolt too.
		text += string(rune(emojiSelector))
	}
	return text, true
}

// glyph returns the glyph id numbers. The table may have dropped it.
func (b *browserFont) glyph(id uint32) (g browserGlyph, ok bool) {
	if id&browserTable != 0 {
		in := b.byID[id]
		if in == nil {
			return browserGlyph{}, false
		}
		b.keep(in)
		return *in, true
	}
	r := rune(id &^ browserRTL)
	if id == 0 || b.cells {
		return browserGlyph{}, false
	}
	g = browserGlyph{id: id, advance: b.width(r)}
	g.rtl = id&browserRTL != 0
	g.text, g.color = browserText([]rune{r})
	return g, true
}

// width returns r's advance, in a share of the font's size. The characters of Chinese are as wide as each other, as are
// the syllables of Korean and the emoji, so one of each is measured: a page of new ones is laid out in one go, where
// each takes the browser a fifth of a millisecond.
func (b *browserFont) width(r rune) float32 {
	like := r
	switch {
	case unicode.Is(unicode.Ideographic, r) && r >= 0x3400:
		like = '中'
	case r >= 0xac00 && r <= 0xd7a3:
		like = '한'
	case emojiPresents(r) && (r < 0x1f1e6 || r > 0x1f1ff) && browserPad.paintsColor():
		like = 0x1f600
	}
	if w, ok := b.widths[like]; ok {
		return w
	}
	if len(b.widths) >= browserWidths {
		clear(b.widths)
	}
	text, _ := browserText([]rune{like})
	browserPad.set(b, measureSize, false)
	w := browserPad.width(text) / measureSize
	b.widths[like] = w
	return w
}

// reach returns how far glyph id goes to the right of its origin, or as much above and below it, in shares of its size.
func (b *browserFont) reach(id uint32) float32 {
	g, ok := b.glyph(id)
	if !ok {
		return 0
	}
	return max(g.advance, float32(g.w)/64, float32(g.up+g.down)/64)
}

// keepGlyph holds on to the glyph id numbers.
func (b *browserFont) keepGlyph(id uint32) {
	if g := b.byID[id]; g != nil {
		b.keep(g)
	}
}

func (b *browserFont) isColor(id uint32) bool {
	g, ok := b.glyph(id)
	return ok && g.color
}

// rasterize draws a glyph at sizePx device pixels, shifted right by dx.
func (b *browserFont) rasterize(id uint32, sizePx, dx float32, lcd bool) Mask {
	mu.Lock()
	defer mu.Unlock()
	c := &browserPad
	g, ok := b.glyph(id)
	if !ok || sizePx <= 0 || !c.ready() {
		return Mask{}
	}
	c.set(b, sizePx, g.rtl)
	e := c.measure(g.text)
	var down int
	if g.w > 0 {
		e, dx, down = b.fit(g, sizePx, dx, e)
	}
	// White is what a browser smooths and thickens text of the least. The coverage is then the mask, and gunim's own.
	fill, sub := "#fff", 1
	if g.color {
		// An emoji the font lacks is a box, to be seen on any background.
		fill = "#808080"
	} else if lcd {
		sub = 3
	}
	pix, box := c.draw(g.text, fill, e, dx, sub)
	return browserMask(pix, box, sub, down, g.color)
}

// fit sets the font for g to fit its cell, and returns its extents there, and where its origin goes.
func (b *browserFont) fit(g browserGlyph, sizePx, dx float32, e browserExtents) (_ browserExtents, x float32, down int) {
	c := &browserPad
	w, up, under := float32(g.w)/64*sizePx, float32(g.up)/64*sizePx, float32(g.down)/64*sizePx
	k := float32(1)
	if wide := max(e.advance, e.left+e.right); wide > w {
		k = w / wide
	}
	if tall := e.ascent + e.descent; tall*k > up+under {
		k = (up + under) / tall
	}
	if k < 1 {
		c.set(b, sizePx*k, g.rtl)
		e = c.measure(g.text)
	}
	// In the middle, moved into the cell where the ink reaches past its advance.
	x = max(min((w-e.advance)/2, w-e.right), e.left)
	if g.color {
		x = float32(math.Round(float64(x)))
	} else {
		x += dx
	}
	// Into the cell, where the browser's fonts reach higher or lower than it.
	switch {
	case e.ascent > up:
		down = int(math.Ceil(float64(e.ascent - up)))
	case e.descent > under:
		down = -int(math.Ceil(float64(e.descent - under)))
	}
	return e, x, down
}

// browserMask cuts the pixels drawn to what they cover, and makes a glyph's mask of them, down pixels lower.
func browserMask(pix []byte, box pixelBox, sub, down int, color bool) Mask {
	w := box.w * sub
	x0, y0, x1, y1 := box.w, box.h, 0, 0
	for y := range box.h {
		for x := range w {
			if pix[(y*w+x)*4+3] != 0 {
				x0, x1 = min(x0, x/sub), max(x1, x/sub+1)
				y0, y1 = min(y0, y), max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		return Mask{}
	}
	if sub > 1 {
		// Room for the filter.
		x0, x1 = max(x0-1, 0), min(x1+1, box.w)
	}
	m := Mask{W: x1 - x0, H: y1 - y0, Color: color, LCD: sub > 1}
	m.Offset.X, m.Offset.Y = box.x0+x0, box.y0+y0+down
	if color {
		m.Pix = make([]byte, 0, m.W*m.H*4)
	} else {
		m.Pix = make([]byte, 0, m.W*sub*m.H)
	}
	for y := y0; y < y1; y++ {
		for x := x0 * sub; x < x1*sub; x++ {
			p := pix[(y*w+x)*4 : (y*w+x)*4+4]
			if !color {
				m.Pix = append(m.Pix, p[3])
				continue
			}
			a := int(p[3])
			m.Pix = append(m.Pix, byte((int(p[0])*a+127)/255), byte((int(p[1])*a+127)/255), byte((int(p[2])*a+127)/255), p[3])
		}
	}
	if sub > 1 {
		m.Pix = filterLCD(m.Pix, m.W*sub, m.H)
	}
	return m
}

// platformCell returns the glyph of the browser's that draws runes, one character, in a cell. A missing character is
// one no face has, and otherwise only one the browser draws better is its: an emoji, in colours. It runs with mu held.
func platformCell(f *Face, runes []rune, missing bool, size, w, ascent, descent float32) (paint.Glyph, bool) {
	if !systemOn || size <= 0 || w <= 0 || !browserWants(f, runes, missing) {
		return paint.Glyph{}, false
	}
	b := browserFaceOf(f, true)
	share := func(v float32) int32 { return max(int32(math.Round(float64(v/size*64))), 1) }
	g := b.browser.inTable(browserKey{s: string(runes), w: share(w), up: share(ascent), down: share(descent)})
	return paint.Glyph{ID: g.id, Face: b.id}, true
}

// platformLacks reports whether no one face has all of runes, one character, so the browser draws it. It asks before
// the character is shaped, which would put one of the browser's own among the glyphs of text.
func platformLacks(f *Face, runes []rune) bool {
	mu.Lock()
	defer mu.Unlock()
	return systemOn && browserPad.ready() && !covers((&fontmap{f: f}).resolve(runes[0]), runes)
}

// platformGlyph returns the glyph of the browser's that draws r, which no face has. It runs with mu held.
func platformGlyph(f *Face, r rune, size float32) (g paint.Glyph, advance float32, ok bool) {
	if !systemOn || !browserWants(f, []rune{r}, true) {
		return paint.Glyph{}, 0, false
	}
	b := browserFaceOf(f, false)
	return paint.Glyph{ID: b.browser.id([]rune{r}, false), Face: b.id}, b.browser.width(r) * size, true
}

// browserWants reports whether the browser draws runes, one character, that a face has or misses.
func browserWants(f *Face, runes []rune, missing bool) bool {
	switch {
	case len(runes) == 0 || !browserPad.ready():
		return false
	case drawsAsEmoji(runes) && browserPad.paintsColor():
		return true
	case !missing || !unicode.IsGraphic(runes[0]) && !unicode.Is(unicode.Co, runes[0]) || unicode.IsSpace(runes[0]):
		return false
	}
	return browserFaceOf(f, false).browser.draws(runes[0])
}

// platformRuns gives the parts of inputs no face can draw to the browser. It runs with mu held.
func platformRuns(f *Face, inputs []shaping.Input) []shaping.Input {
	if len(inputs) == 0 || !systemOn {
		return inputs
	}
	start, end := inputs[0].RunStart, inputs[len(inputs)-1].RunEnd
	runes := inputs[0].Text[start:end]
	// Most text has nothing for the browser, and this is all it costs.
	plain := true
	for i := 0; i < len(inputs) && plain; i++ {
		for _, r := range inputs[i].Text[inputs[i].RunStart:inputs[i].RunEnd] {
			if r < 0x80 {
				continue
			}
			if _, ok := inputs[i].Face.NominalGlyph(r); !ok || emojiPresents(r) || r == emojiSelector {
				plain = false
				break
			}
		}
	}
	if plain || !browserPad.ready() {
		return inputs
	}

	// mine marks the runes the browser draws, and starts those that start a character.
	mine, starts := make([]bool, len(runes)), make([]bool, len(runes))
	at := 0
	characters(runes, func(from, to int) {
		starts[from] = true
		for inputs[at].RunEnd-start <= from {
			at++
		}
		whole := to <= inputs[at].RunEnd-start
		if browserWants(f, runes[from:to], !whole || !covers(inputs[at].Face, runes[from:to])) {
			for i := from; i < to; i++ {
				mine[i] = true
			}
		}
	})
	// The spaces between two of them are the browser's too, so a line of its words is one run and not two for each.
	for i := 0; i < len(runes); i++ {
		if runes[i] != ' ' || i == 0 || !mine[i-1] {
			continue
		}
		j := i
		for j < len(runes) && runes[j] == ' ' {
			j++
		}
		for k := i; j < len(runes) && mine[j] && k < j; k++ {
			mine[k] = true
		}
		i = j
	}
	out := make([]shaping.Input, 0, len(inputs)+2)
	face := browserFaceOf(f, false).face
	for _, in := range inputs {
		for i := in.RunStart; i < in.RunEnd; {
			j := i
			for j < in.RunEnd && mine[j-start] == mine[i-start] {
				j++
			}
			part := in
			part.RunStart, part.RunEnd = i, j
			if mine[i-start] {
				part.Face = face
				// A character the faces split is whole again.
				if n := len(out) - 1; n >= 0 && out[n].Face == face && out[n].RunEnd == i && out[n].Direction == in.Direction &&
					(out[n].Script == in.Script || !starts[i-start]) {
					out[n].RunEnd = j
					i = j
					continue
				}
			}
			out = append(out, part)
			i = j
		}
	}
	return out
}

// wordParts calls each with where every part of a word whose letters join starts and ends, and what draws it there. A
// word is one part, unless it is longer than any is. Then it is cut after a letter that joins none after it, or else
// where the halves of the join are drawn by a joiner at either side.
func wordParts(runes []rune, each func(from, to int, part []rune)) {
	if len(runes) <= wordMax {
		each(0, len(runes), runes)
		return
	}
	var ends []int
	characters(runes, func(_, to int) { ends = append(ends, to) })
	from, before := 0, false
	for i := 0; i < len(ends); {
		cut := min(i+wordMax, len(ends)) - 1
		for k := cut; k > i+wordMax/2 && cut < len(ends)-1; k-- {
			if !joinsOn(runes[ends[k-1]]) {
				cut = k
				break
			}
		}
		// A long heap of marks on the way ends the part too.
		for cut > i && ends[cut]-from > 4*wordMax {
			cut--
		}
		to := ends[cut]
		start := 0
		if cut > 0 {
			start = ends[cut-1]
		}
		after := to < len(runes) && joinsOn(runes[start])
		part := make([]rune, 0, to-from+2)
		if before {
			part = append(part, joiner)
		}
		part = append(part, runes[from:to]...)
		if after {
			part = append(part, joiner)
		}
		each(from, to, part)
		from, before, i = to, after, cut+1
	}
}

// joiner is the zero width joiner, beside which a letter takes the shape it has beside another.
const joiner = 0x200d

// joinsOn reports whether a letter joins the one after it. These of Arabic do not.
func joinsOn(r rune) bool { return !strings.ContainsRune("اآأإٱدذرزژڈڑوؤةۀے", r) }

// characters calls each with where every character of runes, as a reader sees them, starts and ends.
func characters(runes []rune, each func(from, to int)) {
	var seg segmenter.Segmenter
	seg.Init(runes)
	emit := func(from, to int) {
		for ; to-from > characterMax; from += characterMax {
			each(from, from+characterMax)
		}
		each(from, to)
	}
	from, to := 0, 0
	for it := seg.GraphemeIterator(); it.Next(); {
		g := it.Grapheme()
		// The segmenter ends an emoji at its joiner where another emoji stands before it.
		if to > from && (runes[to-1] != joiner || !emojiBase(g.Text[0])) {
			emit(from, to)
			from = to
		}
		to = g.Offset + len(g.Text)
	}
	if to > from {
		emit(from, to)
	}
}

// covers reports whether a font has all of a character, put together or taken apart.
func covers(face *font.Face, runes []rune) bool {
	all := func(runes []rune) bool {
		for _, r := range runes {
			if hidden(r) {
				continue
			}
			if _, ok := face.NominalGlyph(r); !ok {
				return false
			}
		}
		return true
	}
	return all(runes) || all([]rune(norm.NFC.String(string(runes)))) || all([]rune(norm.NFD.String(string(runes))))
}

// joins are the scripts whose letters join, so a word of one is drawn whole.
var joins = []language.Script{
	language.Arabic, language.Syriac, language.Nko, language.Mongolian, language.Mandaic, language.Adlam,
	language.Hanifi_Rohingya, language.Phags_Pa,
}

// platformShape shapes a run the browser draws: a glyph for each character, or each word of a script whose letters
// join. ok is false for any other run. It runs with mu held.
func platformShape(in shaping.Input, size float32) (out shaping.Output, ok bool) {
	b := byFont[in.Face]
	if b == nil || b.browser == nil {
		return shaping.Output{}, false
	}
	rtl := in.Direction.Progression() == di.TowardTopLeft
	runes := in.Text[in.RunStart:in.RunEnd]
	out = shaping.Output{
		Size: toFixed(size), Direction: in.Direction, Face: in.Face,
		Runes: shaping.Range{Offset: in.RunStart, Count: len(runes)},
	}
	scale := size / b.upem
	out.LineBounds = shaping.Bounds{
		Ascent: toFixed(b.ascent * scale), Descent: -toFixed(b.descent * scale), Gap: toFixed(b.gap * scale),
	}
	add := func(from, to int, part []rune) {
		id := b.browser.id(part, rtl)
		g, _ := b.browser.glyph(id)
		advance := toFixed(g.advance * size)
		wide := advance
		if len(part) == 1 && part[0] == ' ' {
			// A line may end at it.
			wide = 0
		}
		out.Glyphs = append(out.Glyphs, shaping.Glyph{
			Width: wide, Height: -toFixed((b.ascent + b.descent) * scale), YBearing: toFixed(b.ascent * scale), Advance: advance,
			GlyphID:      font.GID(id),
			ClusterIndex: in.RunStart + from, RuneCount: to - from, GlyphCount: 1, //nolint:staticcheck // Deprecated to read, not to set.
		})
	}
	if slices.Contains(joins, in.Script) {
		for from := 0; from < len(runes); {
			to := from + 1
			for runes[from] != ' ' && to < len(runes) && runes[to] != ' ' {
				to++
			}
			wordParts(runes[from:to], func(i, j int, part []rune) { add(from+i, from+j, part) })
			from = to
		}
	} else {
		characters(runes, func(from, to int) { add(from, to, runes[from:to]) })
	}
	if rtl {
		slices.Reverse(out.Glyphs)
	}
	out.RecalculateAll()
	return out, true
}
