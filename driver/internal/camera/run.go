package camera

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/paint"
)

// PictureMost is how many pixels a picture to show has at the most, each way.
const PictureMost = 960

// Runs are the runs of a camera, one at a time. The zero value is ready.
type Runs struct {
	mu   sync.Mutex
	now  *Run
	last int64
}

// Run is a run of the camera, for the one that started it.
type Run struct {
	// Owner is who started it, and Number the number it was started as there. ID is the number the camera knows it by.
	Owner  any
	Number int64
	ID     int64

	stopped atomic.Bool
	stream  paint.Stream

	// mu guards the picture that came last, and whether the owner has been told of one.
	mu      sync.Mutex
	picture *paint.Image
	told    bool

	// read is what reads the frames, frames carries them, one at a time, and reading says that one is being read.
	read    func(driver.CameraFrame)
	frames  chan driver.CameraFrame
	reading atomic.Bool
	done    chan struct{}
	// over is closed as the reader has let go of what it read.
	over chan struct{}
	// buf is the memory of the frame being read, written while reading is false.
	buf []byte
}

// Start starts a run for owner, as its number, in place of the one there is, which it returns stopped, or nil. read reads
// the frames, or is nil.
func (rs *Runs) Start(owner any, number int64, read func(driver.CameraFrame)) (r, was *Run) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if was = rs.now; was != nil {
		was.stop()
	}
	rs.last++
	r = &Run{Owner: owner, Number: number, ID: rs.last, done: make(chan struct{}), over: make(chan struct{})}
	if read != nil {
		r.read, r.frames = read, make(chan driver.CameraFrame, 1)
		go r.readFrames()
	} else {
		close(r.over)
	}
	rs.now = r
	return r, was
}

// Stop stops the run of owner, where there is one, and returns it.
func (rs *Runs) Stop(owner any) *Run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.now
	if r == nil || r.Owner != owner {
		return nil
	}
	r.stop()
	rs.now = nil
	return r
}

// Ended stops the run the camera knows as id, which ended by itself, and returns it, or nil where it is not the one there
// is.
func (rs *Runs) Ended(id int64) *Run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.now
	if r == nil || r.ID != id {
		return nil
	}
	r.stop()
	rs.now = nil
	return r
}

// Of returns the run the camera knows as id, where it is the one there is and runs.
func (rs *Runs) Of(id int64) *Run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if r := rs.now; r != nil && r.ID == id && !r.stopped.Load() {
		return r
	}
	return nil
}

// Now returns the run of owner, or nil.
func (rs *Runs) Now(owner any) *Run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if r := rs.now; r != nil && r.Owner == owner {
		return r
	}
	return nil
}

// stop stops r, and lets go of its picture. It runs with the Runs' mu held.
func (r *Run) stop() {
	if r.stopped.Swap(true) {
		return
	}
	close(r.done)
	r.mu.Lock()
	r.picture = nil
	r.mu.Unlock()
}

// Stopped reports whether r has stopped.
func (r *Run) Stopped() bool { return r.stopped.Load() }

// Over is closed once r has stopped and the frame it read last is written over.
func (r *Run) Over() <-chan struct{} { return r.over }

// Picture returns the picture that came last, or nil, and has the next tell the owner again.
func (r *Run) Picture() *paint.Image {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.told = false
	return r.picture
}

// Frame takes a frame of r's, as it came at now: it hands its brightness to the reader, where that is free, and keeps it
// as the picture to show. It reports whether the owner is to be told of a picture, which it is once until it takes one.
// The frame's planes are read and not kept.
func (r *Run) Frame(f *Frame, now time.Time) (tell bool) {
	if r.stopped.Load() {
		return false
	}
	if r.read != nil && !r.reading.Swap(true) {
		buf, err := f.Luma(r.buf)
		if err != nil {
			r.reading.Store(false)
			return false
		}
		r.buf = buf
		r.frames <- driver.CameraFrame{Width: f.Width, Height: f.Height, Luma: buf, Turn: f.Turn, Mirrored: f.Mirror, Time: now}
	}
	pix, w, h, err := f.Picture(PictureMost)
	if err != nil {
		return false
	}
	img := r.stream.Image(w, h, pix)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped.Load() {
		return false
	}
	r.picture = img
	tell = !r.told
	r.told = true
	return tell
}

// readFrames hands the reader the frames, one at a time, and writes over the last as the run stops.
func (r *Run) readFrames() {
	defer close(r.over)
	for {
		select {
		case <-r.done:
			for !r.reading.CompareAndSwap(false, true) {
				select {
				case f := <-r.frames:
					clear(f.Luma)
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			// The camera's thread writes to it no more.
			clear(r.buf)
			return
		case f := <-r.frames:
			if !r.stopped.Load() {
				r.read(f)
			}
			clear(f.Luma)
			r.reading.Store(false)
		}
	}
}
