package paint

import (
	"image/color"
	"math"
	"slices"

	"github.com/marrasen/gunim/geom"
)

// A Mesh is the surface of a solid as triangles, in its own 3D space. A
// driver uploads it to the GPU the first time a frame draws it, and keeps
// it there while frames go on drawing it, as it does an [Image]. A Mesh
// stays as it was made; a shape that changes is a new Mesh.
type Mesh struct {
	verts []MeshVertex
	idx   []uint32
}

// MeshVertex is a corner of a mesh's triangles: where it is, the way the
// surface faces there, and its colour.
type MeshVertex struct {
	Pos, Normal geom.Vec3
	Color       color.NRGBA
}

// NewMesh makes a mesh of triangles, each three indices into verts, in
// counterclockwise order seen from the side the surface faces. It
// copies both.
func NewMesh(verts []MeshVertex, indices []uint32) *Mesh {
	return &Mesh{verts: slices.Clone(verts), idx: slices.Clone(indices)}
}

// Vertices returns the mesh's corners, for a driver to upload.
func (m *Mesh) Vertices() []MeshVertex { return m.verts }

// Indices returns the mesh's triangles, three indices each, for a driver
// to upload.
func (m *Mesh) Indices() []uint32 { return m.idx }

// NewSphere returns a sphere of radius 1 about the origin, in colour c, of
// rings bands from pole to pole and segments around: 16 and 32 look
// round at most sizes. A scale in the item's Model makes it an ellipsoid,
// as a head is.
func NewSphere(rings, segments int, c color.NRGBA) *Mesh {
	rings, segments = max(rings, 2), max(segments, 3)
	verts := make([]MeshVertex, 0, (rings+1)*(segments+1))
	for r := 0; r <= rings; r++ {
		phi := math.Pi * float64(r) / float64(rings)
		for s := 0; s <= segments; s++ {
			theta := 2 * math.Pi * float64(s) / float64(segments)
			p := geom.V3(float32(math.Sin(phi)*math.Sin(theta)), float32(math.Cos(phi)), float32(math.Sin(phi)*math.Cos(theta)))
			verts = append(verts, MeshVertex{Pos: p, Normal: p, Color: c})
		}
	}
	idx := make([]uint32, 0, 6*rings*segments)
	row := uint32(segments + 1)
	for r := range uint32(rings) {
		for s := range uint32(segments) {
			a, b := r*row+s, (r+1)*row+s
			idx = append(idx, a, b, a+1, a+1, b, b+1)
		}
	}
	return &Mesh{verts: verts, idx: idx}
}

// NewBox returns a box of size about the origin, in colour c, its six faces
// flat.
func NewBox(size geom.Vec3, c color.NRGBA) *Mesh {
	h := size.Mul(0.5)
	faces := [6]struct{ n, u, v geom.Vec3 }{
		{geom.V3(1, 0, 0), geom.V3(0, 0, -1), geom.V3(0, 1, 0)},
		{geom.V3(-1, 0, 0), geom.V3(0, 0, 1), geom.V3(0, 1, 0)},
		{geom.V3(0, 1, 0), geom.V3(1, 0, 0), geom.V3(0, 0, -1)},
		{geom.V3(0, -1, 0), geom.V3(1, 0, 0), geom.V3(0, 0, 1)},
		{geom.V3(0, 0, 1), geom.V3(1, 0, 0), geom.V3(0, 1, 0)},
		{geom.V3(0, 0, -1), geom.V3(-1, 0, 0), geom.V3(0, 1, 0)},
	}
	scale := func(v geom.Vec3) geom.Vec3 { return geom.V3(v.X*h.X, v.Y*h.Y, v.Z*h.Z) }
	verts := make([]MeshVertex, 0, 4*len(faces))
	idx := make([]uint32, 0, 6*len(faces))
	for _, f := range faces {
		base := uint32(len(verts))
		for _, corner := range [4][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			p := f.n.Add(f.u.Mul(corner[0])).Add(f.v.Mul(corner[1]))
			verts = append(verts, MeshVertex{Pos: scale(p), Normal: f.n, Color: c})
		}
		idx = append(idx, base, base+1, base+2, base, base+2, base+3)
	}
	return &Mesh{verts: verts, idx: idx}
}

// A Scene is a 3D view: meshes placed in a world, seen through a camera
// and lit by a light. A frame that draws a different one animates it.
type Scene struct {
	Camera Camera
	Light  Light
	Items  []SceneItem
	// Background fills the view behind the meshes. A zero one leaves it
	// clear, showing what is painted behind the view.
	Background color.NRGBA
}

// Camera is where a scene is seen from.
type Camera struct {
	// Eye is where the camera is, At what it looks at, and Up the way
	// that is up in the picture; a zero Up is +Y.
	Eye, At, Up geom.Vec3
	// FOV is how much the camera sees from top to bottom, in radians; a
	// zero FOV is DefaultFOV. Near and Far are the nearest and furthest
	// it sees; zero ones are 0.1 and 100.
	FOV, Near, Far float32
}

// DefaultFOV is the field of view of a Camera with none, in radians:
// 45 degrees.
const DefaultFOV = math.Pi / 4

// Light lights a scene: one light far off, as the sun, and an even light
// from everywhere under it.
type Light struct {
	// Direction is the way the light shines; a zero one shines from the
	// camera's upper left, down and away.
	Direction geom.Vec3
	// Color is the light's colour, and Ambient the even light's. Zero
	// ones are a light and a dim grey, which together light a surface
	// facing the light at its own colour; brighter ones wash it out.
	Color, Ambient color.NRGBA
}

// SceneItem is a mesh placed in a scene.
type SceneItem struct {
	Mesh *Mesh
	// Model places, turns and scales the mesh in the world; a zero Model
	// leaves it as it was made.
	Model geom.Mat4
	// Tint multiplies the mesh's colours; a zero Tint leaves them.
	Tint color.NRGBA
	// Shine is how glossy the surface is, from 0, matte, up; 32 is a
	// soft gloss and 128 a sharp one.
	Shine float32
}

// SceneOp draws a scene into Rect.
type SceneOp struct {
	Rect      geom.Rect
	Scene     Scene
	Transform Transform
}

func (*SceneOp) isOp() {}

// Scene records s drawn into r. The view's aspect is r's, so a sphere
// stays round in a wide view.
func (p *Painter) Scene(r geom.Rect, s Scene) {
	if r.Empty() {
		return
	}
	s.Items = slices.Clone(s.Items)
	p.record(&SceneOp{Rect: r, Scene: s, Transform: p.at()}, r)
}

// sameScene reports whether two scene ops draw the same.
func sameScene(a, b *SceneOp) bool {
	return a.Rect == b.Rect && a.Transform == b.Transform && a.Scene.Camera == b.Scene.Camera &&
		a.Scene.Light == b.Scene.Light && a.Scene.Background == b.Scene.Background &&
		slices.Equal(a.Scene.Items, b.Scene.Items)
}
