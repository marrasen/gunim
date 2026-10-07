package audioui

import (
	"runtime"
	"testing"
)

// Reading a long sound's loudness costs what each 100 ms brings, however long the sound has played: the panel takes
// only the meter's new blocks, where it once copied the whole history each time.
func TestTheLoudnessPanelTakesOnlyWhatIsNew(t *testing.T) {
	const rate = 4000
	l := NewLoudness(rate)
	tenth := make([]float32, 2*rate/10)
	for i := range tenth {
		tenth[i] = 0.1 * float32(i%40) / 40
	}
	// Ten minutes in, the meter holds 6000 blocks and as many windows: 48 kB each to copy.
	for range 10 * 60 * 10 {
		l.Write(tenth)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	const tenths = 20
	for range tenths {
		l.Write(tenth)
	}
	runtime.ReadMemStats(&after)
	if per := (after.TotalAlloc - before.TotalAlloc) / tenths; per > 4096 {
		t.Fatalf("ten minutes in, each 100 ms written allocates %d bytes, want what the new block takes", per)
	}
}
