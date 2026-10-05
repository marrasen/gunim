package paint

import (
	"math"

	"github.com/marrasen/gunim/geom"
)

// Tilt turns a layer in depth, in perspective, as a card turns over or a
// page leans back. The layer turns about the middle of its Bounds, and
// shows what it draws within them. A zero Tilt leaves the layer flat.
type Tilt struct {
	// X tips the layer's top away from the eye by X radians, about the
	// level line through the middle of its Bounds, and Y turns its right
	// side away by Y radians, about the upright line. X turns first.
	X, Y float32
	// Distance is how far the eye is from the screen, in logical pixels:
	// the nearer, the deeper the turn looks. Zero puts it at
	// DefaultDistance.
	Distance float32
	// OneSided shows the layer only while its front faces the eye, as
	// for one face of a card that turns over.
	OneSided bool
}

// DefaultDistance is how far the eye is from the screen for a Tilt with
// no Distance, in logical pixels.
const DefaultDistance = 1000

// tilted reports whether t turns the layer at all.
func (t Tilt) tilted() bool { return t.X != 0 || t.Y != 0 }

// Facing reports whether the layer's front faces the eye: it has turned
// less than a quarter turn, or more than three quarters, either way.
func (t Tilt) Facing() bool {
	return math.Cos(float64(t.X))*math.Cos(float64(t.Y)) > 0
}

// Homography maps points of a plane to where they show on the screen,
// in perspective: a 3 by 3 matrix, row by row, on the point with a 1
// after it.
type Homography [9]float32

// Apply returns where p shows, and w, the depth it is divided by: above
// 1 for a point turned away from the eye, below for one turned towards
// it. A point at or behind the eye has w at or below zero.
func (h Homography) Apply(p geom.Point) (at geom.Point, w float32) {
	x := h[0]*p.X + h[1]*p.Y + h[2]
	y := h[3]*p.X + h[4]*p.Y + h[5]
	w = h[6]*p.X + h[7]*p.Y + h[8]
	if w == 0 {
		return geom.Point{}, 0
	}
	return geom.Pt(x/w, y/w), w
}

// invert returns the homography that undoes h, and false where h folds
// the plane onto a line, as a layer turned exactly edge on does.
func (h Homography) invert() (Homography, bool) {
	a, b, c, d, e, f, g, k, m := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7], h[8]
	A, B, C := e*m-f*k, -(d*m - f*g), d*k-e*g
	det := a*A + b*B + c*C
	if det == 0 {
		return Homography{}, false
	}
	inv := Homography{
		A, -(b*m - c*k), b*f - c*e,
		B, a*m - c*g, -(a*f - c*d),
		C, -(a*k - b*g), a*e - b*d,
	}
	for i := range inv {
		inv[i] /= det
	}
	return inv, true
}

// Homography returns the homography that tilts the plane about centre,
// a point in the same space as the points it maps.
func (t Tilt) Homography(centre geom.Point) Homography {
	d := t.Distance
	if d <= 0 {
		d = DefaultDistance
	}
	sx, cx := math.Sincos(float64(t.X))
	sy, cy := math.Sincos(float64(t.Y))
	// A point u, v from the centre turns to x, y, z, with z away from
	// the eye, and shows at d/(d+z) of x, y from the centre.
	r00, r01 := float32(cy), float32(sx*sy)
	r10, r11 := float32(0), float32(cx)
	r20, r21 := float32(sy)/d, float32(-sx*cy)/d
	ox, oy := centre.X, centre.Y
	// The tilt about the origin, between a move of the centre there and
	// back.
	w0 := 1 - r20*ox - r21*oy
	return Homography{
		r00 + ox*r20, r01 + ox*r21, -r00*ox - r01*oy + ox*w0,
		r10 + oy*r20, r11 + oy*r21, -r10*ox - r11*oy + oy*w0,
		r20, r21, w0,
	}
}

// A Projection is the perspective the tilted layers open round a node
// put its drawing through, from the window's flat space to where it
// shows on the screen. It stays valid after the frame that made it,
// which lets input be mapped through what the frame showed. A nil
// Projection leaves every point where it is.
type Projection struct {
	outer  *Projection
	h, inv Homography
	ok     bool
	// hidden says the layer shows its back to the eye, and is one-sided.
	hidden bool
}

// newProjection returns the projection of a layer tilted by t about
// centre, inside outer.
func newProjection(t Tilt, centre geom.Point, outer *Projection) *Projection {
	pr := &Projection{outer: outer, h: t.Homography(centre), hidden: t.OneSided && !t.Facing()}
	pr.inv, pr.ok = pr.h.invert()
	return pr
}

// Apply returns where p, a point in the window's flat space, shows on
// the screen.
func (pr *Projection) Apply(p geom.Point) geom.Point {
	for ; pr != nil; pr = pr.outer {
		p, _ = pr.h.Apply(p)
	}
	return p
}

// Unapply returns the point in the window's flat space that shows at
// p, and false where no point does: a layer around it shows its back
// and is one-sided, stands edge on, or turns p's line of sight behind
// the eye.
func (pr *Projection) Unapply(p geom.Point) (geom.Point, bool) {
	if pr == nil {
		return p, true
	}
	p, ok := pr.outer.Unapply(p)
	if !ok || !pr.ok || pr.hidden {
		return p, false
	}
	q, w := pr.inv.Apply(p)
	if w == 0 {
		return q, false
	}
	// The point found must lie in front of the eye, where it shows.
	if _, front := pr.h.Apply(q); front <= 0 {
		return q, false
	}
	return q, true
}
