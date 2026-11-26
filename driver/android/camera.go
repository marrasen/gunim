//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"time"
	"unsafe"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/camera"
	"github.com/marrasen/gunim/paint"
)

// cameraRuns are the camera's runs, one at a time, for the window that
// started each.
var cameraRuns camera.Runs

// StartCamera implements [driver.Camera].
func (w *Window) StartCamera(run int64, o driver.CameraOptions) error {
	r, was := cameraRuns.Start(w, run, o.ReadFrame)
	if was != nil {
		C.gunim_camera_stop(C.longlong(was.ID))
		if ww, ok := was.Owner.(*Window); ok && ww != w {
			ww.in.Push(driver.CameraStopped{Run: was.Number, Err: driver.ErrCameraBusy})
		}
	}
	front := C.int(0)
	if o.Front {
		front = 1
	}
	C.gunim_camera_start(C.longlong(r.ID), front)
	return nil
}

// StopCamera implements [driver.Camera].
func (w *Window) StopCamera() {
	if r := cameraRuns.Stop(w); r != nil {
		C.gunim_camera_stop(C.longlong(r.ID))
	}
}

// CameraPicture implements [driver.Camera].
func (w *Window) CameraPicture() *paint.Image {
	if r := cameraRuns.Now(w); r != nil {
		return r.Picture()
	}
	return nil
}

//export goCameraFrame
func goCameraFrame(id C.longlong, y, u, v unsafe.Pointer, ny, nu, nv C.longlong, w, h, ys, uvs, uvp, turn C.int, mirror C.uchar) {
	r := cameraRuns.Of(int64(id))
	if r == nil || y == nil || u == nil || v == nil || ny <= 0 || nu <= 0 || nv <= 0 {
		return
	}
	f := camera.Frame{
		Y: unsafe.Slice((*byte)(y), int(ny)), U: unsafe.Slice((*byte)(u), int(nu)), V: unsafe.Slice((*byte)(v), int(nv)),
		Width: int(w), Height: int(h), YStride: int(ys), UVStride: int(uvs), UVPixel: int(uvp),
		Turn: int(turn), Mirror: mirror != 0,
	}
	if w, ok := r.Owner.(*Window); ok && r.Frame(&f, time.Now()) {
		w.in.Push(driver.CameraReady{})
	}
}

// goCameraStopped takes the word that the camera ended by itself. The run
// stops here too, and lets go of what it held, as the engine stops nothing
// that has ended.
//
//export goCameraStopped
func goCameraStopped(id C.longlong, why C.int) {
	r := cameraRuns.Ended(int64(id))
	if r == nil {
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
	if w, ok := r.Owner.(*Window); ok {
		w.in.Push(driver.CameraStopped{Run: r.Number, Err: err})
	}
}
