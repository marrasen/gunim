package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/paint"
)

// CameraOptions say which camera runs, and what reads its frames; see [driver.CameraOptions].
type CameraOptions = driver.CameraOptions

// CameraFrame is a frame of a camera's, for a program to read; see [driver.CameraFrame].
type CameraFrame = driver.CameraFrame

// A CameraWatcher is a node that shows what the camera sees, as [github.com/marrasen/gunim/widget.Viewfinder] does. While it
// is drawn and WatchesCamera reports true, the engine runs the camera, and stops it as the node leaves, is drawn no more or
// stops watching, and as the window goes out of sight: a program has nothing to stop. Where several watch, the one last in
// the tree has it. WatchesCamera is asked after each frame, so a node that starts or stops watching between frames asks for
// one with [UI.Invalidate]. Its options count as the camera starts; one with another Front starts it anew.
//
// The program asks for [driver.PermissionCamera] first, with [App.Ask].
type CameraWatcher interface {
	Node
	WatchesCamera() (CameraOptions, bool)
	// CameraPicture hands it each picture the camera has, upright, or the last of them where it is slower.
	CameraPicture(img *paint.Image, u *UI)
	// CameraStopped says that the camera stopped: err is nil as the engine stopped it, and says otherwise why it could
	// not start, or ended. One that ended starts again once the node has stopped watching, or been out of sight.
	CameraStopped(err error, u *UI)
}

// cameraHold is the node that has the camera, with the camera's side, and whether it runs.
type cameraHold struct {
	s     *state
	run   int64
	front bool
	on    bool
}

// noteCameraNode notes s while its node can watch the camera, so a frame walks the tree for watchers only while it holds
// one. [UI.forget] lets it go.
func (u *UI) noteCameraNode(s *state) {
	if _, ok := s.node.(CameraWatcher); !ok {
		return
	}
	if u.cameraNodes == nil {
		u.cameraNodes = map[*state]struct{}{}
	}
	u.cameraNodes[s] = struct{}{}
}

// InSight reports whether the window is in sight as far as the system says: neither put away, minimized or with its
// application in the background, nor covered whole. It is false on a nil UI.
func (u *UI) InSight() bool { return u != nil && !u.w.hidden && !u.w.covered }

// cameraWatcher returns the node that the camera is to run for, with its options, or nil.
func (u *UI) cameraWatcher() (found *state, options CameraOptions) {
	if len(u.cameraNodes) == 0 || !u.InSight() || u.goingAway {
		return nil, CameraOptions{}
	}
	var walk func(s *state)
	walk = func(s *state) {
		if s.presence == Exiting || s.drawn != u.seq {
			return
		}
		if c, ok := s.node.(CameraWatcher); ok {
			if o, on := c.WatchesCamera(); on {
				found, options = s, o
			}
		}
		for _, k := range s.kids {
			walk(k)
		}
	}
	for _, r := range u.roots() {
		walk(r)
	}
	return found, options
}

// watchCamera runs the camera for the node that watches it, and stops it once none does. It runs after each frame, which
// settles what is drawn, and as the window goes out of sight.
func (u *UI) watchCamera() {
	s, o := u.cameraWatcher()
	if s == u.camera.s && (s == nil || o.Front == u.camera.front) {
		return
	}
	u.stopCamera()
	if s == nil {
		return
	}
	u.camera = cameraHold{s: s, front: o.Front, run: u.camera.run + 1}
	err := ErrNoCamera
	if c, ok := u.w.dw.(driver.Camera); ok {
		err = c.StartCamera(u.camera.run, o)
	}
	if err != nil {
		u.cameraEnded(err)
		return
	}
	u.camera.on = true
}

// ErrNoCamera is for a device with no camera that gunim can run; see [driver.ErrNoCamera].
var ErrNoCamera = driver.ErrNoCamera

// stopCamera stops the camera, where it runs, and lets go of the node that had it.
func (u *UI) stopCamera() {
	on := u.camera.on
	s := u.camera.s
	u.camera = cameraHold{run: u.camera.run}
	if !on {
		return
	}
	if c, ok := u.w.dw.(driver.Camera); ok {
		c.StopCamera()
	}
	u.tellCamera(s, nil)
}

// cameraEnded takes the word that the camera does not run. The node keeps it, so that it is not started again at once.
func (u *UI) cameraEnded(err error) {
	u.camera.on = false
	u.tellCamera(u.camera.s, err)
}

// tellCamera tells the node at s that the camera stopped.
func (u *UI) tellCamera(s *state, err error) {
	if c, ok := s.node.(CameraWatcher); ok && u.index[s.node] == s {
		u.on(s, func() { c.CameraStopped(err, u) })
	}
}

// cameraReady hands the camera's last picture to the node that has it.
func (u *UI) cameraReady() {
	d, ok := u.w.dw.(driver.Camera)
	if !ok {
		return
	}
	img := d.CameraPicture()
	if img == nil || !u.camera.on {
		return
	}
	s := u.camera.s
	if c, ok := s.node.(CameraWatcher); ok && !s.leaving() {
		u.on(s, func() { c.CameraPicture(img, u) })
	}
}
