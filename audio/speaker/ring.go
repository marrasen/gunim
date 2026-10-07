package speaker

import "sync/atomic"

// ring holds frames between one goroutine that writes them and another
// that reads them, each without waiting for the other.
type ring struct {
	buf []float32
	// w and r count the frames written and read, ever.
	w, r atomic.Int64
}

func newRing(frames int) *ring { return &ring{buf: make([]float32, 2*frames)} }

// len returns how many frames wait to be read.
func (q *ring) len() int64 { return q.w.Load() - q.r.Load() }

// room returns how many frames can be written.
func (q *ring) room() int64 { return int64(len(q.buf)/2) - q.len() }

// push writes frames, as many as there is room for, and returns how many.
func (q *ring) push(frames []float32) int {
	size := int64(len(q.buf) / 2)
	w := q.w.Load()
	n := min(int64(len(frames)/2), size-(w-q.r.Load()))
	for i := range n {
		at := 2 * ((w + i) % size)
		q.buf[at], q.buf[at+1] = frames[2*i], frames[2*i+1]
	}
	q.w.Store(w + n)
	return int(n)
}

// pop reads frames into dst, as many as wait, and returns how many.
func (q *ring) pop(dst []float32) int {
	size := int64(len(q.buf) / 2)
	r := q.r.Load()
	n := min(int64(len(dst)/2), q.w.Load()-r)
	for i := range n {
		at := 2 * ((r + i) % size)
		dst[2*i], dst[2*i+1] = q.buf[at], q.buf[at+1]
	}
	q.r.Store(r + n)
	return int(n)
}
