package plugtest

import (
	"slices"
	"sync"
	"testing"
)

// heardFresh is what a Music at level that never heard before hears of the n frames before at.
func heardFresh(level float64, at int64, n int) []float32 {
	dst := make([]float32, 2*n)
	got := min(n, int(at))
	for i := range got {
		l, r := sample(at-int64(got)+int64(i), 48000)
		lv := level
		if lv <= 0 {
			lv = 0.7
		}
		dst[2*i], dst[2*i+1] = float32(lv*l), float32(lv*r)
	}
	return dst[:2*got]
}

// stepMusic moves m on by steps sixtieths of a second.
func stepMusic(m *Music, steps int) {
	for range steps {
		m.Heard(nil)
	}
}

func TestMusicHearsTheSameFramesHoweverItIsRead(t *testing.T) {
	var m Music
	for i, step := range []int{1, 1, 3, 0, 2, 40, 1, 1, 0, 7} {
		stepMusic(&m, step)
		for _, n := range []int{1, 600, 4096, 800, 9000} {
			dst := make([]float32, 2*n)
			got, at, rate := m.Heard(dst)
			want := heardFresh(m.Level, m.at, n)
			if rate != 48000 || at != m.at || got != len(want)/2 || !slices.Equal(dst[:2*got], want) {
				t.Fatalf("read %d of %d frames at %d: %d frames, not the music's %d", i, n, at, got, len(want)/2)
			}
		}
	}
}

func TestMusicHearsALevelChangedAfterAnEarlierRead(t *testing.T) {
	var m Music
	stepMusic(&m, 10)
	dst := make([]float32, 2*4096)
	m.Heard(dst)
	m.Level = 0.3
	n, _, _ := m.Heard(dst)
	if want := heardFresh(0.3, m.at, 4096); !slices.Equal(dst[:2*n], want) {
		t.Fatal("the music did not hear its new level")
	}
}

func TestMusicWorksOutEachFrameOnce(t *testing.T) {
	var m Music
	dst := make([]float32, 2*8192)
	for range 120 {
		stepMusic(&m, 1)
		m.Heard(dst)
		m.Heard(dst[:2*4096])
		m.Heard(dst)
	}
	if m.worked > m.at {
		t.Fatalf("the music worked out %d frames for %d heard", m.worked, m.at)
	}
}

func TestMusicHeardFromTwoGoroutinesAtOnce(t *testing.T) {
	var m Music
	var wg sync.WaitGroup
	bad := make(chan int64, 2)
	for g := range 2 {
		wg.Go(func() {
			dst := make([]float32, 2*(3000+1000*g))
			for range 200 {
				m.Heard(nil)
				n, at, _ := m.Heard(dst)
				if !slices.Equal(dst[:2*n], heardFresh(0, at, len(dst)/2)) {
					bad <- at
					return
				}
			}
		})
	}
	wg.Wait()
	close(bad)
	for at := range bad {
		t.Fatalf("a read at %d heard the wrong frames", at)
	}
}

// BenchmarkMusicHeard times a frame of a view that reads the music in and out: an op is a step and two reads of
// 8192 frames.
func BenchmarkMusicHeard(b *testing.B) {
	var m Music
	dst := make([]float32, 2*8192)
	stepMusic(&m, 60)
	for b.Loop() {
		m.Heard(nil)
		m.Heard(dst)
		m.Heard(dst)
	}
}
