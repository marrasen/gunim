package geom

import "math"

// Vec3 is a point or a direction in a 3D scene, in the scene's own
// units. Y is up, and the camera looks along -Z, as in OpenGL.
type Vec3 struct{ X, Y, Z float32 }

// V3 is shorthand for Vec3{x, y, z}.
func V3(x, y, z float32) Vec3 { return Vec3{x, y, z} }

// Add returns v moved by w.
func (v Vec3) Add(w Vec3) Vec3 { return Vec3{v.X + w.X, v.Y + w.Y, v.Z + w.Z} }

// Sub returns v moved back by w.
func (v Vec3) Sub(w Vec3) Vec3 { return Vec3{v.X - w.X, v.Y - w.Y, v.Z - w.Z} }

// Mul returns v scaled by s.
func (v Vec3) Mul(s float32) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }

// Dot returns the dot product of v and w.
func (v Vec3) Dot(w Vec3) float32 { return v.X*w.X + v.Y*w.Y + v.Z*w.Z }

// Cross returns the cross product of v and w, at right angles to both.
func (v Vec3) Cross(w Vec3) Vec3 {
	return Vec3{v.Y*w.Z - v.Z*w.Y, v.Z*w.X - v.X*w.Z, v.X*w.Y - v.Y*w.X}
}

// Len returns v's length.
func (v Vec3) Len() float32 { return float32(math.Sqrt(float64(v.Dot(v)))) }

// Unit returns v scaled to length 1, or v itself where it has length 0.
func (v Vec3) Unit() Vec3 {
	if l := v.Len(); l > 0 {
		return v.Mul(1 / l)
	}
	return v
}

// Mat4 is a 4 by 4 matrix that places, turns, scales or projects a 3D
// scene: column by column, as OpenGL reads it, so element [c*4+r] is in
// column c and row r.
type Mat4 [16]float32

// Ident4 is the matrix that leaves every point where it is.
var Ident4 = Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

// Move3 returns the matrix that moves points by v.
func Move3(v Vec3) Mat4 {
	m := Ident4
	m[12], m[13], m[14] = v.X, v.Y, v.Z
	return m
}

// Scale3 returns the matrix that scales points by v, along each axis.
func Scale3(v Vec3) Mat4 {
	return Mat4{v.X, 0, 0, 0, 0, v.Y, 0, 0, 0, 0, v.Z, 0, 0, 0, 0, 1}
}

// TurnX returns the matrix that turns points by rad radians about the X
// axis, bringing Y towards Z.
func TurnX(rad float32) Mat4 {
	s, c := sincos(rad)
	return Mat4{1, 0, 0, 0, 0, c, s, 0, 0, -s, c, 0, 0, 0, 0, 1}
}

// TurnY returns the matrix that turns points by rad radians about the Y
// axis, bringing Z towards X.
func TurnY(rad float32) Mat4 {
	s, c := sincos(rad)
	return Mat4{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1}
}

// TurnZ returns the matrix that turns points by rad radians about the Z
// axis, bringing X towards Y.
func TurnZ(rad float32) Mat4 {
	s, c := sincos(rad)
	return Mat4{c, s, 0, 0, -s, c, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

func sincos(rad float32) (s, c float32) {
	s64, c64 := math.Sincos(float64(rad))
	return float32(s64), float32(c64)
}

// Mul returns m applied after n.
func (m Mat4) Mul(n Mat4) Mat4 {
	var out Mat4
	for c := range 4 {
		for r := range 4 {
			var v float32
			for k := range 4 {
				v += m[k*4+r] * n[c*4+k]
			}
			out[c*4+r] = v
		}
	}
	return out
}

// Apply returns where m takes the point v, divided through by the
// depth a projection gives it.
func (m Mat4) Apply(v Vec3) Vec3 {
	x := m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12]
	y := m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13]
	z := m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14]
	w := m[3]*v.X + m[7]*v.Y + m[11]*v.Z + m[15]
	if w != 0 && w != 1 {
		return Vec3{x / w, y / w, z / w}
	}
	return Vec3{x, y, z}
}

// Perspective returns the projection of a camera that sees fovY radians
// from top to bottom, onto a view aspect times as wide as it is tall,
// between near and far from it.
func Perspective(fovY, aspect, near, far float32) Mat4 {
	f := 1 / float32(math.Tan(float64(fovY)/2))
	return Mat4{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, (far + near) / (near - far), -1,
		0, 0, 2 * far * near / (near - far), 0,
	}
}

// LookAt returns the matrix that takes a scene into the space of a
// camera at eye looking at at, with up pointing as near up as it can.
func LookAt(eye, at, up Vec3) Mat4 {
	f := at.Sub(eye).Unit()
	s := f.Cross(up).Unit()
	u := s.Cross(f)
	return Mat4{
		s.X, u.X, -f.X, 0,
		s.Y, u.Y, -f.Y, 0,
		s.Z, u.Z, -f.Z, 0,
		-s.Dot(eye), -u.Dot(eye), f.Dot(eye), 1,
	}
}

// NormalMatrix returns the 3 by 3 matrix, column by column, that turns
// normals as m turns the surfaces they stand on: the inverse of m's
// upper left 3 by 3, transposed. It is the identity where that has no
// inverse.
func (m Mat4) NormalMatrix() [9]float32 {
	a, b, c := m[0], m[4], m[8]
	d, e, f := m[1], m[5], m[9]
	g, h, k := m[2], m[6], m[10]
	A, B, C := e*k-f*h, -(d*k - f*g), d*h-e*g
	det := a*A + b*B + c*C
	if det == 0 {
		return [9]float32{1, 0, 0, 0, 1, 0, 0, 0, 1}
	}
	inv := 1 / det
	// The inverse's transpose is the cofactors over the determinant,
	// here column by column.
	return [9]float32{
		A * inv, -(b*k - c*h) * inv, (b*f - c*e) * inv,
		B * inv, (a*k - c*g) * inv, -(a*f - c*d) * inv,
		C * inv, -(a*h - b*g) * inv, (a*e - b*d) * inv,
	}
}

// Invert returns the matrix that undoes m, and false where m has none,
// as a scale to zero folds space flat.
func (m Mat4) Invert() (Mat4, bool) {
	var inv Mat4
	inv[0] = m[5]*m[10]*m[15] - m[5]*m[11]*m[14] - m[9]*m[6]*m[15] + m[9]*m[7]*m[14] + m[13]*m[6]*m[11] - m[13]*m[7]*m[10]
	inv[4] = -m[4]*m[10]*m[15] + m[4]*m[11]*m[14] + m[8]*m[6]*m[15] - m[8]*m[7]*m[14] - m[12]*m[6]*m[11] + m[12]*m[7]*m[10]
	inv[8] = m[4]*m[9]*m[15] - m[4]*m[11]*m[13] - m[8]*m[5]*m[15] + m[8]*m[7]*m[13] + m[12]*m[5]*m[11] - m[12]*m[7]*m[9]
	inv[12] = -m[4]*m[9]*m[14] + m[4]*m[10]*m[13] + m[8]*m[5]*m[14] - m[8]*m[6]*m[13] - m[12]*m[5]*m[10] + m[12]*m[6]*m[9]
	inv[1] = -m[1]*m[10]*m[15] + m[1]*m[11]*m[14] + m[9]*m[2]*m[15] - m[9]*m[3]*m[14] - m[13]*m[2]*m[11] + m[13]*m[3]*m[10]
	inv[5] = m[0]*m[10]*m[15] - m[0]*m[11]*m[14] - m[8]*m[2]*m[15] + m[8]*m[3]*m[14] + m[12]*m[2]*m[11] - m[12]*m[3]*m[10]
	inv[9] = -m[0]*m[9]*m[15] + m[0]*m[11]*m[13] + m[8]*m[1]*m[15] - m[8]*m[3]*m[13] - m[12]*m[1]*m[11] + m[12]*m[3]*m[9]
	inv[13] = m[0]*m[9]*m[14] - m[0]*m[10]*m[13] - m[8]*m[1]*m[14] + m[8]*m[2]*m[13] + m[12]*m[1]*m[10] - m[12]*m[2]*m[9]
	inv[2] = m[1]*m[6]*m[15] - m[1]*m[7]*m[14] - m[5]*m[2]*m[15] + m[5]*m[3]*m[14] + m[13]*m[2]*m[7] - m[13]*m[3]*m[6]
	inv[6] = -m[0]*m[6]*m[15] + m[0]*m[7]*m[14] + m[4]*m[2]*m[15] - m[4]*m[3]*m[14] - m[12]*m[2]*m[7] + m[12]*m[3]*m[6]
	inv[10] = m[0]*m[5]*m[15] - m[0]*m[7]*m[13] - m[4]*m[1]*m[15] + m[4]*m[3]*m[13] + m[12]*m[1]*m[7] - m[12]*m[3]*m[5]
	inv[14] = -m[0]*m[5]*m[14] + m[0]*m[6]*m[13] + m[4]*m[1]*m[14] - m[4]*m[2]*m[13] - m[12]*m[1]*m[6] + m[12]*m[2]*m[5]
	inv[3] = -m[1]*m[6]*m[11] + m[1]*m[7]*m[10] + m[5]*m[2]*m[11] - m[5]*m[3]*m[10] - m[9]*m[2]*m[7] + m[9]*m[3]*m[6]
	inv[7] = m[0]*m[6]*m[11] - m[0]*m[7]*m[10] - m[4]*m[2]*m[11] + m[4]*m[3]*m[10] + m[8]*m[2]*m[7] - m[8]*m[3]*m[6]
	inv[11] = -m[0]*m[5]*m[11] + m[0]*m[7]*m[9] + m[4]*m[1]*m[11] - m[4]*m[3]*m[9] - m[8]*m[1]*m[7] + m[8]*m[3]*m[5]
	inv[15] = m[0]*m[5]*m[10] - m[0]*m[6]*m[9] - m[4]*m[1]*m[10] + m[4]*m[2]*m[9] + m[8]*m[1]*m[6] - m[8]*m[2]*m[5]
	det := m[0]*inv[0] + m[1]*inv[4] + m[2]*inv[8] + m[3]*inv[12]
	if det == 0 {
		return Mat4{}, false
	}
	for i := range inv {
		inv[i] /= det
	}
	return inv, true
}
