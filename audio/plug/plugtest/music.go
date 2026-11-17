package plugtest

import (
	"math"
	"sync"
)

// Music is a [plug.Sound] like a mix playing, for a view to draw in a test: a bass line, a chord and drum hits under
// a level that swells and falls over a few seconds. Each time a listener asks how far it has played, it plays a
// sixtieth of a second more, so a view stepped frame by frame hears it move.
type Music struct {
	// Level is how loud it plays at its loudest, from 0 to 1. Zero plays at 0.7.
	Level float64

	mu sync.Mutex
	at int64
	// made holds the last frames, from frame from on, left and right in turn, before Level, as many as the longest read.
	made    []float64
	from    int64
	longest int
	// worked counts the frames worked out.
	worked int64
}

// musicRate is the rate Music plays at.
const musicRate = 48000

// Heard implements [plug.Sound].
func (m *Music) Heard(dst []float32) (frames int, at int64, rate int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(dst) == 0 {
		m.at += musicRate / 60
		return 0, m.at, musicRate
	}
	n := min(len(dst)/2, int(m.at))
	level := m.Level
	if level <= 0 {
		level = 0.7
	}
	src := m.last(n)
	for i, v := range src {
		dst[i] = float32(level * v)
	}
	return n, m.at, musicRate
}

// last returns the n frames before at, left and right in turn, before Level, working out only those made lacks.
func (m *Music) last(n int) []float64 {
	m.longest = max(m.longest, n)
	lo := max(0, m.at-int64(m.longest))
	end := m.from + int64(len(m.made)/2)
	if lo < m.from || end < lo {
		m.made, m.from, end = m.made[:0], lo, lo
	}
	if drop := lo - m.from; drop > 0 {
		m.made, m.from = m.made[2*drop:], lo
	}
	for f := end; f < m.at; f++ {
		l, r := sample(f, musicRate)
		m.made = append(m.made, l, r)
		m.worked++
	}
	return m.made[len(m.made)-2*n:]
}

// sample is the music at frame f, left and right, at most about 1.
func sample(f int64, hz float64) (l, r float64) {
	t := float64(f) / hz
	beat := math.Mod(t, 0.5)
	swell := 0.55 + 0.45*math.Sin(2*math.Pi*t/6)
	bass := 0.35 * math.Sin(2*math.Pi*55*t*(1+float64(int(t*2)%4)/8))
	kick := 0.6 * math.Exp(-beat*18) * math.Sin(2*math.Pi*(50+120*math.Exp(-beat*30))*t)
	chord := 0.12 * (math.Sin(2*math.Pi*262*t) + math.Sin(2*math.Pi*330*t) + math.Sin(2*math.Pi*392*t))
	hat := 0.15 * math.Exp(-math.Mod(t, 0.25)*60) * noise(f)
	l = swell * (bass + kick + chord + hat)
	r = swell * (bass + kick + 0.8*chord + 1.2*hat)
	return l, r
}

// noise is white noise from -1 to 1, the same for the same frame.
func noise(f int64) float64 {
	x := uint64(f)*0x9e3779b97f4a7c15 + 0x632be59bd9b4e019
	x ^= x >> 31
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 29
	return float64(x>>11)/float64(1<<52) - 1
}
