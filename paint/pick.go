package paint

import (
	"math"

	"github.com/marrasen/gunim/geom"
)

// Hit is where a line of sight into a scene meets one of its items.
type Hit struct {
	// Item is the index in the scene's Items of the item met.
	Item int
	// Point is where the line meets it, in the world, and Normal the
	// way the surface faces there, towards the eye.
	Point, Normal geom.Vec3
	// Distance is how far Point is from the camera's eye.
	Distance float32
}

// Pick returns the item that shows at p, for the scene drawn into r, p
// and r in the same space, as a node's Handle gets a pointer's position:
// the nearest item the line of sight through p meets, see-through ones
// included. It reports false where the line meets none. It works out the
// answer on the CPU, from the meshes' triangles, so it holds the same in
// a test as on any GPU.
func (s Scene) Pick(r geom.Rect, p geom.Point) (Hit, bool) {
	size := r.Size()
	if size.W <= 0 || size.H <= 0 {
		return Hit{}, false
	}
	inv, ok := s.Camera.Matrix(size.W / size.H).Invert()
	if !ok {
		return Hit{}, false
	}
	// The line of sight runs through p from the near end of the view to
	// the far one.
	x := 2*(p.X-r.Min.X)/size.W - 1
	y := 1 - 2*(p.Y-r.Min.Y)/size.H
	near, far := inv.Apply(geom.V3(x, y, -1)), inv.Apply(geom.V3(x, y, 1))

	best, found := Hit{}, false
	for i, it := range s.Items {
		if it.Mesh == nil {
			continue
		}
		model := it.Matrix()
		toMesh, ok := model.Invert()
		if !ok {
			continue
		}
		// The line in the mesh's own space, where its triangles are.
		o, e := toMesh.Apply(near), toMesh.Apply(far)
		d := e.Sub(o)
		c, rad := it.Mesh.Bounds()
		if !meetsSphere(o, d, c, rad) {
			continue
		}
		t, n, ok := meetTriangles(it.Mesh, o, d)
		if !ok {
			continue
		}
		at := model.Apply(o.Add(d.Mul(t)))
		dist := at.Sub(s.Camera.Eye).Len()
		if found && dist >= best.Distance {
			continue
		}
		nm := model.NormalMatrix()
		normal := geom.V3(nm[0]*n.X+nm[3]*n.Y+nm[6]*n.Z, nm[1]*n.X+nm[4]*n.Y+nm[7]*n.Z, nm[2]*n.X+nm[5]*n.Y+nm[8]*n.Z).Unit()
		if normal.Dot(far.Sub(near)) > 0 {
			normal = normal.Mul(-1)
		}
		best, found = Hit{Item: i, Point: at, Normal: normal, Distance: dist}, true
	}
	return best, found
}

// meetsSphere reports whether the line o + t*d, for t from 0 to 1,
// comes within the sphere about c of radius rad.
func meetsSphere(o, d, c geom.Vec3, rad float32) bool {
	dd := d.Dot(d)
	if dd == 0 {
		return false
	}
	t := min(max(c.Sub(o).Dot(d)/dd, 0), 1)
	return o.Add(d.Mul(t)).Sub(c).Len() <= rad*1.0001
}

// meetTriangles returns where along o + t*d, t from 0 to 1, the line
// first meets one of m's triangles, from either side, and the
// triangle's normal.
func meetTriangles(m *Mesh, o, d geom.Vec3) (t float32, normal geom.Vec3, ok bool) {
	vs, idx := m.verts, m.idx
	best := float32(math.Inf(1))
	for i := 0; i+2 < len(idx); i += 3 {
		a, b, c := vs[idx[i]].Pos, vs[idx[i+1]].Pos, vs[idx[i+2]].Pos
		// Möller and Trumbore's test.
		e1, e2 := b.Sub(a), c.Sub(a)
		h := d.Cross(e2)
		det := e1.Dot(h)
		if det > -1e-9 && det < 1e-9 {
			continue
		}
		f := 1 / det
		s := o.Sub(a)
		u := f * s.Dot(h)
		if u < 0 || u > 1 {
			continue
		}
		q := s.Cross(e1)
		v := f * d.Dot(q)
		if v < 0 || u+v > 1 {
			continue
		}
		at := f * e2.Dot(q)
		if at < 0 || at > 1 || at >= best {
			continue
		}
		best, normal = at, e1.Cross(e2)
	}
	return best, normal, !math.IsInf(float64(best), 1)
}
