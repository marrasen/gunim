//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/camera"
	"github.com/marrasen/gunim/paint"
)

// pictureMost is how many pixels a picture to show has at the most, each
// way.
const pictureMost = 960

// cameraRun is a run of the camera, for the window that started it.
type cameraRun struct {
	w   *Window
	run int64
	// java is the number Java knows it by.
	java    int64
	stopped atomic.Bool
	stream  paint.Stream

	// mu guards the picture that came last, and whether the window has
	// been told of one.
	mu      sync.Mutex
	picture *paint.Image
	told    bool

	// read is what reads the frames, frames carries them, one at a time,
	// and reading says that one is being read.
	read    func(driver.CameraFrame)
	frames  chan driver.CameraFrame
	reading atomic.Bool
	done    chan struct{}
	// buf is the memory of the frame being read, written while reading is
	// false.
	buf []byte
}

// cameraNow is the run there is, or nil, and cameraRuns numbers them.
var (
	cameraMu   sync.Mutex
	cameraNow  *cameraRun
	cameraRuns int64
)

// StartCamera implements [driver.Camera].
func (w *Window) StartCamera(run int64, o driver.CameraOptions) error {
	cameraMu.Lock()
	defer cameraMu.Unlock()
	if was := cameraNow; was != nil {
		was.stop()
		if was.w != w {
			was.w.in.Push(driver.CameraStopped{Run: was.run, Err: driver.ErrCameraBusy})
		}
	}
	cameraRuns++
	c := &cameraRun{w: w, run: run, java: cameraRuns, done: make(chan struct{})}
	if o.ReadFrame != nil {
		c.read, c.frames = o.ReadFrame, make(chan driver.CameraFrame, 1)
		go c.readFrames()
	}
	cameraNow = c
	front := C.int(0)
	if o.Front {
		front = 1
	}
	C.gunim_camera_start(C.longlong(c.java), front)
	return nil
}

// StopCamera implements [driver.Camera].
func (w *Window) StopCamera() {
	cameraMu.Lock()
	defer cameraMu.Unlock()
	if c := cameraNow; c != nil && c.w == w {
		c.stop()
		cameraNow = nil
	}
}

// CameraPicture implements [driver.Camera].
func (w *Window) CameraPicture() *paint.Image {
	cameraMu.Lock()
	c := cameraNow
	cameraMu.Unlock()
	if c == nil || c.w != w {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.told = false
	return c.picture
}

// stop stops c. It runs with cameraMu held, and Java only queues the
// change for a thread of its own.
func (c *cameraRun) stop() {
	if c.stopped.Swap(true) {
		return
	}
	close(c.done)
	c.mu.Lock()
	c.picture = nil
	c.mu.Unlock()
	C.gunim_camera_stop(C.longlong(c.java))
}

// readFrames hands the program the frames, one at a time, and writes over
// the last as the camera stops.
func (c *cameraRun) readFrames() {
	for {
		select {
		case <-c.done:
			for !c.reading.CompareAndSwap(false, true) {
				select {
				case f := <-c.frames:
					clear(f.Luma)
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			// The camera's thread writes to it no more.
			clear(c.buf)
			return
		case f := <-c.frames:
			if !c.stopped.Load() {
				c.read(f)
			}
			clear(f.Luma)
			c.reading.Store(false)
		}
	}
}

// runOf returns the run that Java knows by the number java, where it is
// the one there is.
func runOf(java int64) *cameraRun {
	cameraMu.Lock()
	defer cameraMu.Unlock()
	if c := cameraNow; c != nil && c.java == java && !c.stopped.Load() {
		return c
	}
	return nil
}

//export goCameraFrame
func goCameraFrame(java C.longlong, y, u, v unsafe.Pointer, ny, nu, nv C.longlong, w, h, ys, uvs, uvp, turn C.int, mirror C.uchar) {
	c := runOf(int64(java))
	if c == nil || y == nil || u == nil || v == nil || ny <= 0 || nu <= 0 || nv <= 0 {
		return
	}
	f := camera.Frame{
		Y: unsafe.Slice((*byte)(y), int(ny)), U: unsafe.Slice((*byte)(u), int(nu)), V: unsafe.Slice((*byte)(v), int(nv)),
		Width: int(w), Height: int(h), YStride: int(ys), UVStride: int(uvs), UVPixel: int(uvp),
		Turn: int(turn), Mirror: mirror != 0,
	}
	now := time.Now()
	if c.read != nil && !c.reading.Swap(true) {
		buf, err := f.Luma(c.buf)
		if err != nil {
			c.reading.Store(false)
			return
		}
		c.buf = buf
		c.frames <- driver.CameraFrame{Width: f.Width, Height: f.Height, Luma: buf, Turn: f.Turn, Mirrored: f.Mirror, Time: now}
	}
	pix, pw, ph, err := f.Picture(pictureMost)
	if err != nil {
		return
	}
	img := c.stream.Image(pw, ph, pix)
	c.mu.Lock()
	tell := !c.told && !c.stopped.Load()
	if !c.stopped.Load() {
		c.picture = img
	}
	c.told = c.told || tell
	c.mu.Unlock()
	if tell {
		c.w.in.Push(driver.CameraReady{})
	}
}

//export goCameraStopped
func goCameraStopped(java C.longlong, why C.int) {
	c := runOf(int64(java))
	if c == nil {
		return
	}
	err := driver.ErrCameraFailed
	switch why {
	case 1:
		err = driver.ErrNoCamera
	case 2:
		err = driver.ErrCameraPermission
	case 3:
		err = driver.ErrCameraBusy
	}
	c.w.in.Push(driver.CameraStopped{Run: c.run, Err: err})
}
