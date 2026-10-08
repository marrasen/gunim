// Package compass works out which way a device faces from how it lies in the world, for the drivers of devices
// with a compass. It is plain arithmetic, so it is tested on any machine, and the Android driver's Java half only
// hands it the sensors' rotation matrix.
package compass

import "math"

// A Rotation is how the screen's content is turned from the device's natural way up, as Android's
// Display.getRotation says, a quarter turn at a time. The content turns against the device: a phone turned a
// quarter anticlockwise, its right edge up, draws its content a quarter clockwise, which is Rotation90.
type Rotation int

// The screen's rotations, numbered as Android's Surface.ROTATION_0 to ROTATION_270 are.
const (
	Rotation0 Rotation = iota
	Rotation90
	Rotation180
	Rotation270
)

// level is the least length the way the device faces may have, on the ground, before Heading takes it for none:
// the device lies so that no way stands out.
const level = 1e-3

// edge is how long the screen's right edge is across the ground, out of 1, below which the top edge starts to
// count: the right edge stands within 30° of straight up or down.
const edge = 0.5

// Heading returns which way the device faces, in degrees clockwise from north, from 0 up to but not 360, and false
// where no way stands out.
//
// r is the device's rotation matrix, row by row, as Android's SensorManager.getRotationMatrix and
// getRotationMatrixFromVector give it. It takes a vector in the device's own axes to the world's: the device's x
// runs right across its natural screen, y up it and z out of it, toward the viewer, and the world's x runs east, y
// north and z up to the sky. So column i of r is the device's axis i in the world. rot is the screen's rotation,
// which says which of the device's edges is the top of what it shows.
//
// The way the device faces is the way the top edge of its screen points while it lies flat, screen up, and the way
// its back looks while it stands upright before the viewer, and the same way at every tilt between: the way
// straight ahead of the screen's right edge, across the ground. So tipping the device toward the viewer, or rolling
// it to one side, leaves its heading where it was: both turn the device about its right edge or about the way it
// faces. Held within 30° of on its side, with that edge pointing up or down, the top edge's way across the
// ground takes over, smoothly.
func Heading(r [9]float32, rot Rotation) (degrees float32, ok bool) {
	// right and up are the screen's right and top edges as it shows its content, in the world.
	right, up := axes(r, rot)
	// Turned a quarter anticlockwise on the ground, seen from above, east goes to north: straight ahead of the right
	// edge.
	east, north := -right[1], right[0]
	// On its side, the right edge points up or down and has little way across the ground: the top edge has one then.
	if flat := math.Hypot(right[0], right[1]); flat < edge {
		side := (1 - flat/edge) * (1 - flat/edge)
		east += up[0] * side
		north += up[1] * side
	}
	if math.Hypot(east, north) < level {
		return 0, false
	}
	deg := math.Atan2(east, north) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	// A hair below 360 rounds up to it in a float32.
	if d := float32(deg); d < 360 {
		return d, true
	}
	return 0, true
}

// axes returns the screen's right and top edges in the world, for the screen turned by rot, from the rotation
// matrix r.
func axes(r [9]float32, rot Rotation) (right, up [3]float64) {
	// col is the device's axis i in the world, negated for neg.
	col := func(i int, neg bool) [3]float64 {
		v := [3]float64{float64(r[i]), float64(r[3+i]), float64(r[6+i])}
		if neg {
			v = [3]float64{-v[0], -v[1], -v[2]}
		}
		return v
	}
	switch rot {
	case Rotation90:
		// Turned a quarter anticlockwise: its right edge is the top.
		return col(1, true), col(0, false)
	case Rotation180:
		return col(0, true), col(1, true)
	case Rotation270:
		// Turned a quarter clockwise: its left edge is the top.
		return col(1, false), col(0, true)
	default:
		return col(0, false), col(1, false)
	}
}
