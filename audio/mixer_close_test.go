package audio

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"weak"

	"github.com/marrasen/gunim/anim"
)

// heavy is a sound that plays a level for ever and holds a large buffer, as an instrument's engine does.
type heavy struct{ buf []float32 }

func newHeavy() *heavy { return &heavy{buf: make([]float32, 1<<18)} }

func (h *heavy) Read(dst []float32) (int, error) {
	for i := range dst {
		dst[i] = h.buf[i%len(h.buf)] + 0.01
	}
	return len(dst) / 2, nil
}

// tellers counts the goroutines that tell a mixer's news.
func tellers() int {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	return strings.Count(string(buf), "tellNews(")
}

// settles collects garbage until ok holds, for at most two seconds, and reports whether it did.
func settles(ok func() bool) bool {
	for end := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		runtime.GC()
		if ok() {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

// playHeavy plays a heavy sound in m, and adds a weak pointer to it to ws.
func playHeavy(m *Mixer, ws *[]weak.Pointer[heavy]) *Voice {
	h := newHeavy()
	*ws = append(*ws, weak.Make(h))
	return m.Play(h, Options{})
}

// playDropped plays voices in a mixer, mixes a little, stops half of them, and drops the mixer. It returns weak
// pointers to the voices' sounds.
func playDropped(n int) []weak.Pointer[heavy] {
	m := NewMixer()
	ws := make([]weak.Pointer[heavy], 0, n)
	vs := make([]*Voice, 0, n)
	for range n {
		vs = append(vs, playHeavy(m, &ws))
	}
	mix(m, 256)
	for i, v := range vs {
		if i%2 == 0 {
			v.Stop(0)
		}
	}
	return ws
}

func TestADroppedMixerLetsGoOfItsTellerAndItsVoices(t *testing.T) {
	before := tellers()
	ws := playDropped(4)
	if !settles(func() bool {
		for _, w := range ws {
			if w.Value() != nil {
				return false
			}
		}
		return tellers() <= before
	}) {
		t.Fatalf("a dropped mixer still has %d tellers running, %d before", tellers(), before)
	}
}

func TestCloseEndsTheTellerAndLetsGoOfTheVoices(t *testing.T) {
	before := tellers()
	m := NewMixer()
	ws := make([]weak.Pointer[heavy], 0, 5)
	vs := make([]*Voice, 0, 6)
	for range 4 {
		vs = append(vs, playHeavy(m, &ws))
	}
	// One that ends in the last mix, its end not told yet.
	vs = append(vs, m.Play(constant(64, 0.5, 0.5).Source(), Options{}))
	mix(m, 256)
	// One played after the last mix, still waiting to be taken up.
	vs = append(vs, playHeavy(m, &ws))
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal("a second Close:", err)
	}
	for i, v := range vs {
		if !soon(v.Done()) {
			t.Errorf("voice %d is not done after Close", i)
		}
	}
	if m.Playing() != 0 {
		t.Errorf("%d voices play after Close", m.Playing())
	}
	if !settles(func() bool {
		for _, w := range ws {
			if w.Value() != nil {
				return false
			}
		}
		return tellers() <= before
	}) {
		t.Fatalf("after Close: %d tellers, %d before; sounds kept: %v", tellers(), before, ws)
	}
	// A closed mixer gives silence, and a sound played into it is done at once.
	v := m.Play(constant(1000, 0.5, 0.5).Source(), Options{})
	if !soon(v.Done()) {
		t.Error("a voice played after Close is not done")
	}
	for i, s := range mix(m, 300) {
		if s != 0 {
			t.Fatalf("sample %d after Close is %v", i, s)
		}
	}
	runtime.KeepAlive(vs)
	runtime.KeepAlive(m)
}

func TestCloseDuringAMixIsSafe(t *testing.T) {
	for range 50 {
		m := NewMixer()
		vs := make([]*Voice, 0, 3)
		for range 3 {
			vs = append(vs, m.Play(&endless{}, Options{}))
		}
		stop := make(chan struct{})
		mixed := make(chan struct{})
		go func() {
			defer close(mixed)
			buf := make([]float32, 2*512)
			for {
				select {
				case <-stop:
					return
				default:
					m.Mix(buf)
				}
			}
		}()
		time.Sleep(time.Millisecond)
		go func() { _ = m.Close() }()
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		for i, v := range vs {
			if !soon(v.Done()) {
				t.Fatalf("voice %d is not done after a Close during a mix", i)
			}
		}
		close(stop)
		<-mixed
		if m.Playing() != 0 {
			t.Fatalf("%d voices play after Close", m.Playing())
		}
	}
}

// mixHash plays scenario seed through a mixer, Mix taking frames at a time, and returns a hash of every sample.
func mixHash(seed uint64, frames int) uint64 {
	r := rand.New(rand.NewPCG(seed, 7))
	m := NewMixer()
	sine := func(n int, hz float64) *Clip {
		s := make([]float32, 2*n)
		for i := range n {
			v := float32(0.4 * math.Sin(2*math.Pi*hz*float64(i)/SampleRate))
			s[2*i], s[2*i+1] = v, v*0.7
		}
		return NewClip(s)
	}
	var vs []*Voice
	play := func() {
		eq := NewEQ()
		eq.Set([]Band{{ID: 1, Kind: Bell, Freq: 1000, Gain: 6, Q: 1, On: true}})
		o := Options{Volume: 0.2 + r.Float32(), Pan: r.Float32()*2 - 1, Loop: r.IntN(2) == 0}
		if r.IntN(3) == 0 {
			o.FadeIn = time.Duration(r.IntN(50)) * time.Millisecond
		}
		if r.IntN(4) == 0 {
			o.Insert = eq.Insert()
		}
		vs = append(vs, m.Play(sine(2000+r.IntN(30000), 100+r.Float64()*2000).Source(), o))
	}
	for range 6 {
		play()
	}
	h := fnv.New64a()
	buf := make([]float32, 2*frames)
	b := make([]byte, 4)
	for done := 0; done < 2*SampleRate; done += frames {
		m.Mix(buf)
		for _, s := range buf {
			u := math.Float32bits(s)
			b[0], b[1], b[2], b[3] = byte(u), byte(u>>8), byte(u>>16), byte(u>>24)
			_, _ = h.Write(b)
		}
		if r.IntN(max(1, 4000/frames)) != 0 {
			continue
		}
		v := vs[r.IntN(len(vs))]
		switch r.IntN(9) {
		case 0:
			v.SetVolume(r.Float32()*2, anim.Tween{Duration: 30 * time.Millisecond, Ease: anim.Linear})
		case 1:
			v.SetPan(r.Float32()*2-1, nil)
		case 2:
			v.Pause(10 * time.Millisecond)
		case 3:
			v.Resume(10 * time.Millisecond)
		case 4:
			v.Stop(20 * time.Millisecond)
		case 5:
			_ = v.Seek(time.Duration(r.IntN(200)) * time.Millisecond)
		case 6:
			v.Then(sine(3000, 330).Source())
		case 7:
			if r.IntN(2) == 0 {
				m.FadeOut(40 * time.Millisecond)
			} else {
				m.FadeIn(40 * time.Millisecond)
			}
		default:
			play()
		}
	}
	return h.Sum64()
}

// mixHashes are mixHash's hashes on amd64, by seed and block, as the mixer gave them before it had Close.
var mixHashes = map[[2]int]uint64{
	{1, 1}: 0x7d3ec7189633ed60, {1, 37}: 0x34951927473df395, {1, 128}: 0x9b752cb8fc019039,
	{1, 200}: 0x94ed686fd662f4cf, {1, 1024}: 0x3e1263152589a347,
	{2, 1}: 0x7f84b7534e74a8ee, {2, 37}: 0xac437de156684dfb, {2, 128}: 0x6038692ca7f677e8,
	{2, 200}: 0xf2b3873dab5734a, {2, 1024}: 0x2dade6c65edb8d03,
	{3, 1}: 0x995e553a0af40fda, {3, 37}: 0xd08f918d03143e59, {3, 128}: 0x4d513bed7c88e3b6,
	{3, 200}: 0xda7f377ccb537c4e, {3, 1024}: 0x16de3823ab3d76ee,
}

func TestTheMixIsTheSameSampleForSample(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the hashes are amd64's: other machines fuse multiplies and adds, so their samples differ")
	}
	for _, seed := range []int{1, 2, 3} {
		for _, frames := range []int{1, 37, 128, 200, 1024} {
			got := mixHash(uint64(seed), frames)
			want, ok := mixHashes[[2]int{seed, frames}]
			if !ok || got != want {
				t.Errorf("seed %d, %d frames a mix: hash %#x, want %#x", seed, frames, got, want)
			}
		}
	}
}

// liveHeap returns the bytes of live heap, after a collection.
func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var s runtime.MemStats
	runtime.ReadMemStats(&s)
	return s.HeapAlloc
}

func TestAHundredDroppedMixersFreeTheirMemory(t *testing.T) {
	for _, closed := range []bool{false, true} {
		before, goroutines := liveHeap(), runtime.NumGoroutine()
		for range 100 {
			m := NewMixer()
			for range 4 {
				m.Play(newHeavy(), Options{}).Stop(0)
			}
			mix(m, 128)
			m.Play(newHeavy(), Options{})
			if closed {
				if err := m.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
		settles(func() bool { return runtime.NumGoroutine() <= goroutines })
		after, left := liveHeap(), runtime.NumGoroutine()
		t.Logf("closed %v: live heap %+.1f MB, goroutines %d before, %d after", closed,
			(float64(after)-float64(before))/(1<<20), goroutines, left)
		if left > goroutines || after > before+20<<20 {
			t.Errorf("closed %v: 100 dropped mixers keep %d goroutines and %.1f MB", closed, left-goroutines,
				(float64(after)-float64(before))/(1<<20))
		}
	}
}

// allDone reports which of vs is not done, a moment after it should be, or -1.
func allDone(vs []*Voice) int {
	for i, v := range vs {
		if !soon(v.Done()) {
			return i
		}
	}
	return -1
}

func TestPlaysAroundACloseAreAllDone(t *testing.T) {
	const players, each = 4, 25
	for round := range 200 {
		m := NewMixer()
		var mu sync.Mutex
		var vs []*Voice
		var wg sync.WaitGroup
		for range players {
			wg.Go(func() {
				for range each {
					v := m.Play(&endless{}, Options{})
					mu.Lock()
					vs = append(vs, v)
					mu.Unlock()
				}
			})
		}
		runtime.Gosched()
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		if i := allDone(vs); i >= 0 {
			t.Fatalf("round %d: voice %d of %d is not done", round, i, len(vs))
		}
		if n := m.Playing(); n != 0 {
			t.Fatalf("round %d: %d voices play after Close", round, n)
		}
	}
}

// closeAtRandom mixes, plays and stops voices on three goroutines, and closes the mixer at a moment seed picks. It
// returns the voice not done after the close, and how many there were, or -1.
func closeAtRandom(seed uint64) (late, of int) {
	r := rand.New(rand.NewPCG(seed, 11))
	m := NewMixer()
	var mu sync.Mutex
	var vs []*Voice
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		buf := make([]float32, 2*64)
		for {
			select {
			case <-stop:
				return
			default:
				m.Mix(buf)
			}
		}
	})
	for p := range 2 {
		wg.Go(func() {
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				v := m.Play(&endless{}, Options{})
				if (i+p)%3 == 0 {
					v.Stop(0)
				}
				mu.Lock()
				vs = append(vs, v)
				mu.Unlock()
				runtime.Gosched()
			}
		})
	}
	time.Sleep(time.Duration(r.IntN(300)) * time.Microsecond)
	_ = m.Close()
	close(stop)
	wg.Wait()
	return allDone(vs), len(vs)
}

func TestACloseAtAnyMomentEndsEveryVoice(t *testing.T) {
	for seed := range uint64(150) {
		if late, of := closeAtRandom(seed); late >= 0 {
			t.Fatalf("seed %d: voice %d of %d is not done", seed, late, of)
		}
	}
}

func TestAClosedMixerIsNotSounding(t *testing.T) {
	m := NewMixer()
	m.Play(&endless{}, Options{})
	mix(m, 256)
	if !m.Sounding() {
		t.Fatal("a mixer playing a sound is not sounding")
	}
	_ = m.Close()
	if m.Sounding() {
		t.Error("a closed mixer is sounding")
	}
	mix(m, 256)
	v := m.Play(&endless{}, Options{})
	v.Resume(0)
	if m.Sounding() {
		t.Error("a closed mixer is sounding after a play and a resume")
	}
}
