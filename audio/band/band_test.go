package band

import (
	"io"
	"math"
	"testing"
	"testing/fstest"

	"github.com/marrasen/gunim/audio"
)

// ramp is a part that plays from one value to another, in both
// channels, over n frames.
type ramp struct {
	from, to float32
	n, i     int64
}

// at is what the ramp plays at frame f.
func (s *ramp) at(f int64) float32 { return s.from + (s.to-s.from)*float32(f)/float32(s.n) }

func (s *ramp) Len() int64 { return s.n }

func (s *ramp) SeekFrame(f int64) error {
	s.i = f
	return nil
}

func (s *ramp) Read(dst []float32) (int, error) {
	k := int(min(int64(len(dst)/2), s.n-s.i))
	for j := range k {
		v := s.at(s.i + int64(j))
		dst[2*j], dst[2*j+1] = v, v
	}
	s.i += int64(k)
	if s.i >= s.n {
		return k, io.EOF
	}
	return k, nil
}

// The fake parts: each as long as the real one, and each running on
// from the last as a bounce does, the intro into the loop and the loop
// into the outro. Only the loop come round again jumps, from its end,
// -0.5, to its start, 0.5.
var (
	fakeSong  = Wander{BPM: 136}
	fakeIntro = ramp{from: 0.1, to: 0.5, n: phraseAt(1)}
	fakeLoop  = ramp{from: 0.5, to: -0.5, n: phraseAt(1)}
	fakeOutro = ramp{from: -0.5, to: 0, n: barAt(4)}
)

// barAt and phraseAt return where bar b and phrase p of fakeSong start.
func barAt(b int) int64    { return fakeSong.BarAt(b) }
func phraseAt(p int) int64 { return fakeSong.phraseAt(p) }

// The defaults the tests count on.
const (
	maxLoops = 4
	soloRest = 6
)

// newBand starts a band of fakeSong with parts, seeded by seed.
func newBand(parts []Part, seed uint64) *Band {
	s := fakeSong
	s.Parts = parts
	return New(&s, seed)
}

// fakeParts returns n parts with the fake pieces, and a solo of 20
// bars.
func fakeParts(n int) []Part {
	open := func(p Piece) (audio.Source, error) {
		var r ramp
		switch p {
		case Intro:
			r = fakeIntro
		case Loop:
			r = fakeLoop
		case Outro:
			r = fakeOutro
		default:
			r = ramp{from: 0.3, to: 0.3, n: barAt(20)}
		}
		return &r, nil
	}
	var out []Part
	for range n {
		out = append(out, Part{Name: "fake", Open: open})
	}
	return append(out, Part{Name: "solo", Solo: true, Open: open})
}

// conductTo moves b on to the start of phrase p, without playing it.
func conductTo(b *Band, p int) {
	for b.phrase < p {
		b.phrase++
		b.at = phraseAt(b.phrase)
		b.turn()
	}
}

// loopTimes has b's one part, as it comes in, loop n times.
func loopTimes(b *Band, n int) {
	l := b.layers[0]
	l.loops, l.next.loops = n-1, n-1
	b.want = 1
}

func TestEachPartComesInLoopsAndLeavesAndSomethingAlwaysPlays(t *testing.T) {
	const phrases = 400
	b := newBand(fakeParts(9), 7)
	states := make([][]int, 0, phrases)
	counts := map[int]bool{}
	for p := range phrases {
		conductTo(b, p)
		row := make([]int, len(b.layers))
		playing, looping := 0, 0
		for i, l := range b.layers {
			row[i] = l.state
			if l.state == inIntro || l.state == inLoop {
				playing++
			}
			if l.state == inLoop {
				looping++
			}
		}
		if playing == 0 {
			t.Fatalf("phrase %d: nothing plays", p)
		}
		counts[playing] = true
		if p > 0 && row[9] == inSolo && states[p-1][9] != inSolo && looping < 2 {
			t.Fatalf("phrase %d: the solo starts over %d parts looping", p, looping)
		}
		states = append(states, row)
	}
	if len(counts) < 6 {
		t.Fatalf("over %d phrases, %v parts played: want the number to wander", phrases, counts)
	}
	// Each part's phrases: rest, an intro, one to maxLoops loops, an
	// outro, a rest again.
	for i := range 9 {
		loops, ins := 0, 0
		for p := 1; p < phrases; p++ {
			was, is := states[p-1][i], states[p][i]
			ok := false
			switch is {
			case resting:
				ok = was == resting || was == inOutro
			case inIntro:
				ok = was == resting
				ins++
			case inLoop:
				ok = was == inIntro || was == inLoop
				loops++
				if loops > maxLoops {
					t.Fatalf("part %d loops more than %d times by phrase %d", i, maxLoops, p)
				}
			case inOutro:
				ok = was == inLoop
				loops = 0
			}
			if !ok {
				t.Fatalf("part %d goes from %d to %d at phrase %d", i, was, is, p)
			}
		}
		if ins < 3 {
			t.Fatalf("part %d came in %d times in %d phrases", i, ins, phrases)
		}
	}
	// The solo, from time to time, soloRest phrases apart at least.
	solos, last := 0, -soloRest
	for p := 1; p < phrases; p++ {
		if states[p][9] == inSolo && states[p-1][9] != inSolo {
			if p-last < soloRest {
				t.Fatalf("the solo starts at phrase %d, %d after the last", p, p-last)
			}
			solos, last = solos+1, p
		}
	}
	if solos < 5 {
		t.Fatalf("the solo played %d times in %d phrases", solos, phrases)
	}
}

func TestPartsChangeOnTheirPhraseAndALoopComesRoundWithoutAJump(t *testing.T) {
	b := newBand(fakeParts(1)[:1], 1)
	l := b.layers[0]
	if l.state != inIntro {
		t.Fatal("the band's one part does not come in")
	}
	loopTimes(b, 3)
	// Read in blocks of an odd size, so the phrases fall inside them.
	buf := make([]float32, 2*997)
	var at int64
	var prev float32
	maxStep := float32(0)
	for at < phraseAt(4)+barAt(4) {
		k, _ := b.Read(buf)
		for i := range k {
			f, v := at+int64(i), buf[2*i]
			// Where no part fades into another, the part playing plays
			// exactly as it is, from its phrase's first frame.
			want, exact := float32(0), true
			switch {
			case f < phraseAt(1):
				want = fakeIntro.at(f)
			case f < phraseAt(2):
				want = fakeLoop.at(f - phraseAt(1))
			case f < phraseAt(2)+seamFrames, f >= phraseAt(3) && f < phraseAt(3)+seamFrames:
				exact = false
			case f < phraseAt(4):
				want = fakeLoop.at(f - phraseAt(2))
				if f >= phraseAt(3) {
					want = fakeLoop.at(f - phraseAt(3))
				}
			case f < phraseAt(4)+barAt(4):
				want = fakeOutro.at(f - phraseAt(4))
			}
			if exact && math.Abs(float64(v-want)) > 1e-6 {
				t.Fatalf("frame %d plays %v, want %v", f, v, want)
			}
			if f > 0 {
				maxStep = max(maxStep, float32(math.Abs(float64(v-prev))))
			}
			prev = v
		}
		at += int64(k)
	}
	// The loop's seams fade from the outro, -0.5, to the loop, 0.5:
	// over seamFrames, a step at most a little over π/2/seamFrames.
	// Played without, the step is 1.
	if maxStep > 0.005 {
		t.Fatalf("a loop came round with a jump of %v", maxStep)
	}
}

func TestLoadFindsPartsByTheirFilesNames(t *testing.T) {
	fsys := fstest.MapFS{
		"music/blade-intro.ogg":      {Data: []byte("x")},
		"music/blade-loop.ogg":       {Data: []byte("x")},
		"music/blade-outro.ogg":      {Data: []byte("x")},
		"music/greek-brass-solo.wav": {Data: []byte("x")},
		"music/encode.sh":            {Data: []byte("x")},
		"music/notes-draft.txt":      {Data: []byte("x")},
	}
	parts, err := Load(fsys, "music")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Name != "blade" || parts[0].Solo || parts[1].Name != "greek-brass" || !parts[1].Solo {
		t.Fatalf("Load found %+v, want blade and the solo greek-brass", parts)
	}
	if _, err := parts[0].Open(Solo); err == nil {
		t.Fatal("blade opens a solo it has none of")
	}
	if _, err := parts[0].Open(Intro); err == nil {
		t.Fatal("blade's intro, of no sound, decodes")
	}
}

func TestAWanderPlaysAsASong(t *testing.T) {
	var s Song = &Wander{Title: "Fake", Artist: "Tests", BPM: 136, Parts: fakeParts(3)}
	if i := s.Info(); i != (Info{Title: "Fake", Artist: "Tests", BPM: 136}) {
		t.Fatalf("Info returned %+v", i)
	}
	p := s.Play(1)
	buf := make([]float32, 2*512)
	if n, err := p.Read(buf); n != 512 || err != nil {
		t.Fatalf("the player read %d frames, %v", n, err)
	}
	if buf[2*100] == 0 {
		t.Fatal("the player plays silence where the intros play")
	}
}
