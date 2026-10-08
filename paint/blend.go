package paint

// Blend is how an op's colour meets what is drawn beneath it.
//
// Shapes, masks and images take a blend: [RRectOp], [MaskOp] and
// [ImageOp]. Text, cells, layers and scenes have none, and draw as
// [BlendNormal] whatever blend is in force.
type Blend uint8

const (
	// BlendNormal lays the op over what is beneath, hiding it as far as
	// the op is opaque. It is the zero Blend.
	BlendNormal Blend = iota
	// BlendAdd adds the op's colour, times its alpha, to what is
	// beneath, and hides nothing, as light does: where two glows
	// overlap they brighten each other, up to white. It is for
	// projectiles, sparks, trails and halos. A colour added to black
	// shows as itself, and an alpha of zero adds nothing.
	//
	// Inside a layer drawn offscreen the light is kept apart from the
	// layer's coverage, and adds to whatever lies beneath the layer when
	// it composites, times the layer's opacity. So a glow inside a layer
	// draws as it would outside, and fades as the layer does. The same
	// holds where nothing opaque lies beneath, as in a transparent
	// window: the light adds to what shows behind it.
	BlendAdd
)

// Blend draws the ops recorded from now on with b, until the returned
// function is called, which puts back the blend in force before:
//
//	defer p.Blend(paint.BlendAdd)()
//
// A shape, mask or image that asks for [BlendAdd] itself, in its own
// Blend, adds whatever the blend in force.
func (p *Painter) Blend(b Blend) func() {
	p.blends = append(p.blends, p.blend)
	p.blend = b
	if p.unblendFn == nil {
		p.unblendFn = p.unblend
	}
	return p.unblendFn
}

func (p *Painter) unblend() {
	n := len(p.blends) - 1
	p.blend = p.blends[n]
	p.blends = p.blends[:n]
}

// blendFor returns the blend an op that asks for b draws with: its own
// where it asks for one, else the one in force.
func (p *Painter) blendFor(b Blend) Blend {
	if b != BlendNormal {
		return b
	}
	return p.blend
}
