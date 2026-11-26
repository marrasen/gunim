package camera

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/marrasen/gunim/driver"
)

// bright is a frame of brightness b everywhere.
func bright(b byte) *Frame {
	f := frame(8, 6)
	for i := range f.Y {
		f.Y[i] = b
	}
	return f
}

// A run that ends by itself lets go of its picture, its reader and the frame read last, as one that is stopped does.
func TestARunThatEndsKeepsNothing(t *testing.T) {
	for _, how := range []string{"ended", "stopped", "replaced"} {
		synctest.Test(t, func(t *testing.T) {
			var rs Runs
			var mu sync.Mutex
			var read []byte
			r, was := rs.Start("window", 1, func(f driver.CameraFrame) {
				mu.Lock()
				defer mu.Unlock()
				read = f.Luma
			})
			if was != nil {
				t.Fatal("a run was there before the first")
			}
			if !r.Frame(bright(200), time.Now()) || r.Picture() == nil {
				t.Fatal("a frame came to nothing")
			}
			synctest.Wait()
			mu.Lock()
			kept := read[:cap(read)]
			mu.Unlock()
			switch how {
			case "ended":
				if rs.Ended(r.ID+1) != nil || rs.Ended(r.ID) != r {
					t.Fatal("the run that ended was not found by its number alone")
				}
			case "stopped":
				if rs.Stop("another") != nil || rs.Stop("window") != r {
					t.Fatal("the run was not found by its owner alone")
				}
			case "replaced":
				if _, was := rs.Start("another", 1, nil); was != r {
					t.Fatal("the run before was not returned")
				}
			}
			<-r.Over()
			if !r.Stopped() || r.Picture() != nil || rs.Of(r.ID) != nil {
				t.Fatalf("%s, the run is still there, or keeps its picture", how)
			}
			if slices.ContainsFunc(kept, func(b byte) bool { return b != 0 }) {
				t.Fatalf("%s, the frame read last is still in memory", how)
			}
			if r.Frame(bright(9), time.Now()) || r.Picture() != nil {
				t.Fatalf("%s, a frame after the end was taken", how)
			}
		})
	}
}

// The owner is told of a picture once until it takes one, and takes the last.
func TestAPictureIsToldOnceUntilTaken(t *testing.T) {
	var rs Runs
	r, _ := rs.Start("w", 7, nil)
	if rs.Now("w") != r || rs.Now("v") != nil || r.Number != 7 {
		t.Fatal("the run is not the owner's")
	}
	if !r.Frame(bright(1), time.Now()) || r.Frame(bright(2), time.Now()) {
		t.Fatal("the owner was told twice before it took one")
	}
	if p := r.Picture(); p == nil || p.Pix()[0] != 2 {
		t.Fatal("the picture taken is not the last")
	}
	if !r.Frame(bright(3), time.Now()) {
		t.Fatal("the owner was not told again once it took one")
	}
	bad := bright(4)
	bad.Turn = 45
	if r.Frame(bad, time.Now()) {
		t.Fatal("a frame that lies was taken")
	}
}

// A frame that comes while the reader reads is left out, and each read frame is written over as the reader returns.
func TestAReaderReadsOneFrameAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var rs Runs
		var seen [][]byte
		gate := make(chan bool)
		r, _ := rs.Start("w", 1, func(f driver.CameraFrame) {
			seen = append(seen, slices.Clone(f.Luma))
			<-gate
		})
		r.Frame(bright(1), time.Now())
		synctest.Wait()
		r.Frame(bright(2), time.Now())
		r.Frame(bright(3), time.Now())
		gate <- true
		synctest.Wait()
		r.Frame(bright(4), time.Now())
		synctest.Wait()
		gate <- true
		synctest.Wait()
		if len(seen) != 2 || seen[0][0] != 1 || seen[1][0] != 4 {
			t.Fatalf("the reader read %d frames", len(seen))
		}
		rs.Stop("w")
		<-r.Over()
	})
}
