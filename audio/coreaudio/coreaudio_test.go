//go:build darwin && !ios

package coreaudio

import (
	"sync/atomic"
	"testing"
	"time"
)

// The default output device asks for its buffers at its rate, a buffer
// of the size asked at a time, and stops asking once it closes.
func TestPlaysAtTheDevicesPace(t *testing.T) {
	var frames, calls atomic.Int64
	d, err := Open(Config{Buffer: 128, Fill: func(f []float32) {
		clear(f)
		frames.Add(int64(len(f) / 2))
		calls.Add(1)
	}, Log: t.Logf})
	if err != nil {
		t.Skipf("no output device to play to: %v", err)
	}
	info := d.Info()
	if info.Buffer <= 0 || info.Rate <= 0 || info.Latency < info.Buffer {
		t.Fatalf("info %+v", info)
	}
	if info.MinBuffer <= 128 && 128 <= info.MaxBuffer && info.Buffer != 128 {
		t.Errorf("buffer %d, want the 128 asked", info.Buffer)
	}
	if _, err := Open(Config{Fill: func([]float32) {}}); err == nil {
		t.Error("a second device opened while one plays")
	}
	const play = 500 * time.Millisecond
	time.Sleep(play)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	got, want := frames.Load(), int64(info.Rate)*int64(play)/int64(time.Second)
	if got < want*8/10 || got > want*12/10+int64(info.Buffer)*4 {
		t.Errorf("filled %d frames in %v, want about %d", got, play, want)
	}
	n := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != n {
		t.Error("filled after Close")
	}
	d2, err := Open(Config{Fill: func(f []float32) { clear(f) }})
	if err != nil {
		t.Fatalf("opening again after Close: %v", err)
	}
	if err := d2.Close(); err != nil {
		t.Fatal(err)
	}
}
