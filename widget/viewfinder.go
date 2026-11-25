package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// ViewfinderFill is what a [Viewfinder] shows with no picture.
var ViewfinderFill = theme.Color("viewfinder.fill", color.NRGBA{R: 0x0c, G: 0x0e, B: 0x12, A: 0xff})

// Viewfinder shows what the device's camera sees, live, and hands each frame's brightness to the program, as to one that
// reads codes. The picture fades in as it comes, and out as the camera stops. It takes no keys.
//
// The camera runs while the viewfinder is drawn, and stops as it leaves, as its window goes out of sight, and while SetOn has
// it off: the program has nothing to stop. Its frames stay in memory, and once it stops the viewfinder keeps none. Ask for
// [github.com/marrasen/gunim/driver.PermissionCamera] before it shows.
type Viewfinder struct {
	anim.Group
	// Front is the camera on the screen's side. Set it before the viewfinder shows, or while it is off.
	Front bool
	// ReadFrame reads the frames, on a goroutine of its own, one at a time, and those that come meanwhile are left out; see
	// [driver.CameraOptions]. Set it before the viewfinder shows.
	ReadFrame func(f gunim.CameraFrame)
	// OnStopped runs as the camera could not start, or ended by itself, as with no permission or where another application
	// has it. It stays so until SetOn has it off and on, or the viewfinder has been out of sight.
	OnStopped func(err error, u *gunim.UI) gunim.Intent
	// Fit is how the picture fills the box. NewViewfinder sets it to [FitCover].
	Fit Fit
	// Label says what it is, for a screen reader.
	Label string
	// Radius rounds the corners.
	Radius float32
	// Size is the size it asks for. Zero takes the room there is.
	Size geom.Size

	off bool
	img *paint.Image
	// in is how far the picture has faded in.
	in *anim.Float
}

// NewViewfinder returns a viewfinder, which is on.
func NewViewfinder() *Viewfinder {
	v := &Viewfinder{Fit: FitCover, in: anim.NewFloat(0)}
	v.Add(v.in)
	return v
}

// On reports whether the viewfinder runs the camera while it is drawn.
func (v *Viewfinder) On() bool { return !v.off }

// SetOn has the viewfinder run the camera while it is drawn, or not.
func (v *Viewfinder) SetOn(on bool, u *gunim.UI) {
	v.off = !on
	u.Invalidate()
}

// WatchesCamera implements [gunim.CameraWatcher].
func (v *Viewfinder) WatchesCamera() (gunim.CameraOptions, bool) {
	return gunim.CameraOptions{Front: v.Front, ReadFrame: v.ReadFrame}, !v.off
}

// CameraPicture implements [gunim.CameraWatcher].
func (v *Viewfinder) CameraPicture(img *paint.Image, u *gunim.UI) {
	if v.in.Target() == 0 {
		v.in.Animate(1, Settle.Get(u.Theme()))
	}
	v.img = img
	u.Invalidate()
}

// CameraStopped implements [gunim.CameraWatcher].
func (v *Viewfinder) CameraStopped(err error, u *gunim.UI) {
	if u.InSight() {
		v.in.Animate(0, Settle.Get(u.Theme()))
	} else {
		// No frame comes to fade it out in, and it is kept no longer.
		v.in.Jump(0)
		v.img = nil
	}
	u.Invalidate()
	if err != nil && v.OnStopped != nil {
		send(u, v, v.OnStopped(err, u))
	}
}

// Layout implements [gunim.Node].
func (v *Viewfinder) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	size := v.Size
	if size == (geom.Size{}) {
		size = fillOr(c, geom.Sz(320, 320))
	}
	return c.Constrain(size)
}

// Paint implements [gunim.Node].
func (v *Viewfinder) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, v.Radius, paint.Solid(ViewfinderFill.Get(f.Theme)))
	t := min(max(v.in.Value(), 0), 1)
	if t <= 0 && v.in.Target() == 0 {
		// What was seen last is kept no longer than it shows.
		v.img = nil
	}
	if v.img == nil {
		return
	}
	dst, src := fitRects(v.Fit, box, v.img)
	p.Image(v.img, dst, paint.ImageOpts{Src: src, Radius: v.Radius, Opacity: t})
}

// Access implements [gunim.Accessible].
func (v *Viewfinder) Access() access.Info {
	return access.Info{Role: access.RoleImage, Name: v.Label}
}
