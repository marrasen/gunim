package driver

import (
	"errors"
	"time"

	"github.com/marrasen/gunim/paint"
)

// CameraOptions say which camera runs, and what reads its frames.
type CameraOptions struct {
	// Front is the camera on the screen's side. Otherwise it is the one on the back, or the only one there is.
	Front bool
	// ReadFrame, where it is set, reads the frames, as one that looks for a code does, on a goroutine of the driver's, one at
	// a time. A frame that comes meanwhile is left out. A call under way as the camera stops goes on.
	ReadFrame func(CameraFrame)
}

// CameraFrame is a frame of a camera's, for a program to read.
type CameraFrame struct {
	// Width and Height are its size in pixels, as the camera gave it.
	Width, Height int
	// Luma is its brightness, a byte a pixel, row by row from the top. It is good until ReadFrame returns, and is then
	// written over.
	Luma []byte
	// Turn is how many degrees clockwise the frame is to be turned to stand upright on the screen: 0, 90, 180 or 270.
	// Mirrored says that the picture the user sees is then shown as in a mirror.
	Turn     int
	Mirrored bool
	// Time is when it came.
	Time time.Time
}

// A Camera is a [Window] on a device with a camera. It runs between StartCamera and StopCamera, which the engine calls; see
// [github.com/marrasen/gunim.CameraWatcher]. The frames stay in memory.
type Camera interface {
	// StartCamera starts the camera, in place of one that runs, as the run numbered. It returns at once: [CameraStopped]
	// says why one that cannot start, or ends by itself, does not run. It returns an error where there is no camera to
	// start.
	StartCamera(run int64, o CameraOptions) error
	// StopCamera stops it, and lets go of the picture.
	StopCamera()
	// CameraPicture returns the picture that came last, upright, as one of a stream of them, or nil before the first. It
	// is called for a [CameraReady].
	CameraPicture() *paint.Image
}

// CameraReady is sent on [Window.Input] as the camera has a new picture, once until it is taken.
type CameraReady struct{}

// CameraStopped is sent on [Window.Input] as a camera that was started as Run does not run: it could not start, or it has
// ended by itself.
type CameraStopped struct {
	Run int64
	Err error
}

// ErrNoCamera is returned where there is no camera that gunim can run.
var ErrNoCamera = errors.New("driver: no camera here")

// ErrCameraPermission is for a camera that the user has not let the application use; see [PermissionCamera].
var ErrCameraPermission = errors.New("driver: the user has not let the application use the camera")

// ErrCameraBusy is for a camera that another application, or another window, has, or that the device has off.
var ErrCameraBusy = errors.New("driver: the camera is in use or off")

// ErrCameraFailed is for a camera that fails.
var ErrCameraFailed = errors.New("driver: the camera fails")
