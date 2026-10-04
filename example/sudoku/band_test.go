package main

import (
	"io"
	"math"
	"testing"

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
	fakeIntro = ramp{from: 0.1, to: 0.5, n: phraseAt(1)}
	fakeLoop  = ramp{from: 0.5, to: -0.5, n: phraseAt(1)}
	fakeOutro = ramp{from: -0.5, to: 0, n: barAt(4)}
)

// fakeSynths returns n synths with the fake parts, and a solo.
func fakeSynths(n int) []*synth {
	part := func(p string) (audio.Source, error) {
		var r ramp
		switch p {
		case "intro":
			r = fakeIntro
		case "loop":
			r = fakeLoop
		case "outro":
			r = fakeOutro
		default:
			r = ramp{from: 0.3, to: 0.3, n: barAt(20)}
		}
		return &r, nil
	}
	var out []*synth
	for range n {
		out = append(out, &synth{name: "fake", open: part})
	}
	return append(out, &synth{name: "solo", solo: true, open: part})
}

// conductTo moves b on to the start of phrase p, without playing it.
func conductTo(b *band, p int) {
	for b.phrase < p {
		b.phrase++
		b.at = phraseAt(b.phrase)
		b.turn()
	}
}

// loopTimes has b's one synth, as it comes in, loop n times.
func loopTimes(b *band, n int) {
	l := b.layers[0]
	l.loops, l.next.loops = n-1, n-1
	b.want = 1
}

func TestEachSynthComesInLoopsAndLeavesAndSomethingAlwaysPlays(t *testing.T) {
	const phrases = 400
	b := newBand(fakeSynths(9), 7)
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
			t.Fatalf("phrase %d: the solo starts over %d synths looping", p, looping)
		}
		states = append(states, row)
	}
	if len(counts) < 6 {
		t.Fatalf("over %d phrases, %v synths played: want the number to wander", phrases, counts)
	}
	// Each synth's phrases: rest, an intro, one to maxLoops loops, an
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
					t.Fatalf("synth %d loops more than %d times by phrase %d", i, maxLoops, p)
				}
			case inOutro:
				ok = was == inLoop
				loops = 0
			}
			if !ok {
				t.Fatalf("synth %d goes from %d to %d at phrase %d", i, was, is, p)
			}
		}
		if ins < 3 {
			t.Fatalf("synth %d came in %d times in %d phrases", i, ins, phrases)
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
	b := newBand(fakeSynths(1)[:1], 1)
	l := b.layers[0]
	if l.state != inIntro {
		t.Fatal("the band's one synth does not come in")
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

// lenOf returns the length of a part, decoded.
func lenOf(t *testing.T, src audio.Source) int64 {
	t.Helper()
	s, ok := src.(audio.Seeker)
	if !ok {
		t.Fatal("a part decodes to a source of no length")
	}
	return s.Len()
}

func TestTheSongsPartsAreAllThereAndALoopComesRoundWithoutAClick(t *testing.T) {
	synths := bandSynths()
	solos := 0
	for _, s := range synths {
		if s.solo {
			solos++
			src, err := s.open("solo")
			if err != nil {
				t.Fatal(err)
			}
			if n := lenOf(t, src); n != barAt(20) {
				t.Fatalf("%s's solo is %d frames, want 20 bars, %d", s.name, n, barAt(20))
			}
			continue
		}
		for part, want := range map[string]int64{"intro": phraseAt(1), "loop": phraseAt(1), "outro": barAt(4)} {
			src, err := s.open(part)
			if err != nil {
				t.Fatalf("%s's %s: %v", s.name, part, err)
			}
			if n := lenOf(t, src); n < want-1 || n > want+1 {
				t.Fatalf("%s's %s is %d frames, want %d", s.name, part, n, want)
			}
		}
	}
	if len(synths) != 10 || solos != 1 {
		t.Fatalf("%d synths, %d of them solos; want 10 and 1", len(synths), solos)
	}
	// The synth whose loop jumped most as it came round, played round:
	// the seam steps no further than its sound does about it.
	var blade *synth
	for _, s := range synths {
		if s.name == "blade-main-theme" {
			blade = s
		}
	}
	b := newBand([]*synth{blade}, 1)
	loopTimes(b, 2)
	sound := make([]float32, 2*(phraseAt(2)+barAt(1)))
	if _, err := b.Read(sound); err != nil {
		t.Fatal(err)
	}
	step := func(from, to int64) float32 {
		m := float32(0)
		for f := from; f < to; f++ {
			for c := range int64(2) {
				m = max(m, float32(math.Abs(float64(sound[2*f+c]-sound[2*f-2+c]))))
			}
		}
		return m
	}
	seam, around := step(phraseAt(2)-8, phraseAt(2)+8), step(phraseAt(2)-48000, phraseAt(2)-8)
	if seam > around {
		t.Fatalf("the loop comes round with a step of %v, where the second before it steps %v at most", seam, around)
	}
}
