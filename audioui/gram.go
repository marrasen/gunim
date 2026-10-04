package audioui

import (
	"image"
	"math"
	"math/cmplx"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// A file's spectrogram, made as its file is read: its pitches over its
// length, a column every GramHop frames, each of GramRows rows of pitch
// from 20 Hz to 20 kHz, evenly by octaves, of its level in decibels,
// packed in a byte.
const (
	// GramHop is the frames between columns, and GramSize the frames
	// each is taken over.
	GramHop  = 1024
	GramSize = 4096
	// GramRows are the rows of pitch.
	GramRows = 160
	// GramFloor is the level, in decibels, of a byte of 0, and a byte
	// is GramStep decibels.
	GramFloor = -120.0
	GramStep  = 0.5
)

// Gram is a file's spectrogram: Cols columns of GramRows bytes each,
// the lowest pitch first, a column every GramHop frames at Rate.
type Gram struct {
	Cols int
	Rate int
	Data []byte
}

// At is column c's level at row r, in decibels.
func (g *Gram) At(c, r int) float64 {
	return GramFloor + GramStep*float64(g.Data[c*GramRows+r])
}

// GramScan makes a file's spectrogram from its frames as they are read.
type GramScan struct {
	g      *Gram
	fft    *audio.FFT
	window []float64
	// ring holds the last GramSize frames, mono, filled to n, and since
	// counts the frames since the last column.
	ring  []float64
	n     int
	since int
	x     []complex128
	// rows are the bins each row of pitch spans.
	rows [GramRows][2]int
}

// NewGramScan returns a scan of a file at rate.
func NewGramScan(rate int) *GramScan {
	s := &GramScan{g: &Gram{Rate: rate}, fft: audio.NewFFT(GramSize), window: make([]float64, GramSize),
		ring: make([]float64, GramSize), x: make([]complex128, GramSize)}
	for i := range s.window {
		s.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/GramSize)
	}
	bin := func(hz float64) float64 { return hz * GramSize / float64(rate) }
	for r := range GramRows {
		lo := bin(20 * math.Pow(1000, float64(r)/GramRows))
		hi := bin(20 * math.Pow(1000, float64(r+1)/GramRows))
		b0 := int(math.Round(lo))
		b1 := max(b0+1, int(math.Round(hi)))
		s.rows[r] = [2]int{min(b0, GramSize/2-1), min(b1, GramSize/2)}
	}
	// Half a window of silence first, so the first column is centred on
	// the first frame.
	s.n = GramSize / 2
	return s
}

// Write takes frames, interleaved stereo.
func (s *GramScan) Write(frames []float32) {
	for i := 0; i+1 < len(frames); i += 2 {
		s.ring[s.n%GramSize] = float64(frames[i]+frames[i+1]) / 2
		s.n++
		s.since++
		if s.since == GramHop {
			s.since = 0
			s.column()
		}
	}
}

// column adds the column of the last GramSize frames.
func (s *GramScan) column() {
	for i := range GramSize {
		s.x[i] = complex(s.ring[(s.n+i)%GramSize]*s.window[i], 0)
	}
	s.fft.Transform(s.x)
	// A full-scale sine is a magnitude of a quarter of the size, through
	// the window.
	norm := 4.0 / GramSize
	for _, b := range &s.rows {
		var peak float64
		for k := b[0]; k < b[1]; k++ {
			peak = max(peak, cmplx.Abs(s.x[k]))
		}
		db := 20 * math.Log10(max(peak*norm, 1e-9))
		s.g.Data = append(s.g.Data, byte(max(0, min((db-GramFloor)/GramStep, 255))))
	}
	s.g.Cols++
}

// Done returns the spectrogram, its last columns run on through half a
// window of silence.
func (s *GramScan) Done() *Gram {
	s.Write(make([]float32, 2*(GramSize/2)))
	return s.g
}

// GramTiles is a file's spectrogram drawn as images, at levels each
// four times coarser than the one before, for views zoomed out, in
// tiles of GramTileCols columns each. Drawing them takes some
// milliseconds, so make them once for a file and keep them.
type GramTiles struct {
	g      *Gram
	levels []gramLevel
}

type gramLevel struct {
	// per is the spectrogram's columns a column of the level spans.
	per   int
	tiles []*paint.Image
}

// GramTileCols is how many columns a tile of [GramTiles] holds.
const GramTileCols = 256

// NewGramTiles draws g's columns as tiles.
func NewGramTiles(g *Gram) *GramTiles {
	t := &GramTiles{g: g}
	cols, n := g.Data, g.Cols
	for per := 1; ; per *= 4 {
		t.levels = append(t.levels, gramLevel{per: per, tiles: gramImages(cols, n)})
		if n <= 2048 {
			break
		}
		// Four columns to one, each row its loudest.
		m := (n + 3) / 4
		next := make([]byte, m*GramRows)
		for c := range m {
			row := next[c*GramRows : (c+1)*GramRows]
			for k := 4 * c; k < min(4*c+4, n); k++ {
				for r, v := range cols[k*GramRows : (k+1)*GramRows] {
					row[r] = max(row[r], v)
				}
			}
		}
		cols, n = next, m
	}
	return t
}

// Tile returns the size of the finest level's first tile, for tests.
func (t *GramTiles) Tile() (w, h int) {
	if len(t.levels) == 0 || len(t.levels[0].tiles) == 0 {
		return 0, 0
	}
	return t.levels[0].tiles[0].Size()
}

// gramPalette is the colour of each byte of a spectrogram, premade.
var gramPalette = func() (p [256][4]byte) {
	for b := range p {
		c := GramColor(GramShade(float32(GramFloor + GramStep*float64(b))))
		p[b] = [4]byte{c.R, c.G, c.B, c.A}
	}
	return p
}()

// GramShade is a level, in decibels, as a file's spectrogram colours
// it, from 0 to 1: from -96 dB to -6, eased at either end.
func GramShade(db float32) float32 {
	t := min(max((db+96)/90, 0), 1)
	return t * t * (3 - 2*t)
}

// gramImages draws n columns of bytes as tiles, the highest pitch at
// the top.
func gramImages(cols []byte, n int) []*paint.Image {
	var out []*paint.Image
	for c0 := 0; c0 < n; c0 += GramTileCols {
		w := min(GramTileCols, n-c0)
		img := image.NewRGBA(image.Rect(0, 0, w, GramRows))
		for c := range w {
			col := cols[(c0+c)*GramRows : (c0+c+1)*GramRows]
			for r, v := range col {
				o := img.PixOffset(c, GramRows-1-r)
				copy(img.Pix[o:o+4], gramPalette[v][:])
			}
		}
		out = append(out, paint.NewImage(img))
	}
	return out
}

// Paint draws the spectrogram over area, the file's time t seconds at
// xOf(t), at the level that gives a column a pixel or more, and the
// pitches marked.
func (t *GramTiles) Paint(p *paint.Painter, th *theme.Live, area geom.Rect, xOf func(t float64) float32) {
	g := t.g
	if g.Cols == 0 || len(t.levels) == 0 {
		return
	}
	colSecs := float64(GramHop) / float64(g.Rate)
	x0, x1 := xOf(0), xOf(colSecs)
	perPx := 1 / max(float64(x1-x0), 1e-9)
	lv := &t.levels[0]
	for i := range t.levels {
		if float64(t.levels[i].per) <= max(perPx, 1) {
			lv = &t.levels[i]
		}
	}
	end := p.Layer(paint.LayerOpts{Bounds: area, Opacity: 1, Clip: true})
	defer end()
	p.RRect(area, 0, paint.Solid(Ground.Get(th)))
	for i, img := range lv.tiles {
		w, _ := img.Size()
		c0 := i * GramTileCols * lv.per
		x0, x1 := xOf(float64(c0)*colSecs), xOf(float64(c0+w*lv.per)*colSecs)
		if x1 < area.Min.X || x0 > area.Max.X {
			continue
		}
		p.Image(img, geom.Rc(x0, area.Min.Y, x1-x0, area.Size().H), paint.ImageOpts{Opacity: 1})
	}
	PaintPitches(p, th, area, true)
}
