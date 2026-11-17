//go:build !js

package vst3

import (
	"runtime"
	"testing"
	"time"
	"weak"
)

// gone collects garbage until each of ws is gone, for at most two seconds, and reports whether they went.
func gone[T any](ws ...weak.Pointer[T]) bool {
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		runtime.GC()
		left := 0
		for _, w := range ws {
			if w.Value() != nil {
				left++
			}
		}
		if left == 0 {
			return true
		}
	}
	return false
}

// watch returns weak pointers to the sources of fd by their IDs, to their samples, and to fd.
func watch(t *testing.T, fd *fakeDoc) (srcs map[string]weak.Pointer[fakeSource],
	samples map[string][]weak.Pointer[float32], doc weak.Pointer[fakeDoc],
) {
	t.Helper()
	fd.mu.Lock()
	defer fd.mu.Unlock()
	srcs, samples = map[string]weak.Pointer[fakeSource]{}, map[string][]weak.Pointer[float32]{}
	for _, s := range fd.sources {
		if len(s.samples) == 0 {
			t.Fatalf("source %s has no samples after its analysis", s.id)
		}
		srcs[s.id] = weak.Make(s)
		for _, ch := range s.samples {
			samples[s.id] = append(samples[s.id], weak.Make(&ch[0]))
		}
	}
	return srcs, samples, weak.Make(fd)
}

func TestTheFakeLetsGoOfWhatAFinishedSessionRead(t *testing.T) {
	x := newADocument(t)
	x.analyse(t)
	srcs, samples, doc := watch(t, x.a.docs[0])

	// A source taken out of a live document goes.
	err := x.d.Edit(func(e *Edit) {
		e.EnableAudio(x.src64, false)
		e.RemoveRegion(x.r2)
		e.RemoveModification(x.mod2)
		e.RemoveSource(x.src64)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gone(srcs["media-64"]) || !gone(samples["media-64"]...) {
		t.Error("the fake keeps a source taken out of its document")
	}
	if srcs["media-1"].Value() == nil {
		t.Fatal("the fake let go of a source its document still has")
	}

	// A closed document goes whole, with the plugin bound to it, though the fake stays.
	p, _ := bound(t, x.a, x.d, PlaybackRenderer)
	if err := p.ARA().SetRegions(x.r1); err != nil {
		t.Fatal(err)
	}
	x.d.Close()
	if !gone(srcs["media-1"]) || !gone(samples["media-1"]...) {
		t.Error("the fake keeps the sources of a closed document")
	}
	if !gone(doc) {
		t.Error("the fake keeps a closed document")
	}
	noViolations(t, x.a)
	runtime.KeepAlive(x.a)
}
